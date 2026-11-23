package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"opsagi/pkg/capability"
)

func guardLease(expiresIn time.Duration) *capability.Lease {
	return &capability.Lease{
		ID:        3,
		FlowID:    7,
		SubtaskID: 11,
		Role:      "commander",
		Mode:      capability.ModeEnforce,
		Tools:     []string{OpsExecToolName, OpsSandboxToolName, OpsTraceToolName},
		Targets:   []string{"lab"},
		Commands:  []string{"unit_logs"},
		Shapes:    []string{"probe"},
		ExpiresAt: time.Now().Add(expiresIn),
	}
}

// guardFor builds a guard with only the sink a given test needs.
func guardFor(
	lease *capability.Lease,
	mode capability.Mode,
	record func(context.Context, capability.Call, capability.Decision),
) *CapabilityGuard {
	return NewCapabilityGuard(CapabilityGuardOptions{Lease: lease, Mode: mode, Record: record})
}

// recordingHandler counts the calls that reached the tool and returns what it
// was given, so a wrapper that mangles the arguments is visible.
func recordingHandler(calls *int, seen *json.RawMessage) ExecutorHandler {
	return func(_ context.Context, _ string, args json.RawMessage) (string, error) {
		*calls++
		*seen = args

		return "handler ran", nil
	}
}

func TestCapabilityGuard_Apply_LeavesTheCallAloneWhenTheFlowIsOff(t *testing.T) {
	var recorded []capability.Decision
	calls := 0
	var seen json.RawMessage
	lease := guardLease(time.Hour)

	handlers := map[string]ExecutorHandler{OpsExecToolName: recordingHandler(&calls, &seen)}
	guarded := guardFor(lease, capability.ModeOff, func(_ context.Context, _ capability.Call, d capability.Decision) {
		recorded = append(recorded, d)
	}).Apply(handlers, nil)

	args := json.RawMessage(`{"target":"prod","command":"pg_locks"}`)
	result, err := guarded[OpsExecToolName](context.Background(), OpsExecToolName, args)

	if err != nil {
		t.Fatalf("off must not refuse even a call outside the lease, got %v", err)
	}
	if result != "handler ran" {
		t.Errorf("result = %q, want the handler's own answer", result)
	}
	if calls != 1 {
		t.Errorf("handler calls = %d, want 1", calls)
	}
	if string(seen) != string(args) {
		t.Errorf("arguments reached the handler as %s, want %s", seen, args)
	}
	if len(recorded) != 0 {
		t.Errorf("off must not record anything, got %v", recorded)
	}
}

func TestCapabilityGuard_Apply_RefusesTheCallTheLeaseDoesNotCover(t *testing.T) {
	var recorded []capability.Decision
	calls := 0
	var seen json.RawMessage

	handlers := map[string]ExecutorHandler{OpsExecToolName: recordingHandler(&calls, &seen)}
	guarded := guardFor(guardLease(time.Hour), capability.ModeEnforce, func(_ context.Context, _ capability.Call, d capability.Decision) {
		recorded = append(recorded, d)
	}).Apply(handlers, nil)

	_, err := guarded[OpsExecToolName](context.Background(), OpsExecToolName, json.RawMessage(`{"target":"prod","command":"unit_logs"}`))

	if err == nil {
		t.Fatal("a host outside the lease must be refused")
	}
	var refusal *Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("refusal must be a tools.Refusal so the executor reports it to the model, got %T", err)
	}
	if !strings.Contains(refusal.Error(), "prod") {
		t.Errorf("the refusal must name the host it refused, got %q", refusal.Error())
	}
	if calls != 0 {
		t.Errorf("the handler ran %d times, want 0: a refusal must not reach the tool", calls)
	}
	if len(recorded) != 1 || recorded[0].Verdict != capability.VerdictDeny {
		t.Fatalf("want one deny decision, got %v", recorded)
	}
}

func TestCapabilityGuard_Apply_RunsTheCallAndRecordsWhatEnforcementWouldStop(t *testing.T) {
	var recorded []capability.Decision
	calls := 0
	var seen json.RawMessage

	handlers := map[string]ExecutorHandler{OpsExecToolName: recordingHandler(&calls, &seen)}
	guarded := guardFor(guardLease(time.Hour), capability.ModeShadow, func(_ context.Context, _ capability.Call, d capability.Decision) {
		recorded = append(recorded, d)
	}).Apply(handlers, nil)

	result, err := guarded[OpsExecToolName](context.Background(), OpsExecToolName, json.RawMessage(`{"target":"prod","command":"unit_logs"}`))

	if err != nil {
		t.Fatalf("shadow must not refuse, got %v", err)
	}
	if result != "handler ran" || calls != 1 {
		t.Fatalf("shadow must run the handler, got %q after %d calls", result, calls)
	}
	if len(recorded) != 1 || recorded[0].Verdict != capability.VerdictWouldDeny {
		t.Fatalf("want one would_deny decision, got %v", recorded)
	}
	if !strings.Contains(recorded[0].Reason, "prod") {
		t.Errorf("the recorded reason must say what would have been refused, got %q", recorded[0].Reason)
	}
}

func TestCapabilityGuard_Apply_RefusesAnEmptyTargetUnderANarrowLease(t *testing.T) {
	calls := 0
	var seen json.RawMessage

	handlers := map[string]ExecutorHandler{OpsExecToolName: recordingHandler(&calls, &seen)}
	guarded := guardFor(guardLease(time.Hour), capability.ModeEnforce, nil).Apply(handlers, nil)

	_, err := guarded[OpsExecToolName](context.Background(), OpsExecToolName, json.RawMessage(`{"command":"unit_logs"}`))

	if err == nil {
		t.Fatal("an unnamed host means the default one, which a lease that lists hosts does not cover")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("the refusal must ask for an explicit target, got %q", err)
	}
	if calls != 0 {
		t.Errorf("the handler ran %d times, want 0", calls)
	}
}

func TestCapabilityGuard_Apply_LeavesTheExitAloneUnderALeaseThatDoesNotNameIt(t *testing.T) {
	calls := 0
	var seen json.RawMessage

	handlers := map[string]ExecutorHandler{DiagnosisToolName: recordingHandler(&calls, &seen)}
	guarded := guardFor(guardLease(time.Hour), capability.ModeEnforce, nil).
		Apply(handlers, map[string]struct{}{DiagnosisToolName: {}})

	result, err := guarded[DiagnosisToolName](context.Background(), DiagnosisToolName, json.RawMessage(`{}`))

	if err != nil {
		t.Fatalf("a narrow lease must not stop the chain from concluding, got %v", err)
	}
	if result != "handler ran" || calls != 1 {
		t.Errorf("the exit ran %d times and answered %q, want 1 and the handler's own answer", calls, result)
	}
}

func TestCapabilityGuard_ExtractCapabilityCall_ReadsTheDimensionTheLeaseJudges(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		args  string
		wants capability.Call
	}{
		{
			name:  "an exec names a host and a command",
			tool:  OpsExecToolName,
			args:  `{"target":"lab","command":"unit_logs"}`,
			wants: capability.Call{Tool: OpsExecToolName, Target: "lab", Command: "unit_logs"},
		},
		{
			name:  "a sandbox with no mode is the probe shape",
			tool:  OpsSandboxToolName,
			args:  `{"target":"lab"}`,
			wants: capability.Call{Tool: OpsSandboxToolName, Target: "lab", Shape: "probe"},
		},
		{
			name:  "a sandbox asks for workshop",
			tool:  OpsSandboxToolName,
			args:  `{"target":"lab","mode":"workshop"}`,
			wants: capability.Call{Tool: OpsSandboxToolName, Target: "lab", Shape: "workshop"},
		},
		{
			name:  "a trace names its program",
			tool:  OpsTraceToolName,
			args:  `{"target":"lab","script":"runqlat"}`,
			wants: capability.Call{Tool: OpsTraceToolName, Target: "lab", TraceProgram: "runqlat"},
		},
		{
			name:  "a tool that takes no host has no dimensions",
			tool:  AskUserToolName,
			args:  `{"question":"which host?"}`,
			wants: capability.Call{Tool: AskUserToolName},
		},
		{
			name:  "arguments the guard cannot read leave the dimensions empty",
			tool:  OpsExecToolName,
			args:  `{"command":`,
			wants: capability.Call{Tool: OpsExecToolName},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCapabilityCall(tc.tool, json.RawMessage(tc.args))
			if got != tc.wants {
				t.Errorf("call = %+v, want %+v", got, tc.wants)
			}
		})
	}
}

func TestCapabilityGuard_Apply_FilesTheApprovalItQuotes(t *testing.T) {
	calls := 0
	var seen json.RawMessage
	var filed []capability.Call
	var reasons []string

	guard := NewCapabilityGuard(CapabilityGuardOptions{
		Lease: guardLease(time.Hour),
		Mode:  capability.ModeEnforce,
		Request: func(_ context.Context, call capability.Call, reason string) (int64, error) {
			filed = append(filed, call)
			reasons = append(reasons, reason)

			return 42, nil
		},
	})

	_, err := guard.Apply(map[string]ExecutorHandler{OpsExecToolName: recordingHandler(&calls, &seen)}, nil)[OpsExecToolName](
		context.Background(), OpsExecToolName, json.RawMessage(`{"target":"prod","command":"unit_logs"}`),
	)

	if err == nil {
		t.Fatal("a host outside the lease must be refused")
	}
	if !strings.Contains(err.Error(), "审批 #42") {
		t.Errorf("the refusal must quote the request it filed, got %q", err.Error())
	}
	if len(filed) != 1 || filed[0].Target != "prod" || filed[0].Command != "unit_logs" {
		t.Fatalf("the filed request must name the refused call, got %+v", filed)
	}
	if len(reasons) != 1 || !strings.Contains(reasons[0], "prod") {
		t.Errorf("the request must carry the reason, got %v", reasons)
	}
	if calls != 0 {
		t.Errorf("the handler ran %d times, want 0", calls)
	}
}

func TestCapabilityGuard_Apply_LetsTheNextCallThroughOnceAnApprovalExists(t *testing.T) {
	calls := 0
	var seen json.RawMessage
	var grants []capability.Grant

	guard := NewCapabilityGuard(CapabilityGuardOptions{
		Lease:  guardLease(time.Hour),
		Mode:   capability.ModeEnforce,
		Grants: func(context.Context) []capability.Grant { return grants },
	})
	handlers := guard.Apply(map[string]ExecutorHandler{OpsExecToolName: recordingHandler(&calls, &seen)}, nil)
	args := json.RawMessage(`{"target":"prod","command":"unit_logs"}`)

	if _, err := handlers[OpsExecToolName](context.Background(), OpsExecToolName, args); err == nil {
		t.Fatal("the call must be refused before anyone approved it")
	}

	grants = []capability.Grant{{
		Tool:      OpsExecToolName,
		Target:    "prod",
		Command:   "unit_logs",
		IssuedBy:  "1",
		ExpiresAt: time.Now().Add(time.Hour),
	}}

	if _, err := handlers[OpsExecToolName](context.Background(), OpsExecToolName, args); err != nil {
		t.Fatalf("the approved call must run, got %v", err)
	}
	if calls != 1 {
		t.Errorf("handler calls = %d, want 1: the approval lifted exactly one attempt", calls)
	}
}

func TestCapabilityGuard_Apply_DoesNotAskForAToolTheRoleNeverHeld(t *testing.T) {
	calls := 0
	var seen json.RawMessage
	asked := 0

	guard := NewCapabilityGuard(CapabilityGuardOptions{
		Lease: guardLease(time.Hour),
		Mode:  capability.ModeEnforce,
		Request: func(context.Context, capability.Call, string) (int64, error) {
			asked++

			return 1, nil
		},
	})

	_, err := guard.Apply(map[string]ExecutorHandler{TerminalToolName: recordingHandler(&calls, &seen)}, nil)[TerminalToolName](
		context.Background(), TerminalToolName, json.RawMessage(`{"message":"id"}`),
	)

	if err == nil {
		t.Fatal("a tool outside the role's lease must be refused")
	}
	if asked != 0 {
		t.Errorf("filed %d requests: a tool grant would bypass the role tool table", asked)
	}
	if strings.Contains(err.Error(), "审批") {
		t.Errorf("refusal %q promises an approval that cannot be granted", err.Error())
	}
}
