package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"opsagi/pkg/database"
	obs "opsagi/pkg/observability"
	"opsagi/pkg/templates"
	"opsagi/pkg/tools"

	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/langchaingo/llms"
)

// analystChainType maps a delegation tool onto the chain it runs. The chain type
// is what the record uses to tell one specialist's work from another's.
var analystChainType = map[string]database.MsgchainType{
	tools.TriageToolName:         database.MsgchainTypeTriage,
	tools.LogAnalystToolName:     database.MsgchainTypeLogAnalyst,
	tools.MetricsAnalystToolName: database.MsgchainTypeMetricsAnalyst,
	tools.ChangeAnalystToolName:  database.MsgchainTypeChangeAnalyst,
}

var analystPromptType = map[string]templates.PromptType{
	tools.TriageToolName:         templates.PromptTypeTriage,
	tools.LogAnalystToolName:     templates.PromptTypeLogAnalyst,
	tools.MetricsAnalystToolName: templates.PromptTypeMetricsAnalyst,
	tools.ChangeAnalystToolName:  templates.PromptTypeChangeAnalyst,
}
// GetTriageHandler and its three siblings are the delegation surface of the
// commander: each runs its specialist's chain under its own prompt and tools,
// and returns that specialist's finding as the tool result.
func (fp *flowProvider) GetTriageHandler(ctx context.Context, taskID, subtaskID *int64) (tools.ExecutorHandler, error) {
	return fp.getAnalystHandler(ctx, tools.TriageToolName, taskID, subtaskID)
}

func (fp *flowProvider) GetLogAnalystHandler(ctx context.Context, taskID, subtaskID *int64) (tools.ExecutorHandler, error) {
	return fp.getAnalystHandler(ctx, tools.LogAnalystToolName, taskID, subtaskID)
}

func (fp *flowProvider) GetMetricsAnalystHandler(ctx context.Context, taskID, subtaskID *int64) (tools.ExecutorHandler, error) {
	return fp.getAnalystHandler(ctx, tools.MetricsAnalystToolName, taskID, subtaskID)
}

func (fp *flowProvider) GetChangeAnalystHandler(ctx context.Context, taskID, subtaskID *int64) (tools.ExecutorHandler, error) {
	return fp.getAnalystHandler(ctx, tools.ChangeAnalystToolName, taskID, subtaskID)
}

func (fp *flowProvider) getAnalystHandler(
	ctx context.Context,
	role string,
	taskID, subtaskID *int64,
) (tools.ExecutorHandler, error) {
	promptType, ok := analystPromptType[role]
	if !ok {
		return nil, fmt.Errorf("unknown analyst role %q", role)
	}

	executionContext, err := fp.getExecutionContext(ctx, taskID, subtaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution context: %w", err)
	}

	return func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider."+role)
		defer span.End()

		var question tools.AnalystQuestion
		if err := json.Unmarshal(args, &question); err != nil {
			return "", fmt.Errorf("failed to unmarshal %s question: %w", role, err)
		}
		if strings.TrimSpace(question.Question) == "" {
			return "", fmt.Errorf("question 为空：写清要它验什么，带上告警原文、受影响负载与时间窗")
		}

		systemTmpl, err := fp.prompter.RenderTemplate(promptType, map[string]any{
			"Lang": fp.language,
		})
		if err != nil {
			return "", fmt.Errorf("failed to render %s prompt: %w", role, err)
		}

		if executionContext != "" {
			systemTmpl = systemTmpl + "\n\n<context>\n" + executionContext + "\n</context>\n"
		}

		finding, err := fp.performAnalyst(ctx, role, systemTmpl, question, taskID, subtaskID)
		if err != nil {
			return "", err
		}

		return finding, nil
	}, nil
}

func (fp *flowProvider) performAnalyst(
	ctx context.Context,
	role, systemTmpl string,
	question tools.AnalystQuestion,
	taskID, subtaskID *int64,
) (string, error) {
	msgChainType, ok := analystChainType[role]
	if !ok {
		return "", fmt.Errorf("unknown analyst role %q", role)
	}

	optAgentType := agentByChain[msgChainType]

	var finding string

	ctx = tools.PutAgentContext(ctx, msgChainType)
	cfg := tools.AnalystExecutorConfig{
		TaskID:    taskID,
		SubtaskID: subtaskID,
		Role:      role,
		Finding: func(ctx context.Context, name string, args json.RawMessage) (string, error) {
			rendered, err := renderAnalystFinding(role, fp.language, args)
			if err != nil {
				return "", err
			}
			finding = rendered
			return "finding accepted", nil
		},
		Summarizer: fp.GetSummarizeResultHandler(taskID, subtaskID),
	}
	if subtaskID != nil {
		cfg.Capability = fp.capabilityGuard(ctx, *subtaskID, role)
	}
	executor, err := fp.executor.GetAnalystExecutor(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to get %s executor: %w", role, err)
	}

	humanMessage := question.Question
	if question.Context != "" {
		humanMessage += "\n\n已经确认的事实（不要重复取证）：\n" + question.Context
	}

	chain := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemTmpl),
		llms.TextParts(llms.ChatMessageTypeHuman, humanMessage),
	}

	chainBlob, err := json.Marshal(chain)
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s chain: %w", role, err)
	}

	msgChain, err := fp.db.CreateMsgChain(ctx, database.CreateMsgChainParams{
		Type:          msgChainType,
		Model:         fp.Model(optAgentType),
		ModelProvider: string(fp.Type()),
		Chain:         chainBlob,
		FlowID:        fp.flowID,
		TaskID:        database.Int64ToNullInt64(taskID),
		SubtaskID:     database.Int64ToNullInt64(subtaskID),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create %s chain: %w", role, err)
	}

	err = fp.performAgentChain(ctx, optAgentType, msgChain.ID, taskID, subtaskID, chain, executor, fp.summarizer)
	if err != nil {
		return "", fmt.Errorf("failed to get %s result: %w", role, err)
	}

	if agentCtx, ok := tools.GetAgentContext(ctx); ok {
		_, _ = fp.putAgentLog(
			ctx,
			agentCtx.ParentAgentType,
			agentCtx.CurrentAgentType,
			question.Question,
			finding,
			taskID,
			subtaskID,
		)
	}

	if strings.TrimSpace(finding) == "" {
		logrus.WithContext(ctx).WithField("role", role).Warn("analyst chain ended without a finding")
		return "", fmt.Errorf("%s 没有交回结论：必须用它的出口工具提交", role)
	}

	return finding, nil
}

// renderAnalystFinding validates a finding before it is accepted, so an
// incomplete report comes back to the specialist as a refusal it can fix rather
// than reaching the commander as a half-answer.
func renderAnalystFinding(role, lang string, args json.RawMessage) (string, error) {
	switch role {
	case tools.TriageToolName:
		var r tools.TriageResult
		if err := json.Unmarshal(args, &r); err != nil {
			return "", fmt.Errorf("failed to unmarshal triage result: %w", err)
		}
		return r.Render(lang)
	case tools.LogAnalystToolName:
		var f tools.LogFinding
		if err := json.Unmarshal(args, &f); err != nil {
			return "", fmt.Errorf("failed to unmarshal log finding: %w", err)
		}
		return f.Render(lang)
	case tools.MetricsAnalystToolName:
		var f tools.MetricFinding
		if err := json.Unmarshal(args, &f); err != nil {
			return "", fmt.Errorf("failed to unmarshal metric finding: %w", err)
		}
		return f.Render(lang)
	case tools.ChangeAnalystToolName:
		var f tools.ChangeFinding
		if err := json.Unmarshal(args, &f); err != nil {
			return "", fmt.Errorf("failed to unmarshal change finding: %w", err)
		}
		return f.Render(lang)
	default:
		return "", fmt.Errorf("unknown analyst role %q", role)
	}
}
