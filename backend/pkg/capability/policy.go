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
	// Roles narrows a mode for one role, keyed by plan mode and then by role. A
	// role rule can only take away what its mode allows: an entry that names a
	// tool the mode never granted still grants nothing.
	Roles map[string]map[string]Rule
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

// EffectiveRule is what one role gets in one plan mode: the mode's rule, cut
// down by the role's rule when the policy has one. Narrowing rather than
// overriding is what keeps a per-role entry from widening a mode, so a typo in
// a role list can only cost access, never grant it.
func (p Policy) EffectiveRule(mode, role string) Rule {
	base := p.RuleFor(mode)

	roles, ok := p.Roles[mode]
	if !ok {
		return base
	}

	narrowing, ok := roles[role]
	if !ok {
		return base
	}

	return narrow(base, narrowing)
}

// Issue builds the lease one subtask runs under. The caller persists it; this
// function only decides what the scope is.
func (p Policy) Issue(mode string, flowID, subtaskID int64, role string, now time.Time) *Lease {
	rule := p.EffectiveRule(mode, role)

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

func narrow(allowed, asked Rule) Rule {
	return Rule{
		Targets:       onlyWithin(allowed.Targets, asked.Targets),
		Tools:         onlyWithin(allowed.Tools, asked.Tools),
		Commands:      onlyWithin(allowed.Commands, asked.Commands),
		Shapes:        onlyWithin(allowed.Shapes, asked.Shapes),
		TracePrograms: onlyWithin(allowed.TracePrograms, asked.TracePrograms),
		Actions:       onlyWithin(allowed.Actions, asked.Actions),
		Budgets:       tighterBudgets(allowed.Budgets, asked.Budgets),
	}
}

// onlyWithin keeps the values of asked that allowed already permits. An empty
// asked says nothing about the dimension and keeps the mode's list.
func onlyWithin(allowed, asked []string) []string {
	if len(asked) == 0 {
		return allowed
	}

	if hasWildcard(allowed) {
		return asked
	}

	kept := make([]string, 0, len(asked))
	for _, value := range asked {
		if contains(allowed, value) {
			kept = append(kept, value)
		}
	}

	return kept
}

// tighterBudgets never raises a limit: a role that names a tool its mode already
// bounds gets the smaller of the two, and a role-wide ceiling also caps the
// tools the mode named one by one.
func tighterBudgets(allowed, asked map[string]int) map[string]int {
	if len(asked) == 0 {
		return allowed
	}

	merged := make(map[string]int, len(allowed)+len(asked))
	for tool, limit := range allowed {
		merged[tool] = limit
	}

	for tool, limit := range asked {
		if current, ok := merged[tool]; ok && current < limit {
			continue
		}

		merged[tool] = limit
	}

	if ceiling, ok := asked["*"]; ok {
		for tool, limit := range merged {
			if tool != "*" && limit > ceiling {
				merged[tool] = ceiling
			}
		}
	}

	return merged
}

func hasWildcard(values []string) bool {
	return contains(values, "*")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
