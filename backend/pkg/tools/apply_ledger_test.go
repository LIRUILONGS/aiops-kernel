package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"opsagi/pkg/opsremote"
)

func acceptanceHandler() ExecutorHandler {
	return func(context.Context, string, json.RawMessage) (string, error) { return "accepted", nil }
}

func TestApplyLedger_RequireMeasuredVerdict_PinsTheVerdictToTheMeasurement(t *testing.T) {
	t.Parallel()

	change := func(state string) opsremote.Change {
		return opsremote.Change{State: state, Reason: "measured"}
	}

	cases := []struct {
		name    string
		state   string
		verdict string
		refused bool
	}{
		{name: "improved agrees with improved", state: opsremote.VerdictImproved, verdict: "improved"},
		{name: "improved refuses a claim of no effect", state: opsremote.VerdictImproved, verdict: "no_measurable_effect", refused: true},
		{name: "improved refuses a claim of damage", state: opsremote.VerdictImproved, verdict: "degraded", refused: true},
		{name: "worsened refuses a claim of repair", state: opsremote.VerdictWorsened, verdict: "improved", refused: true},
		{name: "worsened agrees with degraded", state: opsremote.VerdictWorsened, verdict: "degraded"},
		{name: "unchanged refuses a claim of repair", state: opsremote.VerdictUnchanged, verdict: "improved", refused: true},
		{name: "unchanged agrees with no effect", state: opsremote.VerdictUnchanged, verdict: "no_measurable_effect"},
		{name: "unverified refuses any claim of measurement", state: opsremote.VerdictUnverified, verdict: "improved", refused: true},
		{name: "unverified agrees with unverified", state: opsremote.VerdictUnverified, verdict: "unverified"},
		{name: "a state change may be argued either way", state: opsremote.VerdictChanged, verdict: "improved"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ledger := &applyLedger{}
			ledger.record("container_restart", change(tc.state))

			claimed, err := json.Marshal(map[string]string{"verdict": tc.verdict})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			answer, err := requireMeasuredVerdict(acceptanceHandler(), ledger)(t.Context(), OptimizationResultToolName, claimed)

			if !tc.refused {
				if err != nil {
					t.Fatalf("verdict %q was refused against %q: %v", tc.verdict, tc.state, err)
				}
				if answer != "accepted" {
					t.Fatalf("answer = %q, want the wrapped handler's", answer)
				}
				return
			}

			if err == nil {
				t.Fatalf("verdict %q was accepted against a measurement of %q", tc.verdict, tc.state)
			}
			if !strings.Contains(err.Error(), "ops_apply") {
				t.Errorf("error = %q, want it to name the measurement to quote", err)
			}
		})
	}
}

func TestApplyLedger_RequireMeasuredVerdict_LeavesAnUnmeasuredSubtaskAlone(t *testing.T) {
	t.Parallel()

	ledger := &applyLedger{}

	_, err := requireMeasuredVerdict(acceptanceHandler(), ledger)(
		t.Context(), OptimizationResultToolName, []byte(`{"verdict":"improved"}`),
	)

	if err != nil {
		t.Fatalf("a subtask that repaired nothing must not be judged: %v", err)
	}
}

func TestApplyLedger_RequireMeasuredVerdict_KeepsTheLastMeasurementOfTheSubtask(t *testing.T) {
	t.Parallel()

	ledger := &applyLedger{}
	ledger.record("container_restart", opsremote.Change{State: opsremote.VerdictUnchanged})
	ledger.record("disk_cleanup", opsremote.Change{State: opsremote.VerdictImproved})

	_, err := requireMeasuredVerdict(acceptanceHandler(), ledger)(
		t.Context(), OptimizationResultToolName, []byte(`{"verdict":"improved"}`),
	)

	if err != nil {
		t.Fatalf("the later measurement should decide: %v", err)
	}
}
