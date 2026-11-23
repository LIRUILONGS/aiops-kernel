package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"opsagi/pkg/cast"
	"opsagi/pkg/config"
	"opsagi/pkg/csum"
	"opsagi/pkg/database"
	"opsagi/pkg/docker"
	"opsagi/pkg/flowfiles"
	"opsagi/pkg/graphiti"
	obs "opsagi/pkg/observability"
	"opsagi/pkg/observability/langfuse"
	"opsagi/pkg/providers/embeddings"
	"opsagi/pkg/providers/pconfig"
	"opsagi/pkg/providers/provider"
	"opsagi/pkg/templates"
	"opsagi/pkg/tools"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/langchaingo/llms"
	"github.com/vxcontrol/langchaingo/llms/reasoning"
	"github.com/vxcontrol/langchaingo/llms/streaming"
)

const ToolPlaceholder = "Execute operations via function invocation - textual responses are not acceptable for task completion."

const TasksNumberLimit = 5

const (
	msgGeneratorSizeLimit = 150 * 1024 // 150 KB
	msgRefinerSizeLimit   = 100 * 1024 // 100 KB
	msgReporterSizeLimit  = 100 * 1024 // 100 KB
	msgSummarizerLimit    = 32 * 1024  // 32 KB
)

const textTruncateMessage = "\n\n[...truncated]"

type PerformResult int

const (
	PerformResultError PerformResult = iota
	PerformResultWaiting
	PerformResultDone
)

type StreamMessageChunkType streaming.ChunkType

const (
	StreamMessageChunkTypeThinking StreamMessageChunkType = "thinking"
	StreamMessageChunkTypeContent  StreamMessageChunkType = "content"
	StreamMessageChunkTypeResult   StreamMessageChunkType = "result"
	StreamMessageChunkTypeFlush    StreamMessageChunkType = "flush"
	StreamMessageChunkTypeUpdate   StreamMessageChunkType = "update"
)

type StreamMessageChunk struct {
	Type         StreamMessageChunkType
	MsgType      database.MsglogType
	Content      string
	Thinking     *reasoning.ContentReasoning
	Result       string
	ResultFormat database.MsglogResultFormat
	StreamID     int64
}

type StreamMessageHandler func(ctx context.Context, chunk *StreamMessageChunk) error

type FlowProvider interface {
	ID() int64
	DB() database.Querier
	Type() provider.ProviderType
	Name() provider.ProviderName
	Model(opt pconfig.ProviderOptionsType) string
	Image() string
	Title() string
	Language() string
	ToolCallIDTemplate() string
	Embedder() embeddings.Embedder
	Executor() tools.FlowToolsExecutor
	Prompter() templates.Prompter

	SetTitle(title string)
	SetAgentLogProvider(agentLog tools.AgentLogProvider)
	SetMsgLogProvider(msgLog tools.MsgLogProvider)
	// SetProvider swaps the provider instance backing this flow. It reports
	// whether anything actually changed and, when it did, the tool call ID
	// template resolved for the new provider — so the caller can persist a
	// consistent (provider, template) pair without re-reading shared state.
	SetProvider(ctx context.Context, newProvider provider.Provider) (bool, string, error)

	GetTaskTitle(ctx context.Context, input string) (string, error)
	GenerateSubtasks(ctx context.Context, taskID int64) ([]tools.SubtaskInfo, error)
	RefineSubtasks(ctx context.Context, taskID int64) ([]tools.SubtaskInfo, error)
	GetTaskResult(ctx context.Context, taskID int64) (*tools.TaskResult, error)

	PrepareAgentChain(ctx context.Context, taskID, subtaskID int64) (int64, error)
	PerformAgentChain(ctx context.Context, taskID, subtaskID, msgChainID int64) (PerformResult, error)
	PutInputToAgentChain(ctx context.Context, msgChainID int64, input string) error
	EnsureChainConsistency(ctx context.Context, msgChainID int64) error

	FlowProviderHandlers
}

type FlowProviderHandlers interface {
	GetSummarizeResultHandler(taskID, subtaskID *int64) tools.SummarizeHandler
}

type tasksInfo struct {
	Task     database.Task
	Tasks    []database.Task
	Subtasks []database.Subtask
}

type subtasksInfo struct {
	Subtask   *database.Subtask
	Planned   []database.Subtask
	Completed []database.Subtask
}

type flowProvider struct {
	db database.Querier
	mx *sync.RWMutex

	cfg *config.Config

	embedder       embeddings.Embedder
	graphitiClient *graphiti.Client

	flowID int64

	callCounter *atomic.Int64

	image    string
	title    string
	language string
	askUser bool

	tcIDTemplate string

	prompter templates.Prompter
	executor tools.FlowToolsExecutor
	agentLog tools.AgentLogProvider
	msgLog   tools.MsgLogProvider
	streamCb StreamMessageHandler

	summarizer csum.Summarizer

	summarizerCache *lru.Cache[[32]byte, string]

	maxGACallsLimit int
	maxLACallsLimit int

	provider.Provider
}

// unattended reports whether nobody is watching this instance.
func (fp *flowProvider) unattended() bool {
	return fp.cfg != nil && fp.cfg.Unattended
}

// askGateAction says what the ask tool does here: hand the chain back to an
// operator who will answer, or answer it and let the chain continue.
func (fp *flowProvider) askGateAction() (string, bool) {
	if fp.unattended() {
		return tools.UnattendedAnswer, false
	}

	return "", true
}

func (fp *flowProvider) SetAgentLogProvider(agentLog tools.AgentLogProvider) {
	fp.mx.Lock()
	defer fp.mx.Unlock()

	fp.agentLog = agentLog
}

func (fp *flowProvider) SetMsgLogProvider(msgLog tools.MsgLogProvider) {
	fp.mx.Lock()
	defer fp.mx.Unlock()

	fp.msgLog = msgLog
}

// SetProvider installs newProvider as the one backing this flow and returns
// (changed, toolCallIDTemplate, error).
//
// It is a no-op — changed=false — when newProvider is effectively the one
// already in use. "Effectively" means same name *and* same raw configuration:
// comparing names alone is not enough, because a user provider may be named
// exactly like a built-in one (an intentional feature: it lets a user override
// the product's default configuration for a provider type from the UI). When
// such an override is deleted or renamed away, the name a flow refers to stays
// valid but now resolves to a different configuration, and that switch must go
// through.
func (fp *flowProvider) SetProvider(ctx context.Context, newProvider provider.Provider) (bool, string, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.SetProvider")
	defer span.End()

	if newProvider == nil {
		return false, "", fmt.Errorf("new provider is nil")
	}

	fp.mx.RLock()
	current, prompter := fp.Provider, fp.prompter
	fp.mx.RUnlock()

	if current != nil && current.Name() == newProvider.Name() &&
		bytes.Equal(current.GetRawConfig(), newProvider.GetRawConfig()) {
		return false, fp.ToolCallIDTemplate(), nil
	}

	// Resolved outside the lock on purpose: on a cold cache this probes the
	// provider with live LLM calls, and holding the write lock across it would
	// stall every agent chain reading through this flowProvider. Doing it first
	// also keeps the swap atomic — a failure here leaves the flow untouched.
	tcIDTemplate, err := newProvider.GetToolCallIDTemplate(ctx, prompter)
	if err != nil {
		return false, "", fmt.Errorf("failed to get tool call ID template: %w", err)
	}

	fp.mx.Lock()
	defer fp.mx.Unlock()

	tcIDTemplate = toolCallIDTemplateOrDefault(tcIDTemplate)

	fp.Provider = newProvider
	fp.tcIDTemplate = tcIDTemplate

	return true, tcIDTemplate, nil
}

func (fp *flowProvider) ID() int64 {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.flowID
}

func (fp *flowProvider) DB() database.Querier {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.db
}

func (fp *flowProvider) Image() string {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.image
}

func (fp *flowProvider) Title() string {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.title
}

// userFilesListing returns the compact XML listing of user files in uploads/ and resources/
// for injection into agent system prompts. Returns empty string if no files are present.
func (fp *flowProvider) userFilesListing() string {
	if fp.cfg == nil || fp.cfg.DataDir == "" {
		return ""
	}
	return flowfiles.FileListingForPrompt(fp.cfg.DataDir, uint64(fp.flowID))
}

func (fp *flowProvider) SetTitle(title string) {
	fp.mx.Lock()
	defer fp.mx.Unlock()

	fp.title = title
}

func (fp *flowProvider) Language() string {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.language
}

func (fp *flowProvider) ToolCallIDTemplate() string {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.tcIDTemplate
}

func (fp *flowProvider) Embedder() embeddings.Embedder {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.embedder
}

func (fp *flowProvider) Executor() tools.FlowToolsExecutor {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.executor
}

func (fp *flowProvider) Prompter() templates.Prompter {
	fp.mx.RLock()
	defer fp.mx.RUnlock()

	return fp.prompter
}

func (fp *flowProvider) GetTaskTitle(ctx context.Context, input string) (string, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.GetTaskTitle")
	defer span.End()

	ctx, observation := obs.Observer.NewObservation(ctx)
	getterEvaluator := observation.Evaluator(
		langfuse.WithEvaluatorName("get task title"),
		langfuse.WithEvaluatorInput(input),
		langfuse.WithEvaluatorMetadata(langfuse.Metadata{
			"lang": fp.language,
		}),
	)
	ctx, _ = getterEvaluator.Observation(ctx)

	titleTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeTaskDescriptor, map[string]any{
		"Input":       input,
		"Lang":        fp.language,
		"CurrentTime": getCurrentTime(),
		"N":           150,
	})
	if err != nil {
		return "", wrapErrorEndEvaluatorSpan(ctx, getterEvaluator, "failed to get flow title template", err)
	}

	title, err := fp.Call(ctx, pconfig.OptionsTypeSimple, titleTmpl)
	if err != nil {
		return "", wrapErrorEndEvaluatorSpan(ctx, getterEvaluator, "failed to get flow title", err)
	}
	title = normalizeTitle(title)

	getterEvaluator.End(
		langfuse.WithEvaluatorStatus("success"),
		langfuse.WithEvaluatorOutput(title),
	)

	return title, nil
}

func (fp *flowProvider) GenerateSubtasks(ctx context.Context, taskID int64) ([]tools.SubtaskInfo, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.GenerateSubtasks")
	defer span.End()

	logger := logrus.WithContext(ctx).WithField("task_id", taskID)

	tasksInfo, err := fp.getTasksInfo(ctx, taskID)
	if err != nil {
		logger.WithError(err).Error("failed to get tasks info")
		return nil, fmt.Errorf("failed to get tasks info: %w", err)
	}

	generatorContext := map[string]map[string]any{
		"user": {
			"Task":     tasksInfo.Task,
			"Tasks":    tasksInfo.Tasks,
			"Subtasks": tasksInfo.Subtasks,
		},
		"system": {
			"SubtaskListToolName":     tools.SubtaskListToolName,
			"TerminalToolName":        tools.TerminalToolName,
			"FileToolName":            tools.FileToolName,
			"BrowserToolName":         tools.BrowserToolName,
			"SummarizationToolName":   cast.SummarizationToolName,
			"SummarizedContentPrefix": strings.ReplaceAll(csum.SummarizedContentPrefix, "\n", "\\n"),
			"DockerImage":             fp.image,
			"Cwd":                     docker.WorkFolderPathInContainer,
			"Lang":                    fp.language,
			"CurrentTime":             getCurrentTime(),
			"N":                       TasksNumberLimit,
			"ToolPlaceholder":         ToolPlaceholder,
			"UserFiles":               fp.userFilesListing(),
		},
	}

	ctx, observation := obs.Observer.NewObservation(ctx)
	generatorEvaluator := observation.Evaluator(
		langfuse.WithEvaluatorName("subtasks generator"),
		langfuse.WithEvaluatorInput(tasksInfo),
		langfuse.WithEvaluatorMetadata(langfuse.Metadata{
			"user_context":   generatorContext["user"],
			"system_context": generatorContext["system"],
		}),
	)
	ctx, _ = generatorEvaluator.Observation(ctx)

	generatorTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeSubtasksGenerator, generatorContext["user"])
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, generatorEvaluator, "failed to get task generator template", err)
	}

	subtasksLen := len(tasksInfo.Subtasks)
	for l := subtasksLen; l > 2; l /= 2 {
		if len(generatorTmpl) < msgGeneratorSizeLimit {
			break
		}

		generatorContext["user"]["Subtasks"] = tasksInfo.Subtasks[(subtasksLen - l):]
		generatorTmpl, err = fp.prompter.RenderTemplate(templates.PromptTypeSubtasksGenerator, generatorContext["user"])
		if err != nil {
			return nil, wrapErrorEndEvaluatorSpan(ctx, generatorEvaluator, "failed to get task generator template", err)
		}
	}

	systemGeneratorTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeGenerator, generatorContext["system"])
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, generatorEvaluator, "failed to get task system generator template", err)
	}

	subtasks, err := fp.performSubtasksGenerator(ctx, taskID, systemGeneratorTmpl, generatorTmpl, tasksInfo.Task.Input)
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, generatorEvaluator, "failed to perform subtasks generator", err)
	}

	generatorEvaluator.End(
		langfuse.WithEvaluatorStatus("success"),
		langfuse.WithEvaluatorOutput(subtasks),
	)

	return subtasks, nil
}

func (fp *flowProvider) RefineSubtasks(ctx context.Context, taskID int64) ([]tools.SubtaskInfo, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.RefineSubtasks")
	defer span.End()

	logger := logrus.WithContext(ctx).WithField("task_id", taskID)

	tasksInfo, err := fp.getTasksInfo(ctx, taskID)
	if err != nil {
		logger.WithError(err).Error("failed to get tasks info")
		return nil, fmt.Errorf("failed to get tasks info: %w", err)
	}

	subtasksInfo := fp.getSubtasksInfo(taskID, tasksInfo.Subtasks)

	logger.WithFields(logrus.Fields{
		"planned_count":   len(subtasksInfo.Planned),
		"completed_count": len(subtasksInfo.Completed),
	}).Debug("retrieved subtasks info for refinement")

	refinerContext := map[string]map[string]any{
		"user": {
			"Task":              tasksInfo.Task,
			"Tasks":             tasksInfo.Tasks,
			"PlannedSubtasks":   subtasksInfo.Planned,
			"CompletedSubtasks": subtasksInfo.Completed,
		},
		"system": {
			"SubtaskPatchToolName":    tools.SubtaskPatchToolName,
			"SubtaskListToolName":     tools.SubtaskListToolName,
			"TerminalToolName":        tools.TerminalToolName,
			"FileToolName":            tools.FileToolName,
			"BrowserToolName":         tools.BrowserToolName,
			"SummarizationToolName":   cast.SummarizationToolName,
			"SummarizedContentPrefix": strings.ReplaceAll(csum.SummarizedContentPrefix, "\n", "\\n"),
			"DockerImage":             fp.image,
			"Cwd":                     docker.WorkFolderPathInContainer,
			"Lang":                    fp.language,
			"CurrentTime":             getCurrentTime(),
			"N":                       max(TasksNumberLimit-len(subtasksInfo.Completed), 0),
			"ToolPlaceholder":         ToolPlaceholder,
			"UserFiles":               fp.userFilesListing(),
		},
	}

	ctx, observation := obs.Observer.NewObservation(ctx)
	refinerEvaluator := observation.Evaluator(
		langfuse.WithEvaluatorName("subtasks refiner"),
		langfuse.WithEvaluatorInput(refinerContext),
		langfuse.WithEvaluatorMetadata(langfuse.Metadata{
			"user_context":   refinerContext["user"],
			"system_context": refinerContext["system"],
		}),
	)
	ctx, _ = refinerEvaluator.Observation(ctx)

	refinerTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeSubtasksRefiner, refinerContext["user"])
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to get task subtasks refiner template (1)", err)
	}

	// TODO: here need to store it in the database and use it as a cache for next runs
	if len(refinerTmpl) < msgRefinerSizeLimit {
		summarizerHandler := fp.GetSummarizeResultHandler(&taskID, nil)
		executionState, err := fp.getTaskPrimaryAgentChainSummary(ctx, taskID, summarizerHandler)
		if err != nil {
			return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to prepare execution state", err)
		}

		refinerContext["user"]["ExecutionState"] = executionState
		refinerTmpl, err = fp.prompter.RenderTemplate(templates.PromptTypeSubtasksRefiner, refinerContext["user"])
		if err != nil {
			return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to get task subtasks refiner template (2)", err)
		}

		if len(refinerTmpl) < msgRefinerSizeLimit {
			msgLogsSummary, err := fp.getTaskMsgLogsSummary(ctx, taskID, summarizerHandler)
			if err != nil {
				return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to get task msg logs summary", err)
			}

			refinerContext["user"]["ExecutionLogs"] = msgLogsSummary
			refinerTmpl, err = fp.prompter.RenderTemplate(templates.PromptTypeSubtasksRefiner, refinerContext["user"])
			if err != nil {
				return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to get task subtasks refiner template (3)", err)
			}
		}
	}

	systemRefinerTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeRefiner, refinerContext["system"])
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to get task system refiner template", err)
	}

	subtasks, err := fp.performSubtasksRefiner(ctx, taskID, subtasksInfo.Planned, systemRefinerTmpl, refinerTmpl, tasksInfo.Task.Input)
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, refinerEvaluator, "failed to perform subtasks refiner", err)
	}

	refinerEvaluator.End(
		langfuse.WithEvaluatorStatus("success"),
		langfuse.WithEvaluatorOutput(subtasks),
	)

	return subtasks, nil
}

func (fp *flowProvider) GetTaskResult(ctx context.Context, taskID int64) (*tools.TaskResult, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.GetTaskResult")
	defer span.End()

	logger := logrus.WithContext(ctx).WithField("task_id", taskID)

	tasksInfo, err := fp.getTasksInfo(ctx, taskID)
	if err != nil {
		logger.WithError(err).Error("failed to get tasks info")
		return nil, fmt.Errorf("failed to get tasks info: %w", err)
	}

	subtasksInfo := fp.getSubtasksInfo(taskID, tasksInfo.Subtasks)
	reporterContext := map[string]map[string]any{
		"user": {
			"Task":              tasksInfo.Task,
			"Tasks":             tasksInfo.Tasks,
			"CompletedSubtasks": subtasksInfo.Completed,
			"PlannedSubtasks":   subtasksInfo.Planned,
		},
		"system": {
			"ReportResultToolName":    tools.ReportResultToolName,
			"SummarizationToolName":   cast.SummarizationToolName,
			"SummarizedContentPrefix": strings.ReplaceAll(csum.SummarizedContentPrefix, "\n", "\\n"),
			"Cwd":                     docker.WorkFolderPathInContainer,
			"Lang":                    fp.language,
			"N":                       4000,
			"ToolPlaceholder":         ToolPlaceholder,
			"UserFiles":               fp.userFilesListing(),
		},
	}

	ctx, observation := obs.Observer.NewObservation(ctx)
	reporterEvaluator := observation.Evaluator(
		langfuse.WithEvaluatorName("reporter agent"),
		langfuse.WithEvaluatorInput(reporterContext),
		langfuse.WithEvaluatorMetadata(langfuse.Metadata{
			"user_context":   reporterContext["user"],
			"system_context": reporterContext["system"],
		}),
	)
	ctx, _ = reporterEvaluator.Observation(ctx)

	reporterTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeTaskReporter, reporterContext["user"])
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to get task reporter template (1)", err)
	}

	if len(reporterTmpl) < msgReporterSizeLimit {
		summarizerHandler := fp.GetSummarizeResultHandler(&taskID, nil)
		executionState, err := fp.getTaskPrimaryAgentChainSummary(ctx, taskID, summarizerHandler)
		if err != nil {
			return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to prepare execution state", err)
		}

		reporterContext["user"]["ExecutionState"] = executionState
		reporterTmpl, err = fp.prompter.RenderTemplate(templates.PromptTypeTaskReporter, reporterContext["user"])
		if err != nil {
			return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to get task reporter template (2)", err)
		}

		if len(reporterTmpl) < msgReporterSizeLimit {
			msgLogsSummary, err := fp.getTaskMsgLogsSummary(ctx, taskID, summarizerHandler)
			if err != nil {
				return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to get task msg logs summary", err)
			}

			reporterContext["user"]["ExecutionLogs"] = msgLogsSummary
			reporterTmpl, err = fp.prompter.RenderTemplate(templates.PromptTypeTaskReporter, reporterContext["user"])
			if err != nil {
				return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to get task reporter template (3)", err)
			}
		}
	}

	systemReporterTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeReporter, reporterContext["system"])
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to get task system reporter template", err)
	}

	result, err := fp.performTaskResultReporter(ctx, &taskID, nil, systemReporterTmpl, reporterTmpl, tasksInfo.Task.Input)
	if err != nil {
		return nil, wrapErrorEndEvaluatorSpan(ctx, reporterEvaluator, "failed to perform task result reporter", err)
	}

	reporterEvaluator.End(
		langfuse.WithEvaluatorStatus("success"),
		langfuse.WithEvaluatorOutput(result),
	)

	return result, nil
}

func (fp *flowProvider) PrepareAgentChain(ctx context.Context, taskID, subtaskID int64) (int64, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.PrepareAgentChain")
	defer span.End()

	msgChainType := database.MsgchainTypeCommander
	optAgentType := agentByChain[msgChainType]

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":   fp.Type(),
		"agent":      optAgentType,
		"flow_id":    fp.flowID,
		"task_id":    taskID,
		"subtask_id": subtaskID,
	})

	subtask, err := fp.db.GetSubtask(ctx, subtaskID)
	if err != nil {
		logger.WithError(err).Error("failed to get subtask")
		return 0, fmt.Errorf("failed to get subtask: %w", err)
	}

	executionContext, err := fp.prepareExecutionContext(ctx, taskID, subtaskID)
	if err != nil {
		logger.WithError(err).Error("failed to prepare execution context")
		return 0, fmt.Errorf("failed to prepare execution context: %w", err)
	}

	subtask, err = fp.db.UpdateSubtaskContext(ctx, database.UpdateSubtaskContextParams{
		Context: executionContext,
		ID:      subtaskID,
	})
	if err != nil {
		logger.WithError(err).Error("failed to update subtask context")
		return 0, fmt.Errorf("failed to update subtask context: %w", err)
	}

	optimizationMode := fp.subtaskMode(ctx, subtask) == tools.PlanModeOptimization

	systemAgentTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeCommander, map[string]any{
		"DiagnosisToolName":      tools.DiagnosisToolName,
		"AskToolName":            tools.AskUserToolName,
		"AskUserEnabled":         fp.askUser,
		"Unattended":             fp.unattended(),
		"TriageToolName":         tools.TriageToolName,
		"WatchToolName":          tools.OpsWatchToolName,
		"SandboxToolName":        tools.OpsSandboxToolName,
		"TraceToolName":          tools.OpsTraceToolName,
		"TraceEnabled":           fp.cfg.OpsSandboxBPF,
		"LogAnalystToolName":     tools.LogAnalystToolName,
		"MetricsAnalystToolName": tools.MetricsAnalystToolName,
		"ChangeAnalystToolName":  tools.ChangeAnalystToolName,
		"OptimizationToolName":   tools.OptimizationResultToolName,
		"OptimizationMode":       optimizationMode,
		"ExecutionContext":       executionContext,
		"Lang":                   fp.language,
	})
	if err != nil {
		logger.WithError(err).Error("failed to get system prompt for commander template")
		return 0, fmt.Errorf("failed to get system prompt for commander template: %w", err)
	}

	msgChainID, _, err := fp.restoreChain(
		ctx, &taskID, &subtaskID, optAgentType, msgChainType, systemAgentTmpl, subtask.Description,
	)
	if err != nil {
		logger.WithError(err).Error("failed to restore commander msg chain")
		return 0, fmt.Errorf("failed to restore commander msg chain: %w", err)
	}

	return msgChainID, nil
}

func (fp *flowProvider) PerformAgentChain(ctx context.Context, taskID, subtaskID, msgChainID int64) (PerformResult, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.PerformAgentChain")
	defer span.End()

	msgChainType := database.MsgchainTypeCommander
	optAgentType := agentByChain[msgChainType]

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     fp.Type(),
		"agent":        optAgentType,
		"flow_id":      fp.flowID,
		"task_id":      taskID,
		"subtask_id":   subtaskID,
		"msg_chain_id": msgChainID,
	})

	msgChain, err := fp.db.GetMsgChain(ctx, msgChainID)
	if err != nil {
		logger.WithError(err).Error("failed to get commander msg chain")
		return PerformResultError, fmt.Errorf("failed to get commander msg chain %d: %w", msgChainID, err)
	}

	var chain []llms.MessageContent
	if err := json.Unmarshal(msgChain.Chain, &chain); err != nil {
		logger.WithError(err).Error("failed to unmarshal commander msg chain")
		return PerformResultError, fmt.Errorf("failed to unmarshal commander msg chain %d: %w", msgChainID, err)
	}

	subtask, err := fp.db.GetSubtask(ctx, subtaskID)
	if err != nil {
		logger.WithError(err).Error("failed to get subtask")
		return PerformResultError, fmt.Errorf("failed to get subtask: %w", err)
	}

	ctx, observation := obs.Observer.NewObservation(ctx)
	executorAgent := observation.Agent(
		langfuse.WithAgentName(fmt.Sprintf("commander for subtask %d: %s", subtaskID, subtask.Title)),
		langfuse.WithAgentInput(chain),
		langfuse.WithAgentMetadata(langfuse.Metadata{
			"flow_id":      fp.flowID,
			"task_id":      taskID,
			"subtask_id":   subtaskID,
			"msg_chain_id": msgChainID,
			"provider":     fp.Type(),
			"image":        fp.image,
			"lang":         fp.language,
			"description":  subtask.Description,
		}),
	)
	ctx, _ = executorAgent.Observation(ctx)

	performResult := PerformResultError
	// Both exits go through one closure: diagnosis ends the chain with the
	// conclusion, ask hands control back to a human. Anything else reaching here
	// is a programming error, not a model error.
	barrier := func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		loggerFunc := logger.WithContext(ctx).WithFields(logrus.Fields{
			"name": name,
			"args": string(args),
		})

		// finish records an accepted conclusion: it becomes the subtask result and
		// the report message, which is the only place the evidence survives once
		// the chain is compacted.
		finish := func(report, status string) (string, error) {
			performResult = PerformResultDone

			// TODO: here need to call SetResult from SubtaskWorker interface
			subtask, err = fp.db.UpdateSubtaskResult(ctx, database.UpdateSubtaskResultParams{
				Result: report,
				ID:     subtaskID,
			})
			if err != nil {
				executorAgent.End(
					langfuse.WithAgentStatus(err.Error()),
					langfuse.WithAgentLevel(langfuse.ObservationLevelError),
				)
				loggerFunc.WithError(err).Error("failed to update subtask result")
				return "", fmt.Errorf("failed to update subtask %d result: %w", subtaskID, err)
			}

			// report result to msg log as a final message for the subtask execution
			reportMsgID, err := fp.putMsgLog(
				ctx,
				database.MsglogTypeReport,
				&taskID, &subtaskID, 0,
				"", subtask.Description,
			)
			if err != nil {
				executorAgent.End(
					langfuse.WithAgentStatus(err.Error()),
					langfuse.WithAgentLevel(langfuse.ObservationLevelError),
				)
				loggerFunc.WithError(err).Error("failed to put report msg")
				return "", fmt.Errorf("failed to put report msg: %w", err)
			}

			if err := fp.updateMsgLogResult(
				ctx, reportMsgID, 0, report, database.MsglogResultFormatMarkdown,
			); err != nil {
				executorAgent.End(
					langfuse.WithAgentStatus(err.Error()),
					langfuse.WithAgentLevel(langfuse.ObservationLevelError),
				)
				loggerFunc.WithError(err).Error("failed to update report msg result")
				return "", fmt.Errorf("failed to update report msg result: %w", err)
			}

			executorAgent.End(
				langfuse.WithAgentOutput(report),
				langfuse.WithAgentStatus(status),
			)

			return fmt.Sprintf("function %s successfully processed arguments", name), nil
		}

		switch name {
		case tools.DiagnosisToolName:
			var diagnosis tools.Diagnosis
			if err := json.Unmarshal(args, &diagnosis); err != nil {
				loggerFunc.WithError(err).Error("failed to unmarshal diagnosis")
				return "", fmt.Errorf("failed to unmarshal diagnosis: %w", err)
			}
			if err := diagnosis.Validate(); err != nil {
				loggerFunc.WithError(err).Warn("diagnosis is incomplete")
				return "", fmt.Errorf("诊断不完整: %w", err)
			}

			report, err := diagnosis.Render(fp.language)
			if err != nil {
				loggerFunc.WithError(err).Error("failed to render diagnosis")
				return "", fmt.Errorf("failed to render diagnosis: %w", err)
			}

			return finish(report, "diagnosis: accepted")

		case tools.OptimizationResultToolName:
			var result tools.OptimizationResult
			if err := json.Unmarshal(args, &result); err != nil {
				loggerFunc.WithError(err).Error("failed to unmarshal optimization result")
				return "", fmt.Errorf("failed to unmarshal optimization result: %w", err)
			}
			if err := result.ValidateAgainst(fp.performanceGoal(ctx, subtask.TaskID)); err != nil {
				loggerFunc.WithError(err).Warn("optimization result is incomplete")
				return "", fmt.Errorf("优化结论不完整: %w", err)
			}

			report, err := result.Render(fp.language)
			if err != nil {
				loggerFunc.WithError(err).Error("failed to render optimization result")
				return "", fmt.Errorf("failed to render optimization result: %w", err)
			}

			return finish(report, "optimization: accepted")

		case tools.AskUserToolName:
			if answer, park := fp.askGateAction(); !park {
				return answer, nil
			}

			performResult = PerformResultWaiting

			var askUser tools.AskUser
			if err := json.Unmarshal(args, &askUser); err != nil {
				loggerFunc.WithError(err).Error("failed to unmarshal ask user result")
				return "", fmt.Errorf("failed to unmarshal ask user result: %w", err)
			}

			executorAgent.End(
				langfuse.WithAgentOutput(askUser.Message),
				langfuse.WithAgentStatus("ask user handler"),
			)
		}

		return fmt.Sprintf("function %s successfully processed arguments", name), nil
	}

	// The exit follows the plan entry's mode, not the model's mood: a
	// performance task has no diagnosis tool and an incident has no
	// optimisation tool, so the wrong conclusion cannot be filed.
	optimizationMode := fp.subtaskMode(ctx, subtask) == tools.PlanModeOptimization

	cfg := tools.CommanderExecutorConfig{
		TaskID:     taskID,
		SubtaskID:  subtaskID,
		Summarizer: fp.GetSummarizeResultHandler(&taskID, &subtaskID),
		Capability: fp.capabilityGuard(ctx, subtaskID, "commander"),
	}
	if optimizationMode {
		cfg.Optimization = barrier
	} else {
		cfg.Diagnosis = barrier
	}
	if fp.askUser {
		cfg.AskUser = barrier
		cfg.AskUserUnattended = fp.unattended()
	}

	// Delegation is recursion: each handler runs a full specialist chain whose
	// result comes back as the tool's return value, so the commander keeps one
	// transcript and the specialist keeps its own.
	for _, d := range []struct {
		name    string
		handler func(context.Context, *int64, *int64) (tools.ExecutorHandler, error)
		assign  func(tools.ExecutorHandler)
	}{
		{tools.TriageToolName, fp.GetTriageHandler, func(h tools.ExecutorHandler) { cfg.Triage = h }},
		{tools.LogAnalystToolName, fp.GetLogAnalystHandler, func(h tools.ExecutorHandler) { cfg.LogAnalyst = h }},
		{tools.MetricsAnalystToolName, fp.GetMetricsAnalystHandler, func(h tools.ExecutorHandler) { cfg.MetricsAnalyst = h }},
		{tools.ChangeAnalystToolName, fp.GetChangeAnalystHandler, func(h tools.ExecutorHandler) { cfg.ChangeAnalyst = h }},
	} {
		handler, err := d.handler(ctx, &taskID, &subtaskID)
		if err != nil {
			logger.WithError(err).WithField("role", d.name).Warn("delegation handler is unavailable")
			continue
		}
		d.assign(handler)
	}

	executor, err := fp.executor.GetCommanderExecutor(cfg)
	if err != nil {
		return PerformResultError, wrapErrorEndAgentSpan(ctx, executorAgent, "failed to get commander executor", err)
	}

	ctx = tools.PutAgentContext(ctx, msgChainType)
	err = fp.performAgentChain(
		ctx, optAgentType, msgChain.ID, &taskID, &subtaskID, chain, executor, fp.summarizer,
	)
	if err != nil {
		return PerformResultError, wrapErrorEndAgentSpan(ctx, executorAgent, "failed to perform commander chain", err)
	}

	executorAgent.End()

	return performResult, nil
}

func (fp *flowProvider) PutInputToAgentChain(ctx context.Context, msgChainID int64, input string) error {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.PutInputToAgentChain")
	defer span.End()

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     fp.Type(),
		"flow_id":      fp.flowID,
		"msg_chain_id": msgChainID,
		"input":        input[:min(len(input), 1000)],
	})

	return fp.processChain(ctx, pconfig.OptionsTypeCommander, msgChainID, logger,
		func(chain []llms.MessageContent) ([]llms.MessageContent, error) {
			return fp.updateMsgChainResult(chain, tools.AskUserToolName, input)
		},
	)
}

// EnsureChainConsistency ensures a message chain is in a consistent state by adding
// default responses to any unresponded tool calls.
func (fp *flowProvider) EnsureChainConsistency(ctx context.Context, msgChainID int64) error {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.EnsureChainConsistency")
	defer span.End()

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"provider":     fp.Type(),
		"flow_id":      fp.flowID,
		"msg_chain_id": msgChainID,
	})

	return fp.processChain(ctx, pconfig.OptionsTypeCommander, msgChainID, logger,
		func(chain []llms.MessageContent) ([]llms.MessageContent, error) {
			return fp.ensureChainConsistency(chain)
		},
	)
}

func (fp *flowProvider) putMsgLog(
	ctx context.Context,
	msgType database.MsglogType,
	taskID, subtaskID *int64,
	streamID int64,
	thinking, msg string,
) (int64, error) {
	fp.mx.RLock()
	msgLog := fp.msgLog
	fp.mx.RUnlock()

	if msgLog == nil {
		return 0, nil
	}

	return msgLog.PutMsg(ctx, msgType, taskID, subtaskID, streamID, thinking, msg)
}

func (fp *flowProvider) updateMsgLogResult(
	ctx context.Context,
	msgID, streamID int64,
	result string,
	resultFormat database.MsglogResultFormat,
) error {
	fp.mx.RLock()
	msgLog := fp.msgLog
	fp.mx.RUnlock()

	if msgLog == nil || msgID <= 0 {
		return nil
	}

	return msgLog.UpdateMsgResult(ctx, msgID, streamID, result, resultFormat)
}

func (fp *flowProvider) putAgentLog(
	ctx context.Context,
	initiator, executor database.MsgchainType,
	task, result string,
	taskID, subtaskID *int64,
) (int64, error) {
	fp.mx.RLock()
	agentLog := fp.agentLog
	fp.mx.RUnlock()

	if agentLog == nil {
		return 0, nil
	}

	return agentLog.PutLog(ctx, initiator, executor, task, result, taskID, subtaskID)
}

// subtaskMode resolves which chain a subtask belongs to. An explicit flow type
// outranks the plan's own mode: someone who asked for a performance
// investigation should not be handed an incident conclusion because the planner
// guessed otherwise.
func (fp *flowProvider) subtaskMode(ctx context.Context, subtask database.Subtask) string {
	task, err := fp.db.GetTask(ctx, subtask.TaskID)
	if err != nil {
		return subtaskPlanMode(subtask)
	}

	flow, err := fp.db.GetFlow(ctx, task.FlowID)
	if err != nil || flow.FlowType == "" || flow.FlowType == tools.FlowTypeAuto {
		return subtaskPlanMode(subtask)
	}

	return flow.FlowType
}

// subtaskPlanMode falls back to the label older rows carry in their description,
// because a row written before the column existed has no other record of it.
func subtaskPlanMode(subtask database.Subtask) string {
	if subtask.PlanMode != "" {
		return subtask.PlanMode
	}

	return tools.PlanModeOf(subtask.Description)
}

// performanceGoal reads the goal the user declared when the flow was created.
// The first task of a flow carries the user's own message, which is where the
// 【性能目标】 block lives; a goal that cannot be read leaves every check skipped
// rather than failing the chain on a database hiccup.
func (fp *flowProvider) performanceGoal(ctx context.Context, taskID int64) tools.PerformanceGoal {
	task, err := fp.db.GetTask(ctx, taskID)
	if err != nil {
		return tools.PerformanceGoal{}
	}

	tasks, err := fp.db.GetFlowTasks(ctx, task.FlowID)
	if err != nil || len(tasks) == 0 {
		return tools.PerformanceGoal{}
	}

	return tools.ParsePerformanceGoal(tasks[0].Input)
}
