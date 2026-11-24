package capability

import (
	"strings"
	"testing"
	"time"
)

func TestCapability_Authorize_PermitsWhatTheLeaseNames(t *testing.T) {
	lease := &Lease{
		Tools:         []string{"ops_exec", "ops_logs"},
		Targets:       []string{"lab"},
		Commands:      []string{"unit_logs", "memory"},
		Shapes:        []string{"probe"},
		TracePrograms: []string{"runqlat"},
		ExpiresAt:     time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	decision := lease.Authorize(Call{
		Tool:         "ops_exec",
		Target:       "lab",
		Command:      "unit_logs",
		Shape:        "probe",
		TraceProgram: "runqlat",
	}, now)

	if !decision.Allowed() {
		t.Fatalf("expected the call inside the lease to pass, got %q (%s)", decision.Verdict, decision.Reason)
	}
	if decision.Reason != "" {
		t.Errorf("an allowed call must carry no reason, got %q", decision.Reason)
	}
}

func TestCapability_Authorize_RefusesTheDimensionThatLeavesTheLease(t *testing.T) {
	lease := &Lease{
		Tools:         []string{"ops_exec"},
		Targets:       []string{"lab"},
		Commands:      []string{"unit_logs"},
		Shapes:        []string{"probe"},
		TracePrograms: []string{"runqlat"},
		ExpiresAt:     time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		call  Call
		wants string
	}{
		{
			name:  "tool",
			call:  Call{Tool: "ops_sandbox", Target: "lab", Command: "unit_logs"},
			wants: "ops_sandbox",
		},
		{
			name:  "host",
			call:  Call{Tool: "ops_exec", Target: "prod", Command: "unit_logs"},
			wants: "prod",
		},
		{
			name:  "command",
			call:  Call{Tool: "ops_exec", Target: "lab", Command: "pg_locks"},
			wants: "pg_locks",
		},
		{
			name:  "shape",
			call:  Call{Tool: "ops_exec", Target: "lab", Command: "unit_logs", Shape: "workshop"},
			wants: "workshop",
		},
		{
			name:  "trace program",
			call:  Call{Tool: "ops_exec", Target: "lab", Command: "unit_logs", TraceProgram: "biolatency"},
			wants: "biolatency",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := lease.Authorize(tc.call, now)

			if decision.Allowed() {
				t.Fatalf("expected a refusal, got allow")
			}
			if decision.Verdict != VerdictDeny {
				t.Errorf("expected verdict %q, got %q", VerdictDeny, decision.Verdict)
			}
			if !strings.Contains(decision.Reason, tc.wants) {
				t.Errorf("the reason must name %q so the model can act on it, got %q", tc.wants, decision.Reason)
			}
		})
	}
}

func TestCapability_Authorize_RefusesEverythingWithoutALease(t *testing.T) {
	var lease *Lease

	decision := lease.Authorize(Call{Tool: "ops_exec"}, time.Now())

	if decision.Allowed() {
		t.Fatal("a nil lease must refuse, not permit")
	}
	if !strings.Contains(decision.Reason, "没有能力租约") {
		t.Errorf("the refusal must say the lease is missing, got %q", decision.Reason)
	}
}

func TestCapability_Authorize_RefusesAnEmptyLease(t *testing.T) {
	lease := &Lease{ExpiresAt: time.Now().Add(time.Hour)}

	decision := lease.Authorize(Call{Tool: "ops_exec"}, time.Now())

	if decision.Allowed() {
		t.Fatal("a lease that names nothing must permit nothing")
	}
}

func TestCapability_Authorize_RefusesAnExpiredLease(t *testing.T) {
	expiry := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	lease := &Lease{
		Tools:     []string{"ops_exec"},
		Targets:   []string{"*"},
		ExpiresAt: expiry,
	}

	if decision := lease.Authorize(Call{Tool: "ops_exec"}, expiry.Add(-time.Second)); !decision.Allowed() {
		t.Fatalf("a lease one second before expiry must still pass, got %q", decision.Reason)
	}

	decision := lease.Authorize(Call{Tool: "ops_exec"}, expiry)
	if decision.Allowed() {
		t.Fatal("a lease at its expiry instant must refuse")
	}
	if !strings.Contains(decision.Reason, "过期") {
		t.Errorf("the refusal must say the lease expired, got %q", decision.Reason)
	}
}

func TestCapability_Authorize_TreatsAWildcardAsEveryValue(t *testing.T) {
	lease := &Lease{
		Tools:         []string{"*"},
		Targets:       []string{"*"},
		Commands:      []string{"*"},
		Shapes:        []string{"*"},
		TracePrograms: []string{"*"},
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	decision := lease.Authorize(Call{
		Tool:         "ops_sandbox",
		Target:       "prod",
		Command:      "pg_locks",
		Shape:        "workshop",
		TraceProgram: "execsnoop",
	}, time.Now())

	if !decision.Allowed() {
		t.Fatalf("a wildcard lease must permit the call, got %q", decision.Reason)
	}
}

func TestCapability_ParseMode_MapsEveryValueThatIsNotTheSwitchToOff(t *testing.T) {
	cases := map[string]Mode{
		"off":      ModeOff,
		"shadow":   ModeShadow,
		"enforce":  ModeEnforce,
		"ENFORCE ": ModeEnforce,
		"on":       ModeOff,
		"":         ModeOff,
		"bogus":    ModeOff,
	}

	for value, want := range cases {
		if got := ParseMode(value); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestCapability_Mode_EnablingAndEnforcingAreDifferentQuestions(t *testing.T) {
	if ModeOff.Enabled() || ModeOff.Enforcing() {
		t.Error("off must do neither")
	}
	if !ModeShadow.Enabled() || ModeShadow.Enforcing() {
		t.Error("shadow must record without refusing")
	}
	if !ModeEnforce.Enabled() || !ModeEnforce.Enforcing() {
		t.Error("enforce must do both")
	}
}

func TestCapability_Authorize_LiftsARefusalOnlyForTheCallAGrantNames(t *testing.T) {
	lease := &Lease{
		Tools:     []string{"ops_exec"},
		Targets:   []string{"lab"},
		Commands:  []string{"unit_logs", "memory"},
		Shapes:    []string{"probe"},
		ExpiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)
	grant := Grant{
		Tool:      "ops_exec",
		Target:    "prod",
		Command:   "unit_logs",
		Shape:     "probe",
		IssuedBy:  "operator",
		ExpiresAt: time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC),
	}

	allowed := lease.Authorize(Call{Tool: "ops_exec", Target: "prod", Command: "unit_logs", Shape: "probe"}, now, grant)
	if !allowed.Allowed() {
		t.Fatalf("an approved call was refused: %q (%s)", allowed.Verdict, allowed.Reason)
	}

	otherHost := lease.Authorize(Call{Tool: "ops_exec", Target: "staging", Command: "unit_logs", Shape: "probe"}, now, grant)
	if otherHost.Allowed() {
		t.Error("the grant named prod, so staging must still be refused")
	}

	otherCommand := lease.Authorize(Call{Tool: "ops_exec", Target: "prod", Command: "established", Shape: "probe"}, now, grant)
	if otherCommand.Allowed() {
		t.Error("the grant named unit_logs, so another command must still be refused")
	}
}

func TestCapability_Authorize_MarksOnlyWhatAnOperatorCanApprove(t *testing.T) {
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)
	lease := &Lease{
		Tools:     []string{"ops_exec"},
		Targets:   []string{"lab"},
		Commands:  []string{"unit_logs"},
		Shapes:    []string{"probe"},
		ExpiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}

	outside := lease.Authorize(Call{Tool: "ops_exec", Target: "prod", Command: "unit_logs", Shape: "probe"}, now)
	if !outside.NeedsApproval {
		t.Errorf("an out-of-lease host must be approvable, got %+v", outside)
	}

	unknownTool := lease.Authorize(Call{Tool: "terminal", Target: "lab"}, now)
	if unknownTool.NeedsApproval {
		t.Error("a tool the role never held is not something an approval can grant")
	}

	expired := &Lease{Tools: []string{"ops_exec"}, Targets: []string{"lab"}, ExpiresAt: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)}
	if decision := expired.Authorize(Call{Tool: "ops_exec", Target: "lab"}, now); decision.NeedsApproval {
		t.Error("an expired lease needs a new lease, not an approval")
	}
}

func TestCapability_Grant_Permits_RequiresEveryDimensionItNames(t *testing.T) {
	grant := Grant{Tool: "ops_sandbox", Shape: "workshop"}

	if !grant.Permits(Call{Tool: "ops_sandbox", Shape: "workshop"}) {
		t.Error("the grant names the call exactly, so it must permit it")
	}
	if grant.Permits(Call{Tool: "ops_sandbox", Shape: "probe"}) {
		t.Error("the grant named workshop, so probe is a different call")
	}
	if grant.Permits(Call{Tool: "ops_exec", Shape: "workshop"}) {
		t.Error("the grant named ops_sandbox, not ops_exec")
	}
}

func TestCapability_ApprovedBy_IgnoresAGrantThatHasExpired(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	grant := Grant{Tool: "ops_exec", Target: "prod", ExpiresAt: time.Date(2026, 11, 23, 11, 59, 0, 0, time.UTC)}

	if ApprovedBy([]Grant{grant}, Call{Tool: "ops_exec", Target: "prod"}, now) {
		t.Error("a grant that expired a minute ago must not lift anything")
	}
}

func TestCapability_Authorize_RefusesAWriteTheLeaseNeverNamed(t *testing.T) {
	// The default lease hands out reads only, and the built-in policy names no
	// action: a repair in a lease nobody widened is a denial an operator can lift.
	lease := &Lease{
		Tools:     []string{"ops_apply"},
		Targets:   []string{"lab"},
		Commands:  []string{"*"},
		ExpiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	decision := lease.Authorize(Call{
		Tool: "ops_apply", Target: "lab", Action: "container_restart api-1", Command: "container_restart api-1",
	}, now)

	if decision.Allowed() {
		t.Fatal("a lease with no actions permitted a write")
	}
	if !strings.Contains(decision.Reason, "container_restart api-1") {
		t.Errorf("the reason must name the write so the operator knows what is being asked, got %q", decision.Reason)
	}
	if !decision.NeedsApproval {
		t.Error("a write outside the lease is exactly what an operator can approve")
	}
}

func TestCapability_Authorize_JudgesAWriteByItsActionsNotByTheReadCatalogue(t *testing.T) {
	// A lease that opens the whole read-only catalogue still permits no write.
	lease := &Lease{
		Tools:     []string{"*"},
		Targets:   []string{"*"},
		Commands:  []string{"*"},
		ExpiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	decision := lease.Authorize(Call{
		Tool: "ops_apply", Target: "lab", Action: "disk_cleanup /var/log/nginx/", Command: "disk_cleanup /var/log/nginx/",
	}, now)

	if decision.Allowed() {
		t.Fatal("the read catalogue permitted a write")
	}
}

func TestCapability_Authorize_PermitsAWriteItsActionsListPatternCovers(t *testing.T) {
	// "container_restart *" is how an operator says "restart containers, nothing
	// else" without listing every name a host might carry.
	lease := &Lease{
		Tools:     []string{"ops_apply"},
		Targets:   []string{"lab"},
		Actions:   []string{"container_restart *"},
		ExpiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	permitted := lease.Authorize(Call{Tool: "ops_apply", Target: "lab", Action: "container_restart api-1"}, now)
	if !permitted.Allowed() {
		t.Fatalf("the pattern names this write, got refusal %q", permitted.Reason)
	}

	refused := lease.Authorize(Call{Tool: "ops_apply", Target: "lab", Action: "disk_cleanup /var/log/nginx/"}, now)
	if refused.Allowed() {
		t.Fatal("a cleanup was permitted by a list that only names container restarts")
	}
}

func TestCapability_Authorize_LiftsAWriteApprovalForThatOneObject(t *testing.T) {
	lease := &Lease{
		Tools:     []string{"ops_apply"},
		Targets:   []string{"lab"},
		ExpiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)
	grant := Grant{
		Tool:      "ops_apply",
		Target:    "lab",
		Command:   "container_restart api-1",
		ExpiresAt: time.Date(2026, 11, 23, 11, 30, 0, 0, time.UTC),
	}

	// The guard sets both: the row the operator reads quotes Command, and the
	// lease judges Action. They are the same string by construction.
	approved := lease.Authorize(Call{
		Tool: "ops_apply", Target: "lab", Action: "container_restart api-1", Command: "container_restart api-1",
	}, now, grant)
	if !approved.Allowed() {
		t.Fatalf("the approval covers this write, got refusal %q", approved.Reason)
	}

	other := lease.Authorize(Call{
		Tool: "ops_apply", Target: "lab", Action: "container_restart api-2", Command: "container_restart api-2",
	}, now, grant)
	if other.Allowed() {
		t.Fatal("one container restart lifted the refusal for a different container")
	}
}

func TestCapability_Policy_Issue_NeverLeavesADimensionNil(t *testing.T) {
	t.Parallel()

	lease := DefaultPolicy().Issue("diagnosis", 1, 2, "commander", time.Now())

	// The lease columns are NOT NULL and a nil slice is inserted as NULL, so an
	// unset dimension has to be an empty list or the guard never comes up.
	for name, values := range map[string][]string{
		"targets":  lease.Targets,
		"tools":    lease.Tools,
		"commands": lease.Commands,
		"shapes":   lease.Shapes,
		"traces":   lease.TracePrograms,
		"actions":  lease.Actions,
	} {
		if values == nil {
			t.Errorf("%s is nil, want an empty list", name)
		}
	}
}
