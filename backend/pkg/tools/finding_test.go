package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"opsagi/pkg/database"
)

// findingMsgLog keeps what the tool filed, so a test can read the type and body
// the flow's timeline is built from.
type findingMsgLog struct {
	putErr error

	mu      sync.Mutex
	msgType database.MsglogType
	msg     string
	writes  int
}

var _ MsgLogProvider = (*findingMsgLog)(nil)

func (l *findingMsgLog) PutMsg(
	ctx context.Context, msgType database.MsglogType, _, _ *int64, _ int64, _, msg string,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if l.putErr != nil {
		return 0, l.putErr
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgType, l.msg = msgType, msg
	l.writes++
	return int64(l.writes), nil
}

func (l *findingMsgLog) UpdateMsgResult(context.Context, int64, int64, string, database.MsglogResultFormat) error {
	return nil
}

func TestFinding_Handle_RecordsTheFindingAsAnAdviceMessage(t *testing.T) {
	t.Parallel()

	log := &findingMsgLog{}
	tool := NewFindingTool(log)

	out, err := tool.Handle(t.Context(), RecordFindingToolName, json.RawMessage(`{
		"title": "caddy p99 rose to 1.4s at 09:41",
		"detail": "The 99th percentile went from 180ms to 1.4s while requests per second stayed flat.",
		"evidence": "$ curl -s localhost:2019/metrics | grep caddy_http_request_duration\ncaddy_http_request_duration_seconds{quantile=\"0.99\"} 1.402"
	}`))
	require.NoError(t, err)
	assert.Contains(t, out, "caddy p99 rose to 1.4s at 09:41")

	assert.Equal(t, database.MsglogTypeAdvice, log.msgType, "the finding lands on the flow's timeline, not in the chat log")
	assert.Contains(t, log.msg, "**caddy p99 rose to 1.4s at 09:41**")
	assert.Contains(t, log.msg, "The 99th percentile went from 180ms to 1.4s")
	assert.Contains(t, log.msg, "caddy_http_request_duration_seconds{quantile=\"0.99\"} 1.402")
}

func TestFinding_Handle_LeavesTheEvidenceOutWhenItWasNotGiven(t *testing.T) {
	t.Parallel()

	log := &findingMsgLog{}
	tool := NewFindingTool(log)

	_, err := tool.Handle(t.Context(), RecordFindingToolName, json.RawMessage(`{"title":"load fell after the restart","detail":"load1 went from 14.2 to 0.8 within a minute of the restart"}`))
	require.NoError(t, err)

	assert.NotContains(t, log.msg, "```")
}

func TestFinding_Handle_KeepsEvidenceCarryingAFenceReadable(t *testing.T) {
	t.Parallel()

	log := &findingMsgLog{}
	tool := NewFindingTool(log)

	_, err := tool.Handle(t.Context(), RecordFindingToolName, json.RawMessage(
		"{\"title\":\"the runbook is inside the pod\",\"detail\":\"the container ships a README\",\"evidence\":\"```\\nrun this\\n```\"}"))
	require.NoError(t, err)

	assert.Contains(t, log.msg, "````\n```\nrun this\n```\n````")
}

func TestFinding_Handle_RefusesAFindingWithNothingToRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{name: "no title", args: `{"detail":"load1 went from 14.2 to 0.8"}`, wantErr: "title is required"},
		{name: "a blank title", args: `{"title":"   ","detail":"load1 went from 14.2 to 0.8"}`, wantErr: "title is required"},
		{name: "no detail", args: `{"title":"load fell"}`, wantErr: "detail is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			log := &findingMsgLog{}
			tool := NewFindingTool(log)

			_, err := tool.Handle(t.Context(), RecordFindingToolName, json.RawMessage(tt.args))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Zero(t, log.writes, "a refused finding reaches the flow's log")
		})
	}
}

func TestFinding_Handle_ReportsAWriteTheLogRefused(t *testing.T) {
	t.Parallel()

	log := &findingMsgLog{putErr: errors.New("connection reset by peer")}
	tool := NewFindingTool(log)

	_, err := tool.Handle(t.Context(), RecordFindingToolName, json.RawMessage(`{"title":"load fell","detail":"load1 went from 14.2 to 0.8"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset by peer")
}

func TestFinding_Handle_RefusesToWriteWithoutAMessageLog(t *testing.T) {
	t.Parallel()

	tool := NewFindingTool(nil)

	_, err := tool.Handle(t.Context(), RecordFindingToolName, json.RawMessage(`{"title":"load fell","detail":"load1 went from 14.2 to 0.8"}`))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "message log"))
}
