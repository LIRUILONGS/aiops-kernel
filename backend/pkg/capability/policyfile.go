package capability

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// policyFile is the on-disk shape of OPSAGI_POLICY. It is a separate struct so
// the YAML keys the operator writes and the values the runtime reads can drift
// apart without either one lying: the loader is the only translation.
type policyFile struct {
	TTL   string                `yaml:"ttl"`
	Modes map[string]policyMode `yaml:"modes"`
}

type policyMode struct {
	Targets       []string              `yaml:"targets"`
	Tools         []string              `yaml:"tools"`
	Commands      []string              `yaml:"commands"`
	Shapes        []string              `yaml:"shapes"`
	TracePrograms []string              `yaml:"trace_programs"`
	Actions       []string              `yaml:"actions"`
	Budgets       map[string]int        `yaml:"budgets"`
	Roles         map[string]policyRole `yaml:"roles"`
}

type policyRole struct {
	Targets       []string       `yaml:"targets"`
	Tools         []string       `yaml:"tools"`
	Commands      []string       `yaml:"commands"`
	Shapes        []string       `yaml:"shapes"`
	TracePrograms []string       `yaml:"trace_programs"`
	Actions       []string       `yaml:"actions"`
	Budgets       map[string]int `yaml:"budgets"`
}

// LoadPolicyFile reads a policy document and returns it ready to issue leases.
//
// Everything the runtime would otherwise have to guess is an error here: an
// unknown mode name is a typo that would silently fall back to the diagnosis
// rule, a bad duration would silently become the default, and a negative budget
// would refuse every call of its tool. The file is read once at start-up, so
// failing the boot costs a restart, not a run.
func LoadPolicyFile(path string) (Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("OPSAGI_POLICY cannot be read: %w", err)
	}

	var document policyFile
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return Policy{}, fmt.Errorf("OPSAGI_POLICY is not valid YAML: %w", err)
	}

	if len(document.Modes) == 0 {
		return Policy{}, errors.New("OPSAGI_POLICY names no mode, so every lease would fall back to the built-in default")
	}

	rules := make(map[string]Rule, len(document.Modes))
	roles := make(map[string]map[string]Rule, len(document.Modes))

	for name, mode := range document.Modes {
		if !knownMode(name) {
			return Policy{}, fmt.Errorf("OPSAGI_POLICY names mode %q, which the runtime never issues (known: %s)",
				name, strings.Join(knownModes(), ", "))
		}

		if err := validateBudgets("mode "+name, mode.Budgets); err != nil {
			return Policy{}, err
		}

		rules[name] = Rule{
			Targets:       mode.Targets,
			Tools:         mode.Tools,
			Commands:      mode.Commands,
			Shapes:        mode.Shapes,
			TracePrograms: mode.TracePrograms,
			Actions:       mode.Actions,
			Budgets:       mode.Budgets,
		}

		if len(mode.Roles) == 0 {
			continue
		}

		roles[name] = make(map[string]Rule, len(mode.Roles))
		for role, narrowing := range mode.Roles {
			if !knownRole(role) {
				return Policy{}, fmt.Errorf("OPSAGI_POLICY mode %q names role %q, which the runtime never issues (known: %s)",
					name, role, strings.Join(KnownRoles(), ", "))
			}

			if err := validateBudgets(fmt.Sprintf("mode %q role %q", name, role), narrowing.Budgets); err != nil {
				return Policy{}, err
			}

			roles[name][role] = Rule(narrowing)
		}
	}

	if _, ok := rules["diagnosis"]; !ok {
		return Policy{}, fmt.Errorf("OPSAGI_POLICY has no diagnosis rule, which is what an unknown plan mode falls back to")
	}

	ttl := DefaultLeaseTTL
	if strings.TrimSpace(document.TTL) != "" {
		parsed, err := time.ParseDuration(document.TTL)
		if err != nil {
			return Policy{}, fmt.Errorf("OPSAGI_POLICY ttl %q is not a duration: %w", document.TTL, err)
		}
		if parsed <= 0 {
			return Policy{}, fmt.Errorf("OPSAGI_POLICY ttl %q is not positive", document.TTL)
		}
		ttl = parsed
	}

	return Policy{Rules: rules, Roles: roles, TTL: ttl}, nil
}

func validateBudgets(where string, budgets map[string]int) error {
	for tool, limit := range budgets {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("OPSAGI_POLICY %s has a budget with an empty tool name", where)
		}
		if limit <= 0 {
			return fmt.Errorf("OPSAGI_POLICY %s budgets %s to %d, which refuses every call of it", where, tool, limit)
		}
	}

	return nil
}

func knownMode(name string) bool {
	return name == "diagnosis" || name == "optimization"
}

func knownModes() []string {
	modes := []string{"diagnosis", "optimization"}
	sort.Strings(modes)

	return modes
}

// KnownRoles is the closed set of roles a lease is issued for. The tool table in
// pkg/tools decides what each of them can reach; this list decides whose name a
// policy file may narrow.
func KnownRoles() []string {
	return []string{"change_analyst", "commander", "log_analyst", "metrics_analyst", "triage"}
}

func knownRole(name string) bool {
	for _, role := range KnownRoles() {
		if role == name {
			return true
		}
	}

	return false
}
