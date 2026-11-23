package capability

import (
	"slices"
	"testing"
	"time"
)

func TestCapability_Policy_IssueKeepsTheScopeTheRuntimeHadBeforeTheSwitch(t *testing.T) {
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	for _, mode := range []string{"diagnosis", "optimization"} {
		t.Run(mode, func(t *testing.T) {
			lease := DefaultPolicy().Issue(mode, 7, 42, "commander", now)

			if !slices.Contains(lease.Targets, "*") {
				t.Errorf("every configured host was reachable before the switch, got targets %v", lease.Targets)
			}
			if !slices.Contains(lease.Tools, "*") {
				t.Errorf("a role's whole tool list was usable before the switch, got tools %v", lease.Tools)
			}
			if !slices.Contains(lease.Commands, "*") {
				t.Errorf("the catalogue was fully usable before the switch, got commands %v", lease.Commands)
			}
			if len(lease.Shapes) != 1 || lease.Shapes[0] != "probe" {
				t.Errorf("the probe shape was the default one, got shapes %v", lease.Shapes)
			}
			if got, want := lease.ExpiresAt, now.Add(8*time.Hour); !got.Equal(want) {
				t.Errorf("expiry = %s, want %s", got, want)
			}
			if lease.ExpiresAt.Before(now) {
				t.Error("a freshly issued lease must not already be expired")
			}
			if lease.FlowID != 7 || lease.SubtaskID != 42 || lease.Role != "commander" {
				t.Errorf("the lease must carry the identity it was issued for, got %+v", lease)
			}
			if lease.IssuedBy != "policy" {
				t.Errorf("issuer = %q, want policy", lease.IssuedBy)
			}
		})
	}
}

func TestCapability_Policy_RuleForFallsBackToDiagnosis(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{
			"diagnosis": {Tools: []string{"ops_logs"}},
		},
		TTL: time.Minute,
	}

	if got := policy.RuleFor("optimization"); !slices.Equal(got.Tools, []string{"ops_logs"}) {
		t.Errorf("an unnamed mode must fall back to the diagnosis rule, got %v", got.Tools)
	}
}

func TestCapability_Policy_IssueFallsBackToTheDefaultTTL(t *testing.T) {
	policy := Policy{Rules: DefaultPolicy().Rules, TTL: 0}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	lease := policy.Issue("diagnosis", 1, 2, "commander", now)

	if got, want := lease.ExpiresAt, now.Add(8*time.Hour); !got.Equal(want) {
		t.Errorf("expiry = %s, want %s", got, want)
	}
}
