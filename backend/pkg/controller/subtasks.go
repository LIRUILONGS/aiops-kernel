package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"opsagi/pkg/database"
	"opsagi/pkg/providers"
	"opsagi/pkg/tools"

	"github.com/sirupsen/logrus"
)

type SubtaskController interface {
	LoadSubtasks(ctx context.Context, taskID int64, updater TaskUpdater) error
	GenerateSubtasks(ctx context.Context) error
	RefineSubtasks(ctx context.Context) error
	PopSubtasks(ctx context.Context, updater TaskUpdater, limit int) ([]SubtaskWorker, error)
	ListSubtasks(ctx context.Context) []SubtaskWorker
	GetSubtask(ctx context.Context, subtaskID int64) (SubtaskWorker, error)
	InvalidateSubtasks(subtaskIDs []int64)
}

type subtaskController struct {
	mx       *sync.Mutex
	taskCtx  *TaskContext
	subtasks map[int64]SubtaskWorker

	// claimed names the subtasks a run has taken but not yet finished. The row
	// turns to running only once its worker starts, so a batch that read the
	// planned list twice would otherwise hand the same subtask to two chains.
	claimed map[int64]struct{}
}

func NewSubtaskController(taskCtx *TaskContext) SubtaskController {
	return &subtaskController{
		mx:       &sync.Mutex{},
		taskCtx:  taskCtx,
		subtasks: make(map[int64]SubtaskWorker),
		claimed:  make(map[int64]struct{}),
	}
}

func (stc *subtaskController) LoadSubtasks(ctx context.Context, taskID int64, updater TaskUpdater) error {
	stc.mx.Lock()
	defer stc.mx.Unlock()

	subtasks, err := stc.taskCtx.DB.GetTaskSubtasks(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get subtasks for task %d: %w", taskID, err)
	}

	if len(subtasks) == 0 {
		return fmt.Errorf("no subtasks found for task %d: %w", taskID, ErrNothingToLoad)
	}

	for _, subtask := range subtasks {
		st, err := LoadSubtaskWorker(ctx, subtask, stc.taskCtx, updater)
		if err != nil {
			if errors.Is(err, ErrNothingToLoad) {
				continue
			}

			return fmt.Errorf("failed to create subtask worker: %w", err)
		}

		stc.subtasks[subtask.ID] = st
	}

	return nil
}

func (stc *subtaskController) GenerateSubtasks(ctx context.Context) error {
	plan, err := stc.taskCtx.Provider.GenerateSubtasks(ctx, stc.taskCtx.TaskID)
	if err != nil {
		return fmt.Errorf("failed to generate subtasks for task %d: %w", stc.taskCtx.TaskID, err)
	}

	if len(plan) == 0 {
		if plan == nil {
			return fmt.Errorf("no subtasks generated for task %d", stc.taskCtx.TaskID)
		}

		// An explicit empty generator plan means this task needs no decomposition.
		// Execute the original request as one subtask. A nil plan is not the same
		// thing: it also represents a missing result from a malformed tool call.
		plan = []tools.SubtaskInfo{{
			Title:       stc.taskCtx.TaskTitle,
			Description: stc.taskCtx.TaskInput,
		}}
	}

	// TODO: change it to insert subtasks in transaction
	for _, info := range plan {
		if err := stc.createPlannedSubtask(ctx, info); err != nil {
			return fmt.Errorf("failed to create subtask for task %d: %w", stc.taskCtx.TaskID, err)
		}
	}

	return nil
}

func (stc *subtaskController) createPlannedSubtask(ctx context.Context, info tools.SubtaskInfo) error {
	_, err := stc.taskCtx.DB.CreateSubtask(ctx, database.CreateSubtaskParams{
		Status:      database.SubtaskStatusCreated,
		TaskID:      stc.taskCtx.TaskID,
		Title:       info.Title,
		Description: info.PlanDescription(),
	})
	return err
}

func (stc *subtaskController) RefineSubtasks(ctx context.Context) error {
	subtasks, err := stc.taskCtx.DB.GetTaskSubtasks(ctx, stc.taskCtx.TaskID)
	if err != nil {
		return fmt.Errorf("failed to get task %d subtasks: %w", stc.taskCtx.TaskID, err)
	}

	plan, err := stc.taskCtx.Provider.RefineSubtasks(ctx, stc.taskCtx.TaskID)
	if err != nil {
		// Refinement is advisory while a previously planned subtask remains.
		// A model refusal must not strand the task, but a database failure,
		// cancellation, or a missing remaining plan still needs its normal error.
		if ctx.Err() == nil && !errors.Is(err, context.Canceled) && errors.Is(err, providers.ErrAgentModelCall) {
			for _, subtask := range subtasks {
				if subtask.Status == database.SubtaskStatusCreated {
					logrus.WithContext(ctx).WithError(err).WithField("task_id", stc.taskCtx.TaskID).
						Warn("subtask refiner failed; continuing the existing plan")
					return nil
				}
			}
		}
		return fmt.Errorf("failed to refine subtasks for task %d: %w", stc.taskCtx.TaskID, err)
	}

	// A nil plan means the refiner produced nothing to apply -- it refused, or the
	// call failed in a way the provider already handled. An empty plan is the
	// opposite: the model removed every remaining hypothesis because the evidence
	// was enough. Dropping that decision here leaves those rows 'created' forever,
	// and the task then waits on a hypothesis nobody will ever run.
	if plan == nil {
		return nil
	}

	subtaskIDs := make([]int64, 0, len(subtasks))
	for _, subtask := range subtasks {
		if subtask.Status == database.SubtaskStatusCreated {
			subtaskIDs = append(subtaskIDs, subtask.ID)
		}
	}

	err = stc.taskCtx.DB.DeleteSubtasks(ctx, subtaskIDs)
	if err != nil {
		return fmt.Errorf("failed to delete subtasks for task %d: %w", stc.taskCtx.TaskID, err)
	}
	stc.InvalidateSubtasks(subtaskIDs)

	// TODO: change it to insert subtasks in transaction and union it with delete ones
	for _, info := range plan {
		if err := stc.createPlannedSubtask(ctx, info); err != nil {
			return fmt.Errorf("failed to create subtask for task %d: %w", stc.taskCtx.TaskID, err)
		}
	}

	return nil
}

func (stc *subtaskController) PopSubtasks(ctx context.Context, updater TaskUpdater, limit int) ([]SubtaskWorker, error) {
	stc.mx.Lock()
	defer stc.mx.Unlock()

	planned, err := stc.taskCtx.DB.GetTaskPlannedSubtasks(ctx, stc.taskCtx.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task planned subtasks: %w", err)
	}

	claimed := claimPlanned(planned, stc.claimed, limit)
	workers := make([]SubtaskWorker, 0, len(claimed))

	for _, stdb := range claimed {
		stc.claimed[stdb.ID] = struct{}{}

		if st, ok := stc.subtasks[stdb.ID]; ok {
			workers = append(workers, st)

			continue
		}

		st, err := NewSubtaskWorker(ctx, stc.taskCtx, stdb.ID, stdb.Title, stdb.Description, updater)
		if err != nil {
			return nil, fmt.Errorf("failed to create subtask worker: %w", err)
		}

		stc.subtasks[stdb.ID] = st
		workers = append(workers, st)
	}

	return workers, nil
}

// claimPlanned takes the first planned subtasks that no run holds yet, in the
// order the planner wrote them. The order matters: a plan is a sequence, and the
// first entries are the ones the planner believed were needed first.
func claimPlanned(planned []database.Subtask, claimed map[int64]struct{}, limit int) []database.Subtask {
	if limit < 1 {
		return nil
	}

	out := make([]database.Subtask, 0, limit)

	for _, subtask := range planned {
		if len(out) == limit {
			break
		}

		if _, taken := claimed[subtask.ID]; taken {
			continue
		}

		out = append(out, subtask)
	}

	return out
}

func (stc *subtaskController) ListSubtasks(ctx context.Context) []SubtaskWorker {
	stc.mx.Lock()
	defer stc.mx.Unlock()

	subtasks := make([]SubtaskWorker, 0)
	for _, subtask := range stc.subtasks {
		subtasks = append(subtasks, subtask)
	}

	sort.Slice(subtasks, func(i, j int) bool {
		return subtasks[i].GetSubtaskID() < subtasks[j].GetSubtaskID()
	})

	return subtasks
}

func (stc *subtaskController) GetSubtask(ctx context.Context, subtaskID int64) (SubtaskWorker, error) {
	stc.mx.Lock()
	defer stc.mx.Unlock()

	subtask, ok := stc.subtasks[subtaskID]
	if !ok {
		return nil, fmt.Errorf("subtask not found")
	}

	return subtask, nil
}

// InvalidateSubtasks drops workers deleted outside this controller,
// preventing stale cache lookups.
func (stc *subtaskController) InvalidateSubtasks(subtaskIDs []int64) {
	if len(subtaskIDs) == 0 {
		return
	}

	stc.mx.Lock()
	defer stc.mx.Unlock()

	for _, id := range subtaskIDs {
		delete(stc.subtasks, id)
	}
}
