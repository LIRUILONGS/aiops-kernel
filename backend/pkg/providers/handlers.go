package providers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"opsagi/pkg/csum"
	"opsagi/pkg/database"
	obs "opsagi/pkg/observability"
	"opsagi/pkg/observability/langfuse"
	"opsagi/pkg/schema"
	"opsagi/pkg/templates"
	"opsagi/pkg/tools"

	"github.com/sirupsen/logrus"
)
func wrapErrorEndAgentSpan(ctx context.Context, span langfuse.Agent, msg string, err error) error {
	obs.LogErrorOrCancel(logrus.WithContext(ctx), err, msg)
	err = fmt.Errorf("%s: %w", msg, err)

	level := langfuse.ObservationLevelError
	if errors.Is(err, context.Canceled) {
		level = langfuse.ObservationLevelWarning
	}
	span.End(
		langfuse.WithAgentStatus(err.Error()),
		langfuse.WithAgentLevel(level),
	)
	return err
}

func wrapErrorEndEvaluatorSpan(ctx context.Context, span langfuse.Evaluator, msg string, err error) error {
	obs.LogErrorOrCancel(logrus.WithContext(ctx), err, msg)
	err = fmt.Errorf("%s: %w", msg, err)

	level := langfuse.ObservationLevelError
	if errors.Is(err, context.Canceled) {
		level = langfuse.ObservationLevelWarning
	}
	span.End(
		langfuse.WithEvaluatorStatus(err.Error()),
		langfuse.WithEvaluatorLevel(level),
	)
	return err
}







func (fp *flowProvider) GetSummarizeResultHandler(taskID, subtaskID *int64) tools.SummarizeHandler {
	return func(ctx context.Context, result string) (string, error) {
		ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.getSummarizeResultHandler")
		defer span.End()

		// Return cached summary for identical input without calling the LLM.
		cacheKey := sha256.Sum256([]byte(result))
		if fp.summarizerCache != nil {
			if cached, ok := fp.summarizerCache.Get(cacheKey); ok {
				return cached, nil
			}
		}

		ctx, observation := obs.Observer.NewObservation(ctx)
		summarizerAgent := observation.Agent(
			langfuse.WithAgentName("chain summarizer"),
			langfuse.WithAgentInput(result),
			langfuse.WithAgentMetadata(langfuse.Metadata{
				"task_id":    taskID,
				"subtask_id": subtaskID,
				"lang":       fp.language,
			}),
		)
		ctx, _ = summarizerAgent.Observation(ctx)

		systemSummarizerTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeSummarizer, map[string]any{
			"TaskID":                  taskID,
			"SubtaskID":               subtaskID,
			"CurrentTime":             getCurrentTime(),
			"SummarizedContentPrefix": strings.ReplaceAll(csum.SummarizedContentPrefix, "\n", "\\n"),
		})
		if err != nil {
			return "", wrapErrorEndAgentSpan(ctx, summarizerAgent, "failed to get summarizer template", err)
		}

		// TODO: here need to summarize result by chunks in iterations
		if len(result) > 2*msgSummarizerLimit {
			result = database.SanitizeUTF8(
				result[:msgSummarizerLimit] +
					"\n\n{TRUNCATED}...\n\n" +
					result[len(result)-msgSummarizerLimit:],
			)
		}

		msgChainType := database.MsgchainTypeSummarizer
		opt := agentByChain[msgChainType]
		summary, err := fp.performSimpleChain(ctx, taskID, subtaskID, opt, msgChainType, systemSummarizerTmpl, result)
		if err != nil {
			return "", wrapErrorEndAgentSpan(ctx, summarizerAgent, "failed to get summary", err)
		}

		summary = database.SanitizeUTF8(summary)
		summarizerAgent.End(
			langfuse.WithAgentStatus("success"),
			langfuse.WithAgentOutput(summary),
			langfuse.WithAgentLevel(langfuse.ObservationLevelDebug),
		)

		if fp.summarizerCache != nil {
			fp.summarizerCache.Add(cacheKey, summary)
		}

		return summary, nil
	}
}

func (fp *flowProvider) fixToolCallArgs(
	ctx context.Context,
	funcName string,
	funcArgs json.RawMessage,
	funcSchema *schema.Schema,
	funcExecErr error,
) (json.RawMessage, error) {
	ctx, span := obs.Observer.NewSpan(ctx, obs.SpanKindInternal, "providers.flowProvider.fixToolCallArgsHandler")
	defer span.End()

	funcJsonSchema, err := json.Marshal(funcSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool call schema: %w", err)
	}

	ctx, observation := obs.Observer.NewObservation(ctx)
	toolCallFixerAgent := observation.Agent(
		langfuse.WithAgentName("tool call fixer"),
		langfuse.WithAgentInput(string(funcArgs)),
		langfuse.WithAgentMetadata(langfuse.Metadata{
			"func_name":     funcName,
			"func_schema":   string(funcJsonSchema),
			"func_exec_err": funcExecErr.Error(),
		}),
	)
	ctx, _ = toolCallFixerAgent.Observation(ctx)

	userToolCallFixerTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeInputToolCallFixer, map[string]any{
		"ToolCallName":   funcName,
		"ToolCallArgs":   string(funcArgs),
		"ToolCallSchema": string(funcJsonSchema),
		"ToolCallError":  funcExecErr.Error(),
	})
	if err != nil {
		return nil, wrapErrorEndAgentSpan(ctx, toolCallFixerAgent, "failed to get user tool call fixer template", err)
	}

	systemToolCallFixerTmpl, err := fp.prompter.RenderTemplate(templates.PromptTypeToolCallFixer, map[string]any{})
	if err != nil {
		return nil, wrapErrorEndAgentSpan(ctx, toolCallFixerAgent, "failed to get system tool call fixer template", err)
	}

	msgChainType := database.MsgchainTypeToolCallFixer
	opt := agentByChain[msgChainType]
	toolCallFixerResult, err := fp.performSimpleChain(ctx, nil, nil, opt, msgChainType, systemToolCallFixerTmpl, userToolCallFixerTmpl)
	if err != nil {
		return nil, wrapErrorEndAgentSpan(ctx, toolCallFixerAgent, "failed to get tool call fixer result", err)
	}

	toolCallFixerAgent.End(
		langfuse.WithAgentStatus("success"),
		langfuse.WithAgentOutput(toolCallFixerResult),
		langfuse.WithAgentLevel(langfuse.ObservationLevelDebug),
	)

	return json.RawMessage(toolCallFixerResult), nil
}
