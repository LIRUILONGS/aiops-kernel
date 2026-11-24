package capability

import "time"

// DefaultLeaseTTL bounds how long an issued lease stays valid. It is generous
// against the run it covers: a lease that expires mid-diagnosis would refuse
// every later probe of a subtask that is working correctly, and the refusal
// would look like a policy decision instead of a clock running out.
const DefaultLeaseTTL = 8 * time.Hour

// Rule is the scope one plan mode gets when a lease is issued. A wildcard is
// the only way to say "everything"; an empty list permits nothing.
type Rule struct {
	Targets       []string
	Tools         []string
	Commands      []string
	Shapes        []string
	TracePrograms []string
	// Actions is what this mode may repair without asking. It stays empty in the
	// built-in default: the runtime that never wrote cannot widen itself into
	// writing by upgrading.
	Actions []string
	// Budgets bounds how many times one tool may run in a single execution of a
	// subtask. A tool absent from the map is unlimited, and "*" sets the limit
	// for every tool the map does not name.
	Budgets map[string]int
}

// Policy is what the operator configured. The default is deliberately the
// behaviour the runtime had before leases existed, so that enabling the switch
// does not silently narrow a flow that works today.
type Policy struct {
	Rules map[string]Rule
	TTL   time.Duration
}

// DefaultPolicy permits what the runtime permitted before the switch existed:
// every configured host, every tool the role already has, the whole read-only
// catalogue, and the probe shape.
func DefaultPolicy() Policy {
	full := Rule{
		Targets:       []string{"*"},
		Tools:         []string{"*"},
		Commands:      []string{"*"},
		Shapes:        []string{"probe"},
		TracePrograms: []string{"*"},
	}

	return Policy{
		Rules: map[string]Rule{
			"diagnosis":    full,
			"optimization": full,
		},
		TTL: DefaultLeaseTTL,
	}
}

// RuleFor returns the scope for a plan mode, falling back to the diagnosis rule
// for a mode the policy does not name.
func (p Policy) RuleFor(mode string) Rule {
	if rule, ok := p.Rules[mode]; ok {
		return rule
	}

	return p.Rules["diagnosis"]
}

// Issue builds the lease one subtask runs under. The caller persists it; this
// function only decides what the scope is.
func (p Policy) Issue(mode string, flowID, subtaskID int64, role string, now time.Time) *Lease {
	rule := p.RuleFor(mode)

	ttl := p.TTL
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}

	return &Lease{
		FlowID:        flowID,
		SubtaskID:     subtaskID,
		Role:          role,
		Mode:          ModeEnforce,
		Targets:       notNil(rule.Targets),
		Tools:         notNil(rule.Tools),
		Commands:      notNil(rule.Commands),
		Shapes:        notNil(rule.Shapes),
		TracePrograms: notNil(rule.TracePrograms),
		Actions:       notNil(rule.Actions),
		Budgets:       rule.Budgets,
		IssuedBy:      "policy",
		ExpiresAt:     now.Add(ttl),
	}
}

// notNil keeps an unset dimension an empty list rather than a nil one: the lease
// columns are NOT NULL, and a nil slice reaches PostgreSQL as NULL, which fails
// the insert and silently disables the guard for that subtask.
func notNil(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}
