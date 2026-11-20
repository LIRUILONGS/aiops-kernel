package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"opsagi/pkg/cast"
	"opsagi/pkg/csum"
	"opsagi/pkg/database"
	"opsagi/pkg/docker"
	obs "opsagi/pkg/observability"
	"opsagi/pkg/observability/langfuse"
	"opsagi/pkg/providers/embeddings"
	"opsagi/pkg/providers/pconfig"
	"opsagi/pkg/providers/provider"
	"opsagi/pkg/templates"
	"opsagi/pkg/tools"

	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/langchaingo/llms"
)

type AssistantProvider interface {
	Type() provider.ProviderType
	Model(opt pconfig.ProviderOptionsType) string
	Title() string
	Language() string
	ToolCallIDTemplate() string
	Embedder() embeddings.Embedder

	SetMsgChainID(msgChainID int64)
	SetAgentLogProvider(agentLog tools.AgentLogProvider)
	SetMsgLogProvider(msgLog tools.MsgLogProvider)
	SetFlowWorker(flowWorker FlowWorker)

	PrepareAgentChain(ctx context.Context) (int64, error)
	PerformAgentChain(ctx context.Context) error
	PutInputToAgentChain(ctx context.Context, input string) error
	EnsureChainConsistency(ctx context.Context) error
}

type FlowWorker interface {
	GetTitle() string
	GetStatus(ctx context.Context) (database.FlowStatus, error)
	PutInput(ctx context.Context, input string, prv provider.Provider, resources []database.UserResource) error
	PutResources(ctx context.Context, resources []database.UserResource) error
	Stop(ctx context.Context) error
	Rename(ctx context.Context, title string) error
	WaitTaskCompletion(ctx context.Context) error
	InvalidateTaskSubtasks(ctx context.Context, taskID int64, subtaskIDs []int64)
}

type assistantProvider struct {
	id         int64
	msgChainID int64
	summarizer csum.Summarizer
	flowWorker FlowWorker
	fp         flowProvider
}

func (ap *assistantProvider) Type() provider.ProviderType {
	return ap.fp.Type()
}

func (ap *assistantProvider) Model(opt pconfig.ProviderOptionsType) string {
	return ap.fp.Model(opt)
}

func (ap *assistantProvider) Title() string {
	return ap.fp.Title()
}

func (ap *assistantProvider) Language() string {
	return ap.fp.Language()
}

func (ap *assistantProvider) ToolCallIDTemplate() string {
	return ap.fp.ToolCallIDTemplate()
}

func (ap *assistantProvider) Embedder() embeddings.Embedder {
	return ap.fp.Embedder()
}

func (ap *assistantProvider) SetMsgChainID(msgChainID int64) {
	ap.msgChainID = msgChainID
}

func (ap *assistantProvider) SetAgentLogProvider(agentLog tools.AgentLogProvider) {
	ap.fp.SetAgentLogProvider(agentLog)
}

func (ap *assistantProvider) SetMsgLogProvider(msgLog tools.MsgLogProvider) {
	ap.fp.SetMsgLogProvider(msgLog)
}

func (ap *assistantProvider) SetFlowWorker(flowWorker FlowWorker) {
	ap.flowWorker = flowWorker
}

func (ap *assistantProvider) PrepareAgentChain(ctx context.Context) (int64, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.PrepareAssistantChain")
	defer span.End()

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
	})

	systemPrompt, err := ap.getAssistantSystemPrompt(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to get assistant system prompt")
		return 0, fmt.Errorf("failed to get assistant system prompt: %w", err)
	}

	msgChainType := database.MsgchainTypeAssistant
	optAgentType := agentByChain[msgChainType]
	ap.msgChainID, _, err = ap.fp.restoreChain(
		ctx, nil, nil, optAgentType, msgChainType, systemPrompt, "",
	)
	if err != nil {
		logger.WithError(err).Error("failed to restore assistant msg chain")
		return 0, fmt.Errorf("failed to restore assistant msg chain: %w", err)
	}

	return ap.msgChainID, nil
}

func (ap *assistantProvider) PerformAgentChain(ctx context.Context) error {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.assistantProvider.PerformAgentChain")
	defer span.End()

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
		"msg_chain_id": ap.msgChainID,
	})

	msgChain, err := ap.fp.DB().GetMsgChain(ctx, ap.msgChainID)
	if err != nil {
		logger.WithError(err).Error("failed to get primary agent msg chain")
		return fmt.Errorf("failed to get primary agent msg chain %d: %w", ap.msgChainID, err)
	}

	var chain []llms.MessageContent
	if err := json.Unmarshal(msgChain.Chain, &chain); err != nil {
		logger.WithError(err).Error("failed to unmarshal primary agent msg chain")
		return fmt.Errorf("failed to unmarshal primary agent msg chain %d: %w", ap.msgChainID, err)
	}

	ctx, observation := obs.Observer.NewObservation(ctx)
	executorAgent := observation.Agent(
		langfuse.WithAgentName(fmt.Sprintf("assistant %d for flow %d: %s", ap.id, ap.fp.ID(), ap.fp.Title())),
		langfuse.WithAgentInput(chain),
		langfuse.WithAgentMetadata(langfuse.Metadata{
			"assistant_id": ap.id,
			"flow_id":      ap.fp.ID(),
			"msg_chain_id": ap.msgChainID,
			"provider":     ap.fp.Type(),
			"image":        ap.fp.Image(),
			"lang":         ap.fp.Language(),
		}),
	)
	ctx, _ = executorAgent.Observation(ctx)

	cfg := tools.AssistantExecutorConfig{
		Summarizer:  ap.fp.GetSummarizeResultHandler(nil, nil),
		FlowManager: ap.buildFlowManagerHandlers(),
	}

	executor, err := ap.fp.Executor().GetAssistantExecutor(cfg)
	if err != nil {
		return wrapErrorEndAgentSpan(ctx, executorAgent, "failed to get assistant executor", err)
	}

	ctx = tools.PutAgentContext(ctx, database.MsgchainTypeAssistant)
	err = ap.fp.performAgentChain(
		ctx, pconfig.OptionsTypeAssistant, msgChain.ID, nil, nil, chain, executor, ap.summarizer,
	)
	if err != nil {
		return wrapErrorEndAgentSpan(ctx, executorAgent, "failed to perform assistant agent chain", err)
	}

	executorAgent.End()

	return nil
}

func (ap *assistantProvider) PutInputToAgentChain(ctx context.Context, input string) error {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.assistantProvider.PutInputToAgentChain")
	defer span.End()

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
		"msg_chain_id": ap.msgChainID,
		"input":        input[:min(len(input), 1000)],
	})

	return ap.fp.processChain(ctx, pconfig.OptionsTypeAssistant, ap.msgChainID, logger,
		func(chain []llms.MessageContent) ([]llms.MessageContent, error) {
			return ap.updateAssistantChain(ctx, chain, input)
		},
	)
}

func (ap *assistantProvider) EnsureChainConsistency(ctx context.Context) error {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.assistantProvider.EnsureChainConsistency")
	defer span.End()

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"flow_id":      ap.fp.flowID,
		"msg_chain_id": ap.msgChainID,
		"assistant_id": ap.id,
	})

	return ap.fp.processChain(ctx, pconfig.OptionsTypeAssistant, ap.msgChainID, logger,
		func(chain []llms.MessageContent) ([]llms.MessageContent, error) {
			return ap.fp.ensureChainConsistency(chain)
		},
	)
}

func (ap *assistantProvider) updateAssistantChain(
	ctx context.Context, chain []llms.MessageContent, humanPrompt string,
) ([]llms.MessageContent, error) {
	systemPrompt, err := ap.getAssistantSystemPrompt(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get assistant system prompt: %w", err)
	}

	if len(chain) == 0 {
		return []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
			llms.TextParts(llms.ChatMessageTypeHuman, humanPrompt),
		}, nil
	}

	ast, err := cast.NewChainAST(chain, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain ast: %w", err)
	}

	systemMessage := llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt)
	ast.Sections[0].Header.SystemMessage = &systemMessage

	ast.AppendHumanMessage(humanPrompt)

	return ast.Messages(), nil
}

func (ap *assistantProvider) getAssistantSystemPrompt(ctx context.Context) (string, error) {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
	})

	executionContext, err := ap.getAssistantExecutionContext(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to get assistant execution context")
		return "", fmt.Errorf("failed to get assistant execution context: %w", err)
	}

	opsTargets := ap.fp.Executor().OpsTargets()
	probes := ap.fp.Executor().ProbeAvailability()

	systemAssistantTmpl, err := ap.fp.Prompter().RenderTemplate(templates.PromptTypeAssistant, map[string]any{
		"TerminalToolName":           tools.TerminalToolName,
		"FileToolName":               tools.FileToolName,
		"WebSearchToolName":          tools.WebSearchToolName,
		"SearchInMemoryToolName":     tools.SearchInMemoryToolName,
		"SearchGuideToolName":        tools.SearchGuideToolName,
		"SearchAnswerToolName":       tools.SearchAnswerToolName,
		"SearchCodeToolName":         tools.SearchCodeToolName,
		"SummarizationToolName":      cast.SummarizationToolName,
		"DockerImage":                ap.fp.Image(),
		"Cwd":                        docker.WorkFolderPathInContainer,
		"ContainerPorts":             ap.fp.getContainerPortsDescription(),
		"ExecutionContext":           executionContext,
		"Lang":                       ap.fp.Language(),
		"CurrentTime":                getCurrentTime(),
		"OpsProbesEnabled":           len(opsTargets) > 0,
		"OpsTargets":                 strings.Join(quoteAll(opsTargets), ", "),
		"OpsSystemToolName":          tools.OpsSystemToolName,
		"OpsContainersToolName":      tools.OpsContainersToolName,
		"OpsLogsToolName":            tools.OpsLogsToolName,
		"OpsBaselineToolName":        tools.OpsBaselineToolName,
		"OpsDiffToolName":            tools.OpsDiffToolName,
		"OpsExecToolName":            tools.OpsExecToolName,
		"PromQueryToolName":          tools.PromQueryToolName,
		"PromQueryEnabled":           probes.Prometheus,
		"K8sGetToolName":             tools.K8sGetToolName,
		"K8sGetEnabled":              probes.Kubernetes,
		"RecordFindingToolName":      tools.RecordFindingToolName,
		"FlowManagerEnabled":         ap.flowWorker != nil,
		"GetFlowStatusToolName":      tools.GetFlowStatusToolName,
		"StopFlowToolName":           tools.StopFlowToolName,
		"SubmitFlowInputToolName":    tools.SubmitFlowInputToolName,
		"PatchFlowSubtasksToolName":  tools.PatchFlowSubtasksToolName,
		"WaitFlowCompletionToolName": tools.WaitFlowCompletionToolName,
		"UserFiles":                  ap.fp.userFilesListing(),
	})
	if err != nil {
		logger.WithError(err).Error("failed to get system prompt for assistant template")
		return "", fmt.Errorf("failed to get system prompt for assistant template: %w", err)
	}

	return systemAssistantTmpl, nil
}

// quoteAll wraps each id in backticks, so the prompt shows the exact string a
// model may pass as `target`.
func quoteAll(ids []string) []string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "`" + id + "`"
	}

	return quoted
}

func (ap *assistantProvider) getAssistantExecutionContext(ctx context.Context) (string, error) {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
	})

	subtasks, err := ap.fp.DB().GetFlowSubtasks(ctx, ap.fp.ID())
	if err != nil {
		logger.WithError(err).Error("failed to get flow subtasks")
		return "", fmt.Errorf("failed to get flow subtasks: %w", err)
	}

	slices.SortFunc(subtasks, func(a, b database.Subtask) int {
		return int(a.ID - b.ID)
	})

	var (
		executionContext     string
		lastActiveSubtaskIDX = -1
	)
	for sdx, subtask := range subtasks {
		if subtask.Status != database.SubtaskStatusCreated {
			lastActiveSubtaskIDX = sdx
		}

		if subtask.Context != "" {
			executionContext = subtask.Context
		}
	}

	if executionContext == "" && len(subtasks) > 0 {
		if lastActiveSubtaskIDX == -1 {
			lastActiveSubtaskIDX = len(subtasks) - 1
		}

		lastSubtask := subtasks[lastActiveSubtaskIDX]
		executionContext, err = ap.fp.prepareExecutionContext(ctx, lastSubtask.TaskID, lastSubtask.ID)
		if err != nil {
			logger.WithError(err).Error("failed to prepare execution context")
			return "", fmt.Errorf("failed to prepare execution context: %w", err)
		}
	}

	return executionContext, nil
}

// buildFlowManagerHandlers constructs FlowManagerHandlers from the injected flowWorker.
// Returns zero-value (no-op) handlers when flowWorker is nil, so the 4 flow-management
// tools are simply not registered in the assistant executor.
func (ap *assistantProvider) buildFlowManagerHandlers() tools.FlowManagerHandlers {
	if ap.flowWorker == nil {
		return tools.FlowManagerHandlers{}
	}

	return tools.FlowManagerHandlers{
		StopFlow:      ap.stopAssistantFlow,
		SendFlowInput: ap.sendAssistantFlowInput,
		PatchSubtasks: ap.patchAssistantFlowSubtasks,
		WaitFlow:      ap.waitAssistantFlowCompletion,
	}
}

func (ap *assistantProvider) stopAssistantFlow(ctx context.Context, reason string) error {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
		"msg_chain_id": ap.msgChainID,
		"reason":       reason[:min(len(reason), 1000)],
	})

	logger.Debug("stopping assistant flow")

	if ap.flowWorker == nil {
		logger.Error("flow worker is not set")
		return fmt.Errorf("flow worker is not set")
	}

	status, err := ap.flowWorker.GetStatus(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to get flow status")
		return fmt.Errorf("failed to get flow status: %w", err)
	}

	if status != database.FlowStatusRunning {
		logger.Debug("flow is not running, skipping stop")
		return nil
	}

	err = ap.flowWorker.Stop(ctx)

	level := langfuse.ObservationLevelDefault
	statusMsg := "success"
	if err != nil {
		level = langfuse.ObservationLevelError
		statusMsg = err.Error()
	}

	_, observation := obs.Observer.NewObservation(ctx)
	observation.Event(
		langfuse.WithEventName("stopping flow by assistant"),
		langfuse.WithEventInput(map[string]any{
			"reason": reason,
		}),
		langfuse.WithEventMetadata(langfuse.Metadata{
			"provider":     ap.fp.Type(),
			"assistant_id": ap.id,
			"flow_id":      ap.fp.ID(),
			"msg_chain_id": ap.msgChainID,
		}),
		langfuse.WithEventLevel(level),
		langfuse.WithEventStatus(statusMsg),
	)

	if err != nil {
		logger.WithError(err).Error("failed to stop flow")
		return fmt.Errorf("failed to stop flow: %w", err)
	}

	return nil
}

func (ap *assistantProvider) waitAssistantFlowCompletion(ctx context.Context) error {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
		"msg_chain_id": ap.msgChainID,
	})

	logger.Debug("waiting for flow task completion from assistant")

	if ap.flowWorker == nil {
		logger.Error("flow worker is not set")
		return fmt.Errorf("flow worker is not set")
	}

	return ap.flowWorker.WaitTaskCompletion(ctx)
}

func (ap *assistantProvider) sendAssistantFlowInput(ctx context.Context, input string) error {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
		"msg_chain_id": ap.msgChainID,
		"input":        input[:min(len(input), 1000)],
	})

	logger.Debug("sending input to flow from assistant")

	if ap.flowWorker == nil {
		logger.Error("flow worker is not set")
		return fmt.Errorf("flow worker is not set")
	}

	status, err := ap.flowWorker.GetStatus(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to get flow status")
		return fmt.Errorf("failed to get flow status: %w", err)
	}

	if status != database.FlowStatusWaiting {
		logger.Error("flow is not waiting, cannot send input")
		return fmt.Errorf("flow is not in 'waiting' state (current: %s); cannot submit input", status)
	}

	err = ap.flowWorker.PutInput(ctx, input, nil, nil)

	level := langfuse.ObservationLevelDefault
	statusMsg := "success"
	if err != nil {
		level = langfuse.ObservationLevelError
		statusMsg = err.Error()
	}

	_, observation := obs.Observer.NewObservation(ctx)
	observation.Event(
		langfuse.WithEventName("sending input to flow by assistant"),
		langfuse.WithEventInput(map[string]any{
			"input": input,
		}),
		langfuse.WithEventMetadata(langfuse.Metadata{
			"provider":     ap.fp.Type(),
			"assistant_id": ap.id,
			"flow_id":      ap.fp.ID(),
			"msg_chain_id": ap.msgChainID,
		}),
		langfuse.WithEventLevel(level),
		langfuse.WithEventStatus(statusMsg),
	)

	if err != nil {
		logger.WithError(err).Error("failed to put flow input")
		return fmt.Errorf("failed to put flow input: %w", err)
	}

	return nil
}

func (ap *assistantProvider) patchAssistantFlowSubtasks(
	ctx context.Context,
	taskID int64,
	patch tools.SubtaskPatch,
) error {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     ap.fp.Type(),
		"assistant_id": ap.id,
		"flow_id":      ap.fp.ID(),
		"msg_chain_id": ap.msgChainID,
		"task_id":      taskID,
		"ops_count":    len(patch.Operations),
	})

	logger.Debug("patching flow subtasks from assistant")

	if ap.flowWorker == nil {
		logger.Error("flow worker is not set")
		return fmt.Errorf("flow worker is not set")
	}

	status, err := ap.flowWorker.GetStatus(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to get flow status")
		return fmt.Errorf("failed to get flow status: %w", err)
	}

	if status == database.FlowStatusRunning {
		logger.Error("flow is running, cannot patch subtasks")
		return fmt.Errorf("flow is in 'running' state; patching is not allowed while a task is executing")
	}

	db := ap.fp.DB()

	tasksInfo, err := ap.fp.getTasksInfo(ctx, taskID)
	if err != nil {
		logger.WithError(err).Error("failed to get tasks info")
		return fmt.Errorf("failed to get tasks info: %w", err)
	}

	if tasksInfo.Task.ID != taskID {
		logger.Error("task is not found in the flow tasks")

		if len(tasksInfo.Tasks) == 0 {
			logger.Warn("no tasks found for the flow")
			return fmt.Errorf("task ID %d not found: the flow has no tasks yet; submit input to create the first task", taskID)
		}

		return fmt.Errorf("task ID %d not found in this flow", taskID)
	}

	subtasksInfo := ap.fp.getSubtasksInfo(taskID, tasksInfo.Subtasks)

	// The current active subtask is normally untouchable by a patch.
	// We include it only when the agent explicitly references it by ID — meaning
	// it consciously wants to remove or modify it, accepting the state reset to created.
	patchableSubs := make([]database.Subtask, 0, len(subtasksInfo.Planned)+1)

	if subtasksInfo.Subtask != nil && slices.ContainsFunc(patch.Operations, func(op tools.SubtaskOperation) bool {
		return op.ID != nil && *op.ID == subtasksInfo.Subtask.ID
	}) {
		patchableSubs = append(patchableSubs, *subtasksInfo.Subtask)
	}

	patchableSubs = append(patchableSubs, subtasksInfo.Planned...)

	logger.WithField("patchable_count", len(patchableSubs)).Debug("built patchable subtask list")

	result, err := applySubtaskOperations(patchableSubs, patch, logger)
	if err != nil {
		return fmt.Errorf("failed to apply subtask operations for task %d: %w", taskID, err)
	}

	idsToDelete := make([]int64, 0, len(patchableSubs))
	for _, st := range patchableSubs {
		idsToDelete = append(idsToDelete, st.ID)
	}

	if len(idsToDelete) > 0 {
		if err := db.DeleteSubtasks(ctx, idsToDelete); err != nil {
			return fmt.Errorf("failed to delete subtasks for task %d: %w", taskID, err)
		}
		// The task worker for taskID (if currently loaded in memory) still holds
		// SubtaskWorker objects for idsToDelete; without this it would later try
		// to operate on rows that no longer exist (e.g. on flow Finish/Stop).
		ap.flowWorker.InvalidateTaskSubtasks(ctx, taskID, idsToDelete)
	}

	// result contains only entries that were in patchableSubs (minus removed ones)
	// plus any new entries from add operations — all safe to create unconditionally.
	for _, info := range result {
		if _, err := db.CreateSubtask(ctx, database.CreateSubtaskParams{
			Status:      database.SubtaskStatusCreated,
			TaskID:      taskID,
			Title:       info.Title,
			Description: info.Description,
			PlanMode:    tools.PlanModeDiagnosis,
		}); err != nil {
			return fmt.Errorf("failed to create subtask %q for task %d: %w", info.Title, taskID, err)
		}
	}

	subtasksResult := convertSubtaskInfoPatch(result)
	_, _ = ap.fp.putAgentLog(
		ctx,
		database.MsgchainTypeAssistant,
		database.MsgchainTypeCommander,
		patch.Message,
		ap.fp.subtasksToMarkdown(subtasksResult),
		&taskID,
		nil,
	)

	level := langfuse.ObservationLevelDefault
	statusMsg := fmt.Sprintf(
		"patched %d planned subtasks for task %d: %d ops applied",
		len(result), taskID, len(patch.Operations),
	)

	_, observation := obs.Observer.NewObservation(ctx)
	observation.Event(
		langfuse.WithEventName("patching flow subtasks by assistant"),
		langfuse.WithEventInput(map[string]any{
			"task_id":    taskID,
			"operations": patch.Operations,
			"message":    patch.Message,
			"subtasks":   patchableSubs,
		}),
		langfuse.WithEventOutput(map[string]any{
			"subtasks": subtasksResult,
		}),
		langfuse.WithEventMetadata(langfuse.Metadata{
			"provider":     ap.fp.Type(),
			"assistant_id": ap.id,
			"flow_id":      ap.fp.ID(),
			"msg_chain_id": ap.msgChainID,
		}),
		langfuse.WithEventLevel(level),
		langfuse.WithEventStatus(statusMsg),
	)

	return nil
}
