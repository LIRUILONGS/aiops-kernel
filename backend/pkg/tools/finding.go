package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"opsagi/pkg/database"
)

// RecordFindingArgs is what an assistant files when a probe showed something the
// chat answer alone would not keep: the observation, what it means, and the
// output it rests on.
type RecordFindingArgs struct {
	Title    string `json:"title" jsonschema:"required" jsonschema_description:"结论的一句话标题，会作为事故时间线上的那一行，例如 caddy 的 p99 在 09:41 涨到 1.4s"`
	Detail   string `json:"detail" jsonschema_description:"探针看到了什么、意味着什么：带上工具返回里的数字与时刻，不要写「负载偏高」这种没有量级的描述"`
	Evidence string `json:"evidence,omitempty" jsonschema_description:"支撑这条结论的命令或查询，以及它输出的那几行原文，原样粘贴"`
}

type findingTool struct {
	mlp MsgLogProvider
}

func NewFindingTool(mlp MsgLogProvider) *findingTool {
	return &findingTool{mlp: mlp}
}

// Handle appends the finding to the flow's own message log rather than to the
// assistant's, so it outlives the chat it was written in.
func (f *findingTool) Handle(ctx context.Context, _ string, args json.RawMessage) (string, error) {
	if f.mlp == nil {
		return "", fmt.Errorf("the flow's message log is not available to this executor")
	}

	var in RecordFindingArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		return "", fmt.Errorf("title is required: it is the line the incident timeline shows")
	}
	detail := strings.TrimSpace(in.Detail)
	if detail == "" {
		return "", fmt.Errorf("detail is required: a title alone leaves the next reader nothing to check")
	}

	var body strings.Builder
	body.WriteString("**" + title + "**\n\n" + detail)
	if evidence := strings.TrimSpace(in.Evidence); evidence != "" {
		fence := "```"
		for strings.Contains(evidence, fence) {
			fence += "`"
		}
		body.WriteString("\n\n" + fence + "\n" + evidence + "\n" + fence)
	}

	if _, err := f.mlp.PutMsg(ctx, database.MsglogTypeAdvice, nil, nil, 0, "", body.String()); err != nil {
		return "", fmt.Errorf("failed to record the finding in the flow log: %w", err)
	}

	return "Recorded in the flow's log: " + title, nil
}
