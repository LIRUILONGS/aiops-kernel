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

func TestValidateSandboxImage_RefusesWhatTheOperatorNeverAllowed(t *testing.T) {
	t.Parallel()

	allowed := []string{SandboxImageDefault, "quay.io/iovisor/bcc:latest"}

	tests := []struct {
		name    string
		image   string
		want    string
		wantErr string
	}{
		{"the default is always allowed", SandboxImageDefault, SandboxImageDefault, ""},
		{"an empty name falls back to the default", "", SandboxImageDefault, ""},
		{"an allowlisted image passes", "quay.io/iovisor/bcc:latest", "quay.io/iovisor/bcc:latest", ""},
		{"an image outside the list is refused", "alpine:3.19", "", "不在允许列表"},
		{"a name with a space is refused", "debian:latest ; rm -rf /", "", "含不允许的字符"},
		{"a name with a dollar is refused", "$(id)", "", "含不允许的字符"},
		{"a name with a newline is refused", "debian\n:latest", "", "含不允许的字符"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ValidateSandboxImage(tc.image, allowed)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("got %q and no error, want an error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got error %q, want it to contain %q", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
