package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"opsagi/pkg/cast"
	"opsagi/pkg/database"
	obs "opsagi/pkg/observability"
	"opsagi/pkg/providers/pconfig"
	"opsagi/pkg/tools"

	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/langchaingo/llms"
	"github.com/vxcontrol/langchaingo/llms/reasoning"
)

func (fp *flowProvider) performTaskResultReporter(
	ctx context.Context,
	taskID, subtaskID *int64,
	systemReporterTmpl, userReporterTmpl, input string,
) (*tools.TaskResult, error) {
	var (
		taskResult   tools.TaskResult
		msgChainType = database.MsgchainTypeReporter
		optAgentType = agentByChain[msgChainType]
	)

	chain := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemReporterTmpl),
		llms.TextParts(llms.ChatMessageTypeHuman, userReporterTmpl),
	}

	ctx = tools.PutAgentContext(ctx, msgChainType)
	cfg := tools.ReporterExecutorConfig{
		TaskID:    taskID,
		SubtaskID: subtaskID,
		ReportResult: func(ctx context.Context, name string, args json.RawMessage) (string, error) {
			err := json.Unmarshal(args, &taskResult)
			if err != nil {
				return "", fmt.Errorf("failed to unmarshal task result: %w", err)
			}
			if err := taskResult.Validate(); err != nil {
				return "", fmt.Errorf("failed to unmarshal task result: %w", err)
			}
			return "report result successfully processed", nil
		},
	}
	executor, err := fp.executor.GetReporterExecutor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get reporter executor: %w", err)
	}

	chainBlob, err := json.Marshal(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal msg chain: %w", err)
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
		return nil, fmt.Errorf("failed to create msg chain: %w", err)
	}

	err = fp.performAgentChain(ctx, optAgentType, msgChain.ID, taskID, subtaskID, chain, executor, fp.summarizer)
	if err != nil {
		return nil, fmt.Errorf("failed to get task reporter result: %w", err)
	}

	if agentCtx, ok := tools.GetAgentContext(ctx); ok {
		_, _ = fp.putAgentLog(
			ctx,
			agentCtx.ParentAgentType,
			agentCtx.CurrentAgentType,
			input,
			taskResult.Result,
			taskID,
			subtaskID,
		)
	}

	return &taskResult, nil
}

func (fp *flowProvider) performSubtasksGenerator(
	ctx context.Context,
	taskID int64,
	systemGeneratorTmpl, userGeneratorTmpl, input string,
) ([]tools.SubtaskInfo, error) {
	var (
		subtaskList  tools.SubtaskList
		msgChainType = database.MsgchainTypeGenerator
		optAgentType = agentByChain[msgChainType]
	)

	chain := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemGeneratorTmpl),
		llms.TextParts(llms.ChatMessageTypeHuman, userGeneratorTmpl),
	}

	ctx = tools.PutAgentContext(ctx, msgChainType)
	cfg := tools.GeneratorExecutorConfig{
		TaskID: taskID,
		SubtaskList: func(ctx context.Context, name string, args json.RawMessage) (string, error) {
			decoded, err := decodeGeneratorSubtaskList(args)
			if err != nil {
				return "", err
			}
			subtaskList = decoded
			return "subtask list successfully processed", nil
		},
	}
	executor, err := fp.executor.GetGeneratorExecutor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get generator executor: %w", err)
	}

	chainBlob, err := json.Marshal(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal msg chain: %w", err)
	}

	msgChain, err := fp.db.CreateMsgChain(ctx, database.CreateMsgChainParams{
		Type:          msgChainType,
		Model:         fp.Model(optAgentType),
		ModelProvider: string(fp.Type()),
		Chain:         chainBlob,
		FlowID:        fp.flowID,
		TaskID:        database.Int64ToNullInt64(&taskID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create msg chain: %w", err)
	}

	err = fp.performAgentChain(ctx, optAgentType, msgChain.ID, &taskID, nil, chain, executor, fp.summarizer)
	if err != nil {
		return nil, fmt.Errorf("failed to get subtasks generator result: %w", err)
	}

	if agentCtx, ok := tools.GetAgentContext(ctx); ok {
		_, _ = fp.putAgentLog(
			ctx,
			agentCtx.ParentAgentType,
			agentCtx.CurrentAgentType,
			input,
			fp.subtasksToMarkdown(subtaskList.Subtasks),
			&taskID,
			nil,
		)
	}

	return subtaskList.Subtasks, nil
}

func decodeGeneratorSubtaskList(args json.RawMessage) (tools.SubtaskList, error) {
	var list tools.SubtaskList
	if err := json.Unmarshal(args, &list); err != nil {
		return list, fmt.Errorf("failed to unmarshal subtask list: %w", err)
	}
	if list.Subtasks == nil {
		return list, fmt.Errorf("failed to unmarshal subtask list: subtasks array is missing")
	}
	if err := list.Validate(); err != nil {
		return list, fmt.Errorf("计划不完整: %w", err)
	}
	return list, nil
}

func (fp *flowProvider) performSubtasksRefiner(
	ctx context.Context,
	taskID int64,
	plannedSubtasks []database.Subtask,
	systemRefinerTmpl, userRefinerTmpl, input string,
) ([]tools.SubtaskInfo, error) {
	var (
		subtaskPatch tools.SubtaskPatch
		chain        []llms.MessageContent
		msgChainType = database.MsgchainTypeRefiner
		optAgentType = agentByChain[msgChainType]
	)

	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"task_id":        taskID,
		"planned_count":  len(plannedSubtasks),
		"msg_chain_type": msgChainType,
		"opt_agent_type": optAgentType,
	})

	logger.Debug("starting subtasks refiner")

	startTime := time.Now()

	restoreChain := func(msgChain json.RawMessage) ([]llms.MessageContent, error) {
		var msgList []llms.MessageContent
		err := json.Unmarshal(msgChain, &msgList)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal msg chain: %w", err)
		}

		ast, err := cast.NewChainAST(msgList, true)
		if err != nil {
			return nil, fmt.Errorf("failed to create refiner chain ast: %w", err)
		}

		if len(ast.Sections) == 0 {
			return nil, fmt.Errorf("failed to get sections from refiner chain ast")
		}

		systemSection := ast.Sections[0] // there may be multiple sections due to reflector agent
		systemMessage := llms.TextParts(llms.ChatMessageTypeSystem, systemRefinerTmpl)
		systemSection.Header.SystemMessage = &systemMessage

		// remove the last report with subtasks list/patch
		for idx := len(systemSection.Body) - 1; idx >= 0; idx-- {
			if systemSection.Body[idx].Type == cast.RequestResponse {
				systemSection.Body = systemSection.Body[:idx]
				break
			}
		}

		// build human message with tool calls history
		// we combine the history into single part for better LLMs compatibility
		toolCalls := extractToolCallsFromChain(systemSection.Messages())
		toolCallsHistory := extractHistoryFromHumanMessage(systemSection.Header.HumanMessage)
		combinedToolCallsHistory := appendNewToolCallsToHistory(toolCallsHistory, toolCalls)
		combinedUserRefinerTmpl := combineHistoryToolCallsToHumanMessage(combinedToolCallsHistory, userRefinerTmpl)
		humanMessage := llms.TextParts(llms.ChatMessageTypeHuman, combinedUserRefinerTmpl)
		systemSection.Header.HumanMessage = &humanMessage

		// reset messages in the chain, it's already saved in the header
		systemSection.Body = []*cast.BodyPair{}

		// restore the chain
		return systemSection.Messages(), nil
	}

	msgChain, err := fp.db.GetFlowTaskTypeLastMsgChain(ctx, database.GetFlowTaskTypeLastMsgChainParams{
		FlowID: fp.flowID,
		TaskID: database.Int64ToNullInt64(&taskID),
		Type:   msgChainType,
	})
	if err != nil || isEmptyChain(msgChain.Chain) {
		// fallback to generator chain if refiner chain is not found or empty
		msgChain, err = fp.db.GetFlowTaskTypeLastMsgChain(ctx, database.GetFlowTaskTypeLastMsgChainParams{
			FlowID: fp.flowID,
			TaskID: database.Int64ToNullInt64(&taskID),
			Type:   database.MsgchainTypeGenerator,
		})
		if err != nil || isEmptyChain(msgChain.Chain) {
			// is unexpected, but we should fallback to empty chain
			chain = []llms.MessageContent{
				llms.TextParts(llms.ChatMessageTypeSystem, systemRefinerTmpl),
				llms.TextParts(llms.ChatMessageTypeHuman, userRefinerTmpl),
			}
		} else {
			if chain, err = restoreChain(msgChain.Chain); err != nil {
				return nil, fmt.Errorf("failed to restore chain from generator state: %w", err)
			}
		}
	} else {
		if chain, err = restoreChain(msgChain.Chain); err != nil {
			return nil, fmt.Errorf("failed to restore chain from refiner state: %w", err)
		}
	}

	ctx = tools.PutAgentContext(ctx, msgChainType)
	cfg := tools.RefinerExecutorConfig{
		TaskID: taskID,
		SubtaskPatch: func(ctx context.Context, name string, args json.RawMessage) (string, error) {
			logger.WithField("args_len", len(args)).Debug("received subtask patch")
			if err := json.Unmarshal(args, &subtaskPatch); err != nil {
				logger.WithError(err).Error("failed to unmarshal subtask patch")
				return "", fmt.Errorf("failed to unmarshal subtask patch: %w", err)
			}
			if err := subtaskPatch.Validate(); err != nil {
				logger.WithError(err).Error("invalid subtask patch")
				return "", fmt.Errorf("invalid subtask patch: %w", err)
			}
			logger.WithField("operations_count", len(subtaskPatch.Operations)).Debug("subtask patch validated")
			return "subtask patch successfully processed", nil
		},
	}
	executor, err := fp.executor.GetRefinerExecutor(cfg)
	if err != nil {
		logger.WithError(err).Error("failed to get refiner executor")
		return nil, fmt.Errorf("failed to get refiner executor: %w", err)
	}

	chainBlob, err := json.Marshal(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal msg chain: %w", err)
	}

	msgChain, err = fp.db.CreateMsgChain(ctx, database.CreateMsgChainParams{
		Type:            msgChainType,
		Model:           fp.Model(optAgentType),
		ModelProvider:   string(fp.Type()),
		Chain:           chainBlob,
		FlowID:          fp.flowID,
		TaskID:          database.Int64ToNullInt64(&taskID),
		DurationSeconds: time.Since(startTime).Seconds(),
	})
	if err != nil {
		obs.LogErrorOrCancel(logger, err, "failed to create msg chain")
		return nil, fmt.Errorf("failed to create msg chain: %w", err)
	}

	logger.WithField("msg_chain_id", msgChain.ID).Debug("created msg chain for refiner")

	err = fp.performAgentChain(ctx, optAgentType, msgChain.ID, &taskID, nil, chain, executor, fp.summarizer)
	if err != nil {
		obs.LogErrorOrCancel(logger, err, "failed to perform subtasks refiner agent chain")
		return nil, fmt.Errorf("failed to get subtasks refiner result: %w", err)
	}

	// Apply the patch operations to the planned subtasks
	result, err := applySubtaskOperations(plannedSubtasks, subtaskPatch, logger)
	if err != nil {
		logger.WithError(err).Error("failed to apply subtask operations")
		return nil, fmt.Errorf("failed to apply subtask operations: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"input_count":  len(plannedSubtasks),
		"output_count": len(result),
		"operations":   len(subtaskPatch.Operations),
	}).Debug("successfully applied subtask patch")

	subtasks := convertSubtaskInfoPatch(result)
	if agentCtx, ok := tools.GetAgentContext(ctx); ok {
		_, _ = fp.putAgentLog(
			ctx,
			agentCtx.ParentAgentType,
			agentCtx.CurrentAgentType,
			input,
			fp.subtasksToMarkdown(subtasks),
			&taskID,
			nil,
		)
	}

	return subtasks, nil
}







// performPlanner invokes adviser to create an execution plan for agent tasks

// performMentor invokes adviser to monitor agent execution progress

func (fp *flowProvider) performSimpleChain(
	ctx context.Context,
	taskID, subtaskID *int64,
	opt pconfig.ProviderOptionsType,
	msgChainType database.MsgchainType,
	systemTmpl, userTmpl string,
) (string, error) {
	startTime := time.Now()

	chain := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemTmpl),
		llms.TextParts(llms.ChatMessageTypeHuman, userTmpl),
	}

	resp, err := retryTransient(ctx, delayBetweenRetries, func() (*llms.ContentResponse, error) {
		return fp.CallEx(ctx, opt, chain, nil)
	})
	if err != nil {
		return "", fmt.Errorf("failed to call simple chain: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	var parts []string
	var usage pconfig.CallUsage
	var reasoning *reasoning.ContentReasoning
	for _, choice := range resp.Choices {
		parts = append(parts, choice.Content)
		usage.Merge(fp.GetUsage(choice.GenerationInfo))
		// Preserve reasoning from first choice for simple chains (safe for all providers)
		if reasoning == nil && !choice.Reasoning.IsEmpty() {
			reasoning = choice.Reasoning
		}
	}

	// Update cost based on price info
	usage.UpdateCost(fp.GetPriceInfo(opt))

	// Universal pattern for simple chains - preserve reasoning if present
	msg := llms.MessageContent{Role: llms.ChatMessageTypeAI}
	content := strings.Join(parts, "\n")
	if part, ok := aiResponsePart(content, reasoning); ok {
		msg.Parts = append(msg.Parts, part)
	}
	chain = append(chain, msg)

	chainBlob, err := json.Marshal(chain)
	if err != nil {
		return "", fmt.Errorf("failed to marshal summarizer msg chain: %w", err)
	}

	if _, err = fp.db.CreateMsgChain(ctx, database.CreateMsgChainParams{
		Type:            msgChainType,
		Model:           fp.Model(opt),
		ModelProvider:   string(fp.Type()),
		UsageIn:         usage.Input,
		UsageOut:        usage.Output,
		UsageCacheIn:    usage.CacheRead,
		UsageCacheOut:   usage.CacheWrite,
		UsageCostIn:     usage.CostInput,
		UsageCostOut:    usage.CostOutput,
		DurationSeconds: time.Since(startTime).Seconds(),
		Chain:           chainBlob,
		FlowID:          fp.flowID,
		TaskID:          database.Int64ToNullInt64(taskID),
		SubtaskID:       database.Int64ToNullInt64(subtaskID),
	}); err != nil {
		return "", fmt.Errorf("failed to create simple msg chain: %w", err)
	}

	return strings.Join(parts, "\n\n"), nil
}
