package opsremote

import (
	"strings"
	"testing"
)

func TestTraceCommand_CarriesTheCapabilitiesTracingNeedsAndNothingElse(t *testing.T) {
	t.Parallel()

	script, ok := LookupTrace("runqlat")
	if !ok {
		t.Fatal("the catalogue has no runqlat")
	}

	command, err := TraceCommand("debian:bookworm", script, 10, false)
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

	for _, never := range []string{"--privileged", "--pid=host", "docker.sock", "SYS_ADMIN", "SYS_PTRACE", "SYS_KILL", "/:/host"} {
		if strings.Contains(command, never) {
			t.Errorf("command carries %q, which would let a trace change the host: %s", never, command)
		}
	}
}

func TestTraceCommand_TakesEgressOnlyWhenTheOperatorAllowedIt(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("runqlat")

	open, err := TraceCommand("debian:bookworm", script, 10, true)
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
		if _, err := TraceCommand(image, script, 10, false); err == nil {
			t.Errorf("image %q was accepted", image)
		}
	}
}

func TestTraceCommand_RefusesAWindowOutsideTheCatalogue(t *testing.T) {
	t.Parallel()

	script, _ := LookupTrace("runqlat")

	for _, seconds := range []int{0, 2, TraceMaxSeconds + 1} {
		if _, err := TraceCommand("debian:bookworm", script, seconds, false); err == nil {
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
