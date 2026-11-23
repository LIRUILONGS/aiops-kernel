// Package capability answers one question per tool call: may this subtask do
// this, here, now? It is deliberately ignorant of tools, Docker and the
// database — the caller describes the call, this package returns a verdict and
// the reason for it, and the caller records what it decided.
package capability

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Mode is the per-flow setting an operator picks when the flow is created.
type Mode string

const (
	// ModeOff leaves every tool call untouched: the guard is not even installed.
	ModeOff Mode = "off"
	// ModeShadow evaluates the lease and records what it would refuse, but lets
	// the call through. It exists so a policy can be measured before it bites.
	ModeShadow Mode = "shadow"
	// ModeEnforce refuses the call.
	ModeEnforce Mode = "enforce"
)

// ParseMode maps a stored or requested value onto a mode. Anything unknown is
// off: a typo in a request or an old row must not turn enforcement on.
func ParseMode(value string) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(value))) {
	case ModeShadow:
		return ModeShadow
	case ModeEnforce:
		return ModeEnforce
	default:
		return ModeOff
	}
}

// Enabled reports whether the guard has anything to do at all.
func (m Mode) Enabled() bool { return m == ModeShadow || m == ModeEnforce }

// Enforcing reports whether a refusal actually stops the call.
func (m Mode) Enforcing() bool { return m == ModeEnforce }

// Call is the part of a tool call the lease is allowed to judge. Empty fields
// mean the tool does not take that dimension, not that anything is permitted.
type Call struct {
	Tool         string
	Target       string
	Command      string
	Shape        string
	TraceProgram string
}

// Lease is one subtask's scope: what it may use, on which hosts, until when.
type Lease struct {
	ID            int64
	FlowID        int64
	SubtaskID     int64
	Role          string
	Mode          Mode
	Targets       []string
	Tools         []string
	Commands      []string
	Shapes        []string
	TracePrograms []string
	IssuedBy      string
	ExpiresAt     time.Time
}

// Verdict is what the caller records, and in enforce mode what it obeys.
type Verdict string

const (
	VerdictAllow     Verdict = "allow"
	VerdictDeny      Verdict = "deny"
	VerdictWouldDeny Verdict = "would_deny"
)

// Decision carries the verdict and the sentence the model reads when refused.
type Decision struct {
	Verdict Verdict
	Reason  string
}

// Allowed reports whether the call may proceed.
func (d Decision) Allowed() bool { return d.Verdict != VerdictDeny }

// ForRecord returns the verdict to store. A refusal that did not stop the call
// happened in shadow mode, so what it records is what enforcement would do.
func (d Decision) ForRecord(mode Mode) Decision {
	if d.Verdict == VerdictDeny && !mode.Enforcing() {
		d.Verdict = VerdictWouldDeny
	}

	return d
}

func allow() Decision { return Decision{Verdict: VerdictAllow} }

func deny(format string, args ...any) Decision {
	return Decision{Verdict: VerdictDeny, Reason: fmt.Sprintf(format, args...)}
}

// Authorize judges one call against the lease. An empty lease list is a denial,
// not a wildcard: a lease that names nothing permits nothing.
func (l *Lease) Authorize(call Call, now time.Time) Decision {
	if l == nil {
		return deny("这个子任务没有能力租约，按缺省拒绝处理")
	}

	if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
		return deny("能力租约已于 %s 过期，需要操作员重新签发", l.ExpiresAt.Format(time.RFC3339))
	}

	if !permits(l.Tools, call.Tool) {
		return deny("工具 %s 不在这个子任务的租约里（允许：%s）", call.Tool, describe(l.Tools))
	}

	if call.Target != "" && !permits(l.Targets, call.Target) {
		return deny("目标主机 %s 不在这个子任务的租约里（允许：%s），换一台主机需要操作员批准", call.Target, describe(l.Targets))
	}

	if call.Command != "" && !permits(l.Commands, call.Command) {
		return deny("只读命令 %s 不在这个子任务的租约里（允许：%s）", call.Command, describe(l.Commands))
	}

	if call.Shape != "" && !permits(l.Shapes, call.Shape) {
		return deny("采集容器形状 %s 不在这个子任务的租约里（允许：%s），换形状需要操作员批准", call.Shape, describe(l.Shapes))
	}

	if call.TraceProgram != "" && !permits(l.TracePrograms, call.TraceProgram) {
		return deny("eBPF 程序 %s 不在这个子任务的租约里（允许：%s）", call.TraceProgram, describe(l.TracePrograms))
	}

	return allow()
}

func permits(allowed []string, value string) bool {
	return slices.Contains(allowed, "*") || slices.Contains(allowed, value)
}

// HasWildcard reports whether a list permits every value. A caller needs it
// when the concrete value is not known yet, as with an empty target: leaving it
// empty selects the default host, which is only safe when the lease is open.
func HasWildcard(allowed []string) bool {
	return slices.Contains(allowed, "*")
}

func describe(values []string) string {
	if len(values) == 0 {
		return "无"
	}

	return strings.Join(values, ", ")
}
