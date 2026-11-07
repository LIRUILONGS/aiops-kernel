package opsremote

import (
	"strings"
	"testing"
)

func TestSandboxCommand_TakesAShapeAndKeepsTheHostReadOnlyInBoth(t *testing.T) {
	t.Parallel()

	probe, err := SandboxCommand(SandboxImageDefault, SandboxModeProbe)
	if err != nil {
		t.Fatalf("probe shape: %v", err)
	}

	workshop, err := SandboxCommand(SandboxImageDefault, SandboxModeWorkshop)
	if err != nil {
		t.Fatalf("workshop shape: %v", err)
	}

	shared := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--pids-limit=256",
		"-v /proc:/host/proc:ro",
		"-v /sys:/host/sys:ro",
	}
	neverAllowed := []string{"--privileged", "--pid=host", "docker.sock", "-v /proc:/host/proc "}
	hostWritable := []string{"-v /proc:/host/proc:rw", "-v /sys:/host/sys:rw", "/:/host"}

	for _, command := range []string{probe, workshop} {
		for _, flag := range shared {
			if !strings.Contains(command, flag) {
				t.Errorf("shape is missing %q, so the host is not read-only: %s", flag, command)
			}
		}
		for _, flag := range append(neverAllowed, hostWritable...) {
			if strings.Contains(command, flag) {
				t.Errorf("shape carries %q, which hands the host to the script: %s", flag, command)
			}
		}
		if !strings.HasSuffix(command, " "+SandboxImageDefault+" sh -s") {
			t.Errorf("the script must arrive on stdin, command ends with %q", command)
		}
	}

	for _, flag := range []string{"--network=none", "--read-only"} {
		if !strings.Contains(probe, flag) {
			t.Errorf("the probe shape must carry %q: %s", flag, probe)
		}
		if strings.Contains(workshop, flag) {
			t.Errorf("the workshop shape must not carry %q, or nothing can be installed: %s", flag, workshop)
		}
	}
}

func TestSandboxCommand_RefusesAShapeItDoesNotKnow(t *testing.T) {
	t.Parallel()

	command, err := SandboxCommand(SandboxImageDefault, SandboxMode("root"))
	if err == nil {
		t.Fatalf("got command %q and no error, want a refusal", command)
	}
	if !strings.Contains(err.Error(), "未知的采集容器形状") {
		t.Errorf("got error %q, want it to name the unknown shape", err)
	}
}

func TestValidateSandboxImage_RefusesWhatIsNotAnImageName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		image string
		ok    bool
	}{
		{name: "default", image: "", ok: true},
		{name: "debian", image: "debian:latest", ok: true},
		{name: "registry with port", image: "registry.local:5000/tools/probe:1", ok: true},
		{name: "flag injection", image: "debian --privileged", ok: false},
		{name: "command substitution", image: "debian:$(id)", ok: false},
		{name: "newline", image: "debian:latest\nsh -c", ok: false},
		{name: "leading dash", image: "-v/:/host", ok: false},
		{name: "semicolon", image: "debian;reboot", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ValidateSandboxImage(tc.image)
			if tc.ok {
				if err != nil {
					t.Fatalf("image %q refused: %v", tc.image, err)
				}
				if got == "" {
					t.Fatalf("image %q accepted but came back empty", tc.image)
				}
				return
			}

			if err == nil {
				t.Fatalf("image %q accepted as %q, want a refusal", tc.image, got)
			}
		})
	}
}

func TestImageRefusal_LetsTheAgentSeeWhatTheHostAlreadyHas(t *testing.T) {
	t.Parallel()

	if err := imageRefusal("debian:latest", true, false, nil); err != nil {
		t.Fatalf("an image that is present was refused: %v", err)
	}
	if err := imageRefusal("debian:latest", false, true, nil); err != nil {
		t.Fatalf("pulling is allowed but the image was refused: %v", err)
	}

	err := imageRefusal("alpine:latest", false, false, []string{"debian:latest", "postgres:16"})
	if err == nil {
		t.Fatal("an image that is absent without pull rights was accepted")
	}
	for _, want := range []string{"alpine:latest", "OPSAGI_SANDBOX_PULL", "debian:latest", "postgres:16"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err.Error(), want)
		}
	}

	empty := imageRefusal("alpine:latest", false, false, nil)
	if empty == nil || !strings.Contains(empty.Error(), "没有可用的镜像") {
		t.Errorf("refusal %v does not say the host has nothing to offer", empty)
	}
}

