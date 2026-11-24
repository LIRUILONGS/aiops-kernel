package capability

import (
	"slices"
	"testing"
	"time"
)

func TestCapability_Policy_EffectiveRule_CutsTheModeDownToTheRole(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{
			"diagnosis": {
				Targets:       []string{"*"},
				Tools:         []string{"ops_exec", "ops_logs", "prom_query"},
				Commands:      []string{"listen_ports", "unit_logs"},
				Shapes:        []string{"probe"},
				TracePrograms: []string{"runqlat"},
			},
		},
		Roles: map[string]map[string]Rule{
			"diagnosis": {
				"log_analyst": {
					Tools:    []string{"ops_logs", "ops_sandbox"},
					Commands: []string{"unit_logs", "dmesg"},
				},
			},
		},
		TTL: time.Hour,
	}

	rule := policy.EffectiveRule("diagnosis", "log_analyst")

	if got := rule.Tools; !slices.Equal(got, []string{"ops_logs"}) {
		t.Errorf("tools = %v, want only the ones the mode also grants", got)
	}
	if got := rule.Commands; !slices.Equal(got, []string{"unit_logs"}) {
		t.Errorf("commands = %v, want only unit_logs", got)
	}
	if got := rule.Targets; !slices.Equal(got, []string{"*"}) {
		t.Errorf("targets = %v, want the mode's wildcard: the role said nothing about hosts", got)
	}
	if got := rule.TracePrograms; !slices.Equal(got, []string{"runqlat"}) {
		t.Errorf("trace programs = %v, want the mode's", got)
	}
}

func TestCapability_Policy_EffectiveRule_GivesARoleItNeverHadNothing(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{"diagnosis": {Tools: []string{"ops_logs"}}},
		Roles: map[string]map[string]Rule{
			"diagnosis": {"log_analyst": {Tools: []string{"ops_apply"}}},
		},
		TTL: time.Hour,
	}

	if got := policy.EffectiveRule("diagnosis", "log_analyst").Tools; len(got) != 0 {
		t.Errorf("tools = %v, want empty: a role rule cannot widen the mode", got)
	}
}

func TestCapability_Policy_EffectiveRule_LeavesAnUnnamedRoleOnTheModeRule(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{"diagnosis": {Tools: []string{"ops_logs", "ops_exec"}}},
		Roles: map[string]map[string]Rule{
			"diagnosis": {"log_analyst": {Tools: []string{"ops_logs"}}},
		},
		TTL: time.Hour,
	}

	if got := policy.EffectiveRule("diagnosis", "triage").Tools; !slices.Equal(got, []string{"ops_logs", "ops_exec"}) {
		t.Errorf("tools = %v, want the mode rule for a role the policy does not narrow", got)
	}
}

func TestCapability_Policy_EffectiveRule_NeverRaisesABudget(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{
			"diagnosis": {Tools: []string{"*"}, Budgets: map[string]int{"ops_exec": 60, "ops_logs": 30}},
		},
		Roles: map[string]map[string]Rule{
			"diagnosis": {
				"log_analyst": {Budgets: map[string]int{"ops_exec": 10, "ops_logs": 90, "prom_query": 5}},
			},
		},
		TTL: time.Hour,
	}

	budgets := policy.EffectiveRule("diagnosis", "log_analyst").Budgets

	if got := budgets["ops_exec"]; got != 10 {
		t.Errorf("ops_exec = %d, want the role's tighter 10", got)
	}
	if got := budgets["ops_logs"]; got != 30 {
		t.Errorf("ops_logs = %d, want the mode's 30 rather than the role's 90", got)
	}
	if got := budgets["prom_query"]; got != 5 {
		t.Errorf("prom_query = %d, want the role's 5", got)
	}
}

func TestCapability_Policy_EffectiveRule_LetsARoleWideCeilingCapTheModesNamedTool(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{
			"diagnosis": {Tools: []string{"*"}, Budgets: map[string]int{"ops_exec": 60}},
		},
		Roles: map[string]map[string]Rule{
			"diagnosis": {"log_analyst": {Budgets: map[string]int{"*": 8}}},
		},
		TTL: time.Hour,
	}

	if got := policy.EffectiveRule("diagnosis", "log_analyst").Budgets["ops_exec"]; got != 8 {
		t.Errorf("ops_exec = %d, want the role's ceiling of 8", got)
	}
}

func TestCapability_Policy_Issue_LeasesTheRuleTheRoleActuallyGets(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{"diagnosis": {Tools: []string{"ops_exec", "ops_logs"}}},
		Roles: map[string]map[string]Rule{
			"diagnosis": {"log_analyst": {Tools: []string{"ops_logs"}}},
		},
		TTL: time.Hour,
	}
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	lease := policy.Issue("diagnosis", 1, 2, "log_analyst", now)

	if got := lease.Tools; !slices.Equal(got, []string{"ops_logs"}) {
		t.Errorf("tools = %v, want the role's narrowed list", got)
	}
}

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

func TestCapability_Policy_IssueCarriesTheBudgetOfItsMode(t *testing.T) {
	policy := Policy{
		Rules: map[string]Rule{
			"diagnosis":    {Tools: []string{"*"}, Budgets: map[string]int{"ops_exec": 40}},
			"optimization": {Tools: []string{"*"}, Budgets: map[string]int{"ops_exec": 10}},
		},
		TTL: time.Hour,
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	lease := policy.Issue("optimization", 1, 2, "triage", now)

	if got := lease.Budgets["ops_exec"]; got != 10 {
		t.Errorf("budget = %d, want the optimization rule's 10", got)
	}
}

func TestCapability_Lease_BudgetFor_FallsBackFromTheToolToTheWildcard(t *testing.T) {
	lease := &Lease{Budgets: map[string]int{"ops_exec": 5, "*": 50}}

	if limit, ok := lease.BudgetFor("ops_exec"); !ok || limit != 5 {
		t.Errorf("ops_exec = %d (%v), want 5", limit, ok)
	}
	if limit, ok := lease.BudgetFor("ops_logs"); !ok || limit != 50 {
		t.Errorf("an unnamed tool must take the wildcard, got %d (%v)", limit, ok)
	}
	if _, ok := (&Lease{}).BudgetFor("ops_exec"); ok {
		t.Error("a lease with no budgets must not report a limit")
	}
}
