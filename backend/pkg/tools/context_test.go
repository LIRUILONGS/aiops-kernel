package tools

import (
	"context"
	"testing"

	"opsagi/pkg/database"
)

func TestContext_GetAgentContext_FindsNoAgentWhereNoneWasPut(t *testing.T) {
	t.Parallel()

	type contextForeignKey string

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "an empty context", ctx: t.Context()},
		{name: "a context holding only other values", ctx: context.WithValue(t.Context(), contextForeignKey("k"), "v")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, ok := GetAgentContext(tt.ctx); ok {
				t.Errorf("GetAgentContext() = %+v, true; want no agent", got)
			}
		})
	}
}

// The chain is built whole before any row runs, so a context changed by deriving its child fails its own row.
func TestContext_PutAgentContext_MakesThePreviousCurrentAgentTheParent(t *testing.T) {
	t.Parallel()

	base := t.Context()
	first := PutAgentContext(base, database.MsgchainTypeCommander)
	second := PutAgentContext(first, database.MsgchainTypeMetricsAnalyst)
	third := PutAgentContext(second, database.MsgchainTypeChangeAnalyst)

	tests := []struct {
		name        string
		ctx         context.Context
		wantOK      bool
		wantParent  database.MsgchainType
		wantCurrent database.MsgchainType
	}{
		{name: "the base context keeps no agent", ctx: base, wantOK: false},
		{
			name:        "the first agent is its own parent",
			ctx:         first,
			wantOK:      true,
			wantParent:  database.MsgchainTypeCommander,
			wantCurrent: database.MsgchainTypeCommander,
		},
		{
			name:        "the second agent has the first as parent",
			ctx:         second,
			wantOK:      true,
			wantParent:  database.MsgchainTypeCommander,
			wantCurrent: database.MsgchainTypeMetricsAnalyst,
		},
		{
			name:        "the third agent has the second as parent, not the first",
			ctx:         third,
			wantOK:      true,
			wantParent:  database.MsgchainTypeMetricsAnalyst,
			wantCurrent: database.MsgchainTypeChangeAnalyst,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := GetAgentContext(tt.ctx)
			if ok != tt.wantOK {
				t.Fatalf("GetAgentContext() ok = %v, want %v", ok, tt.wantOK)
			}
			if got.ParentAgentType != tt.wantParent {
				t.Errorf("ParentAgentType = %q, want %q", got.ParentAgentType, tt.wantParent)
			}
			if got.CurrentAgentType != tt.wantCurrent {
				t.Errorf("CurrentAgentType = %q, want %q", got.CurrentAgentType, tt.wantCurrent)
			}
		})
	}
}
