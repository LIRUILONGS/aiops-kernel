package tools

import (
	"context"
	"encoding/json"
	"time"

	"opsagi/pkg/capability"
	"opsagi/pkg/opsremote"
)

// CapabilityGuard is the one place a flow's lease is turned into a decision
// about a tool call. It is an aspect: it wraps handlers, knows nothing about
// what they do, and is not installed at all when the flow's mode is off.
type CapabilityGuard struct {
	lease  *capability.Lease
	mode   capability.Mode
	record func(ctx context.Context, call capability.Call, decision capability.Decision)
	now    func() time.Time
}

// NewCapabilityGuard builds the guard for one subtask. A nil record sink is
// allowed: the decision is still enforced, it is simply not audited.
func NewCapabilityGuard(
	lease *capability.Lease,
	mode capability.Mode,
	record func(ctx context.Context, call capability.Call, decision capability.Decision),
) *CapabilityGuard {
	return &CapabilityGuard{lease: lease, mode: mode, record: record, now: time.Now}
}

// Apply wraps the evidence handlers with the lease check. It returns the map
// untouched when the flow did not opt in, so an instance with the switch off
// runs exactly the code path it ran before leases existed.
//
// Handlers named in except are left alone. Those are the exits and the question
// channel: a lease says what a subtask may touch, and refusing the way it
// concludes its work would turn a narrow lease into a chain that cannot end.
func (g *CapabilityGuard) Apply(handlers map[string]ExecutorHandler, except map[string]struct{}) map[string]ExecutorHandler {
	if g == nil || !g.mode.Enabled() || g.lease == nil {
		return handlers
	}

	wrapped := make(map[string]ExecutorHandler, len(handlers))
	for name, handler := range handlers {
		if _, isExit := except[name]; isExit || handler == nil {
			wrapped[name] = handler
			continue
		}
		wrapped[name] = g.wrap(name, handler)
	}

	return wrapped
}

func (g *CapabilityGuard) wrap(name string, handler ExecutorHandler) ExecutorHandler {
	return func(ctx context.Context, toolName string, args json.RawMessage) (string, error) {
		call := extractCapabilityCall(name, args)
		decision := g.authorize(call)

		if g.record != nil {
			g.record(ctx, call, decision.ForRecord(g.mode))
		}

		if decision.Allowed() || !g.mode.Enforcing() {
			return handler(ctx, toolName, args)
		}

		return "", Refuse("%s", decision.Reason)
	}
}

// authorize adds the one check the lease cannot make on its own: an empty
// target means the default host, which is only in scope when the lease is open.
func (g *CapabilityGuard) authorize(call capability.Call) capability.Decision {
	if call.Target == "" && callNeedsTarget(call.Tool) && !capability.HasWildcard(g.lease.Targets) {
		return capability.Decision{
			Verdict: capability.VerdictDeny,
			Reason:  "这次调用没有写 target，而租约只放开了列出的主机；请显式指定目标主机",
		}
	}

	return g.lease.Authorize(call, g.now())
}

// callNeedsTarget reports whether a tool talks to a configured host, and so has
// a target dimension for the lease to judge.
func callNeedsTarget(tool string) bool {
	switch tool {
	case OpsSystemToolName, OpsContainersToolName, OpsLogsToolName, OpsBaselineToolName,
		OpsDiffToolName, OpsExecToolName, OpsWatchToolName, OpsSandboxToolName, OpsTraceToolName:
		return true
	default:
		return false
	}
}

type probeCallArgs struct {
	Target  string `json:"target"`
	Command string `json:"command"`
	Mode    string `json:"mode"`
	Script  string `json:"script"`
}

// extractCapabilityCall reads the dimensions a lease judges out of the call's
// arguments. Malformed arguments are left to the handler, which reports them
// better than the guard could: the guard only fills what it can read.
func extractCapabilityCall(tool string, args json.RawMessage) capability.Call {
	call := capability.Call{Tool: tool}
	if !callNeedsTarget(tool) || len(args) == 0 {
		return call
	}

	var parsed probeCallArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return call
	}

	call.Target = parsed.Target

	switch tool {
	case OpsExecToolName, OpsWatchToolName:
		call.Command = parsed.Command
	case OpsSandboxToolName:
		shape := opsremote.SandboxMode(parsed.Mode)
		if shape == "" {
			shape = opsremote.SandboxModeProbe
		}
		call.Shape = string(shape)
	case OpsTraceToolName:
		call.TraceProgram = parsed.Script
	}

	return call
}
