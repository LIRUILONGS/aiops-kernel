package opsremote

import (
	"strings"
	"testing"
)

func TestSandboxCommand_SpendsTheGuaranteeOnFlagsAndNotOnTheScript(t *testing.T) {
	t.Parallel()

	command := SandboxCommand(SandboxImageDefault)

	required := []string{
		"--network=none",
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--pids-limit=256",
		"--memory=256m",
		"--cpus=1",
		"-v /proc:/host/proc:ro",
		"-v /sys:/host/sys:ro",
	}
	for _, flag := range required {
		if !strings.Contains(command, flag) {
			t.Errorf("command is missing %q, so the box is not read-only: %s", flag, command)
		}
	}

	forbidden := []string{"--privileged", "--pid=host", "docker.sock", "--network=host"}
	for _, flag := range forbidden {
		if strings.Contains(command, flag) {
			t.Errorf("command carries %q, which hands the host back to the script: %s", flag, command)
		}
	}

	if !strings.HasSuffix(command, " "+SandboxImageDefault+" sh -s") {
		t.Errorf("the script must arrive on stdin, command ends with %q", command)
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
