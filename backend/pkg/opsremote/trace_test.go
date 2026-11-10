package opsremote

import (
	"context"
	"strings"
	"testing"
)

func TestTraceCommand_CarriesTheCapabilitiesTracingNeedsAndNothingElse(t *testing.T) {
	t.Parallel()

	script, ok := LookupTrace("runqlat")
	if !ok {
		t.Fatal("the catalogue has no runqlat")
	}

	command, err := TraceCommand("debian:bookworm", script, 10, TraceOptions{})
	if err != nil {
		t.Fatalf("trace command: %v", err)
	}

	for _, want := range []string{
		"--cap-drop=ALL",
		"--cap-add=BPF",
		"--cap-add=PERFMON",
		"--cap-add=SYS_RESOURCE",
		"--security-opt=no-new-privileges",
		"--network=none",
		"--read-only",
		"-v /sys/kernel/debug:/sys/kernel/debug:ro",
		"-v /sys/kernel/tracing:/sys/kernel/tracing:ro",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("command is missing %q: %s", want, command)
		}
	}

	for _, never := range []string{"--privileged", "--pid=host", "docker.sock", "SYS_ADMIN", "SYS_PTRACE", "SYS_KILL", "/:/host", "/proc:/proc"} {
		if strings.Contains(command, never) {
			t.Errorf("command carries %q, which would let a trace change the host: %s", never, command)
		}
	}
}

func TestTraceCommand_TakesEgressOnlyWhenTheOperatorAllowedIt(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("runqlat")

	open, err := TraceCommand("debian:bookworm", script, 10, TraceOptions{Network: true})
	if err != nil {
		t.Fatalf("trace command with egress: %v", err)
	}
	if !strings.Contains(open, "--network=bridge") || strings.Contains(open, "--network=none") {
		t.Errorf("command = %q, want egress and not a closed network", open)
	}

	if !strings.Contains(open, "--cap-drop=ALL") {
		t.Errorf("egress dropped the capability discipline: %q", open)
	}
}

func TestTraceCommand_RefusesAnImageThatIsNotAName(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("runqlat")

	for _, image := range []string{"debian; reboot", "debian:bookworm --privileged", ""} {
		if _, err := TraceCommand(image, script, 10, TraceOptions{}); err == nil {
			t.Errorf("image %q was accepted", image)
		}
	}
}

func TestTraceCommand_RefusesAWindowOutsideTheCatalogue(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("runqlat")

	for _, seconds := range []int{0, 2, TraceMaxSeconds + 1} {
		if _, err := TraceCommand("debian:bookworm", script, seconds, TraceOptions{}); err == nil {
			t.Errorf("a %d second window was accepted", seconds)
		}
	}
}

func TestTraceProgram_RunsTheCatalogueTextAndExitsOnItsOwn(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("execsnoop")

	program := TraceProgram(script, 7)
	if !strings.Contains(program, script.Program) {
		t.Errorf("program does not carry the catalogue text:\n%s", program)
	}
	if !strings.Contains(program, "interval:s:7 { exit(); }") {
		t.Errorf("program does not bound itself to the window:\n%s", program)
	}
	if !strings.Contains(program, "exit 127") {
		t.Errorf("program does not refuse loudly when bpftrace is missing:\n%s", program)
	}
}

func TestTraceScripts_AreAddressableAndListedInOrder(t *testing.T) {
	t.Parallel()

	scripts := TraceScripts()
	if len(scripts) < 4 {
		t.Fatalf("catalogue has %d scripts, want the kernel questions a diagnosis needs", len(scripts))
	}

	seen := map[string]bool{}
	for _, script := range scripts {
		if script.ID == "" || script.Program == "" || script.Description == "" {
			t.Errorf("script %+v is incomplete", script)
		}
		if seen[script.ID] {
			t.Errorf("script id %q is listed twice", script.ID)
		}
		seen[script.ID] = true

		if _, ok := LookupTrace(script.ID); !ok {
			t.Errorf("script %q is listed but cannot be looked up", script.ID)
		}
	}

	if _, ok := LookupTrace("run_whatever"); ok {
		t.Error("an id outside the catalogue resolved")
	}

	ids := TraceIDs()
	for id := range seen {
		if !strings.Contains(ids, id) {
			t.Errorf("id %q is missing from the list the error message shows: %s", id, ids)
		}
	}
}

func TestClampTraceSeconds_KeepsTheWindowInsideTheCatalogue(t *testing.T) {
	t.Parallel()

	if got := ClampTraceSeconds(0); got != TraceDefaultSeconds {
		t.Errorf("ClampTraceSeconds(0) = %d, want the default", got)
	}
	if got := ClampTraceSeconds(5); got != 5 {
		t.Errorf("ClampTraceSeconds(5) = %d, want 5", got)
	}
	if got := ClampTraceSeconds(TraceMaxSeconds + 60); got != TraceMaxSeconds {
		t.Errorf("ClampTraceSeconds(over) = %d, want the ceiling", got)
	}
}

func TestTraceCommand_EarnsSysAdminOnlyWhenItEscalates(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("runqlat")

	narrow, err := TraceCommand("debian:bookworm", script, 10, TraceOptions{})
	if err != nil {
		t.Fatalf("narrow command: %v", err)
	}
	if strings.Contains(narrow, "SYS_ADMIN") {
		t.Errorf("the first attempt already carries SYS_ADMIN: %s", narrow)
	}

	wide, err := TraceCommand("debian:bookworm", script, 10, TraceOptions{Escalate: true})
	if err != nil {
		t.Fatalf("wide command: %v", err)
	}
	if !strings.Contains(wide, "--cap-add=SYS_ADMIN") {
		t.Errorf("the retry does not carry SYS_ADMIN: %s", wide)
	}
	if !strings.HasPrefix(wide, "docker run") || !strings.Contains(wide, "--cap-drop=ALL") {
		t.Errorf("the retry is not the same shape with one capability more: %s", wide)
	}
	if got, want := strings.Count(wide, "--cap-add="), strings.Count(narrow, "--cap-add=")+1; got != want {
		t.Errorf("the retry added %d capabilities, want 1", got-strings.Count(narrow, "--cap-add="))
	}
}

func TestTraceGate_RecognisesTheOneFailureAWiderCapabilityFixes(t *testing.T) {
	t.Parallel()

	gated := Result{
		ExitCode: 255,
		Stderr:   "perf_event_open(/sys/kernel/debug/tracing/events/sched/sched_wakeup/id): Permission denied\nERROR: Error attaching probe",
	}
	if !TraceGate(gated) {
		t.Error("the perf_event_open refusal was not recognised, so the retry never happens")
	}

	for _, other := range []Result{
		{Stderr: "bpftrace 不在镜像里。"},
		{Stderr: "tracepoint not found: sched:sched_wakeup"},
		{Stderr: "Permission denied (publickey)"},
		{Stdout: "@us[x]: [1, 2) 1 |@@|"},
	} {
		if TraceGate(other) {
			t.Errorf("result %+v would be escalated for no reason", other)
		}
	}
}

func TestClient_RunTrace_RetriesOnceWithSysAdminWhenTheKernelRefuses(t *testing.T) {
	t.Parallel()

	var commands []string
	client := &Client{runStdin: func(_ context.Context, command, _ string) (Result, error) {
		commands = append(commands, command)
		if strings.Contains(command, "SYS_ADMIN") {
			return Result{ExitCode: 0, Stdout: "@us[x]: [8, 16) 1 |@@|"}, nil
		}

		return Result{ExitCode: 255, Stderr: "perf_event_open(/sys/kernel/debug/tracing/events/x/id): Permission denied"}, nil
	}}

	script, _ := LookupTrace("runqlat")

	res, err := client.RunTrace(t.Context(), "debian:bookworm", script, 10, TraceOptions{})
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("ran %d times, want a retry exactly once", len(commands))
	}
	if res.Stdout == "" || res.ExitCode != 0 {
		t.Errorf("the retry did not keep the working result: %+v", res)
	}
}

func TestClient_RunTrace_DoesNotRetryAnErrorAWiderCapabilityCannotFix(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &Client{runStdin: func(_ context.Context, _, _ string) (Result, error) {
		attempts++

		return Result{ExitCode: 127, Stderr: "bpftrace 不在镜像里。"}, nil
	}}

	script, _ := LookupTrace("runqlat")

	if _, err := client.RunTrace(t.Context(), "debian:bookworm", script, 10, TraceOptions{}); err != nil {
		t.Fatalf("trace: %v", err)
	}
	if attempts != 1 {
		t.Errorf("ran %d times, want the failure reported as it is", attempts)
	}
}

func TestClient_RunTrace_DoesNotEscalateWhenTheCallerDidNotAsk(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &Client{runStdin: func(_ context.Context, _, _ string) (Result, error) {
		attempts++

		return Result{ExitCode: 0, Stdout: "ok"}, nil
	}}

	script, _ := LookupTrace("execsnoop")

	if _, err := client.RunTrace(t.Context(), "debian:bookworm", script, 10, TraceOptions{Escalate: true}); err != nil {
		t.Fatalf("trace: %v", err)
	}
	if attempts != 1 {
		t.Errorf("ran %d times, want the caller's choice respected", attempts)
	}
}
