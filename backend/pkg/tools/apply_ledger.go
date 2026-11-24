package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"opsagi/pkg/opsremote"
)

// applyLedger is what the code measured about the writes this subtask made. It
// exists so a conclusion can be checked against the measurement instead of being
// taken on trust: a repair that measured nothing cannot be reported as improved.
type applyLedger struct {
	mu     sync.Mutex
	change opsremote.Change
	action string
	seen   bool
}

func (l *applyLedger) record(action string, change opsremote.Change) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.change = change
	l.action = action
	l.seen = true
}

func (l *applyLedger) latest() (opsremote.Change, string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.change, l.action, l.seen
}

// requireMeasuredVerdict wraps the conclusion handler so that a performance
// result cannot contradict what ops_apply measured a moment earlier. The model
// still writes the report; the one field it cannot invent is the verdict, because
// that one was computed by comparing two fingerprints.
func requireMeasuredVerdict(next ExecutorHandler, ledger *applyLedger) ExecutorHandler {
	return func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		if err := measuredVerdictError(ledger, args); err != nil {
			return "", err
		}

		return next(ctx, name, args)
	}
}

func measuredVerdictError(ledger *applyLedger, args json.RawMessage) error {
	change, action, ok := ledger.latest()
	if !ok {
		return nil
	}

	var claim struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(args, &claim); err != nil {
		return nil
	}

	verdict := strings.TrimSpace(claim.Verdict)
	if verdict == "" {
		return nil
	}

	if allowed, why := verdictAgrees(verdict, change.State); !allowed {
		return fmt.Errorf("%s。这次的 %s 由代码量过（%s）：%s。把 verdict 改成 %s，"+
			"并把 ops_apply 返回的变更结论原文写进 observed_gain",
			why, action, change.State, change.Reason, suggestedVerdict(change.State))
	}

	return nil
}

// verdictAgrees says whether a claimed verdict survives the measurement. A
// state change carries no direction on its own, so either direction may be
// argued from it; everything else pins the claim to what the rows said.
func verdictAgrees(verdict, measured string) (bool, string) {
	switch measured {
	case opsremote.VerdictUnverified:
		if verdict != "unverified" {
			return false, "代码没有测到这次变更的效果，所以不能声称测过"
		}
	case opsremote.VerdictUnchanged:
		if verdict != "no_measurable_effect" {
			return false, "代码测到的结果是没有任何可测量的变化"
		}
	case opsremote.VerdictWorsened:
		if verdict == "improved" {
			return false, "代码测到的变化里至少有一项是恶化"
		}
	case opsremote.VerdictImproved:
		if verdict == "degraded" || verdict == "no_measurable_effect" {
			return false, "代码测到的相关指标全部在改善"
		}
	}

	return true, ""
}

func suggestedVerdict(measured string) string {
	switch measured {
	case opsremote.VerdictImproved:
		return "improved"
	case opsremote.VerdictWorsened:
		return "degraded"
	case opsremote.VerdictUnchanged:
		return "no_measurable_effect"
	default:
		return "unverified"
	}
}
