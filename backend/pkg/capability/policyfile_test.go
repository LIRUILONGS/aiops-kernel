package capability

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func writePolicy(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "policy.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	return path
}

func TestCapability_LoadPolicyFile_ReadsTheScopeAndTheBudgetAnOperatorWrote(t *testing.T) {
	path := writePolicy(t, `
ttl: 90m
modes:
  diagnosis:
    targets: ["lab"]
    tools: [ops_exec, ops_logs]
    commands: [unit_logs]
    shapes: [probe]
    trace_programs: [runqlat]
    budgets:
      ops_exec: 12
  optimization:
    targets: ["lab"]
    tools: [ops_exec]
    commands: ["*"]
    shapes: [probe]
`)

	policy, err := LoadPolicyFile(path)
	if err != nil {
		t.Fatalf("LoadPolicyFile: %v", err)
	}

	if policy.TTL != 90*time.Minute {
		t.Errorf("ttl = %s, want 90m", policy.TTL)
	}

	diagnosis := policy.RuleFor("diagnosis")
	if len(diagnosis.Targets) != 1 || diagnosis.Targets[0] != "lab" {
		t.Errorf("targets = %v, want [lab]", diagnosis.Targets)
	}
	if got := diagnosis.Budgets["ops_exec"]; got != 12 {
		t.Errorf("budget = %d, want 12", got)
	}

	if got := policy.RuleFor("optimization").Commands; len(got) != 1 || got[0] != "*" {
		t.Errorf("commands = %v, want the wildcard", got)
	}
}

func TestCapability_LoadPolicyFile_TheShippedExampleIsOneItAccepts(t *testing.T) {
	policy, err := LoadPolicyFile(filepath.Join("..", "..", "..", "docs", "aiops", "policy.example.yml"))
	if err != nil {
		t.Fatalf("the example an operator copies must load: %v", err)
	}

	if len(policy.Rules) != 2 {
		t.Errorf("rules = %d, want the two modes the example documents", len(policy.Rules))
	}
	if got := policy.RuleFor("diagnosis").Budgets["ops_exec"]; got != 60 {
		t.Errorf("diagnosis ops_exec budget = %d, want the 60 the example states", got)
	}

	logAnalyst := policy.EffectiveRule("diagnosis", "log_analyst")
	if !slices.Contains(logAnalyst.Tools, "ops_logs") || slices.Contains(logAnalyst.Tools, "ops_exec") {
		t.Errorf("log_analyst tools = %v, want logs and no shell", logAnalyst.Tools)
	}
	if got := logAnalyst.Budgets["ops_exec"]; got != 10 {
		t.Errorf("log_analyst ops_exec budget = %d, want the 10 the example states", got)
	}
}

func TestCapability_LoadPolicyFile_ReadsARoleNarrowingUnderItsMode(t *testing.T) {
	path := writePolicy(t, `
modes:
  diagnosis:
    tools: [ops_exec, ops_logs, prom_query]
    budgets:
      ops_exec: 60
    roles:
      log_analyst:
        tools: [ops_logs]
        budgets:
          ops_exec: 5
`)

	policy, err := LoadPolicyFile(path)
	if err != nil {
		t.Fatalf("LoadPolicyFile: %v", err)
	}

	rule := policy.EffectiveRule("diagnosis", "log_analyst")
	if len(rule.Tools) != 1 || rule.Tools[0] != "ops_logs" {
		t.Errorf("tools = %v, want the role's narrowed list", rule.Tools)
	}
	if got := rule.Budgets["ops_exec"]; got != 5 {
		t.Errorf("ops_exec budget = %d, want 5", got)
	}
	if got := policy.EffectiveRule("diagnosis", "triage").Tools; len(got) != 3 {
		t.Errorf("triage tools = %v, want the mode's three", got)
	}
}

func TestCapability_LoadPolicyFile_RefusesWhatWouldSilentlyFallBack(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		wants string
	}{
		{
			name:  "a mode name the runtime never issues",
			body:  "modes:\n  diagnosys:\n    tools: [ops_exec]\n  diagnosis:\n    tools: [ops_exec]\n",
			wants: `names mode "diagnosys"`,
		},
		{
			name:  "a file that names no mode at all",
			body:  "ttl: 1h\n",
			wants: "names no mode",
		},
		{
			name:  "no diagnosis rule to fall back to",
			body:  "modes:\n  optimization:\n    tools: [ops_exec]\n",
			wants: "no diagnosis rule",
		},
		{
			name:  "a duration the runtime cannot parse",
			body:  "ttl: soon\nmodes:\n  diagnosis:\n    tools: [ops_exec]\n",
			wants: `ttl "soon"`,
		},
		{
			name:  "a budget that refuses every call",
			body:  "modes:\n  diagnosis:\n    tools: [ops_exec]\n    budgets:\n      ops_exec: 0\n",
			wants: "budgets ops_exec to 0",
		},
		{
			name:  "yaml that does not parse",
			body:  "modes: [\n",
			wants: "not valid YAML",
		},
		{
			name:  "a role the runtime never issues",
			body:  "modes:\n  diagnosis:\n    tools: [ops_exec]\n    roles:\n      analyst:\n        tools: [ops_exec]\n",
			wants: `names role "analyst"`,
		},
		{
			name:  "a role budget that refuses every call",
			body:  "modes:\n  diagnosis:\n    tools: [ops_exec]\n    roles:\n      log_analyst:\n        budgets:\n          ops_exec: 0\n",
			wants: "budgets ops_exec to 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadPolicyFile(writePolicy(t, tt.body))
			if err == nil {
				t.Fatalf("want an error mentioning %q, got none", tt.wants)
			}
			if !strings.Contains(err.Error(), tt.wants) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wants)
			}
		})
	}
}

func TestCapability_LoadPolicyFile_RefusesAPathItCannotRead(t *testing.T) {
	_, err := LoadPolicyFile(filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("a missing policy file must fail the load")
	}
	if !strings.Contains(err.Error(), "OPSAGI_POLICY cannot be read") {
		t.Errorf("error = %q, want it to name the setting an operator set", err.Error())
	}
}
