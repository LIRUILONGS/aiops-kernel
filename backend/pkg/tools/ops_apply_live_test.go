//go:build live

package tools

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"opsagi/pkg/config"
)

// A repair changes the host, so the live tier refuses to run one unless the
// operator names the object it may restart: OPSAGI_LIVE_APPLY_CONTAINER. The
// container has to be one whose restart costs nothing; the test asserts the
// restart happened, not that the service came back healthy.
func TestOpsApply_HandleApply_LiveARestartReachesTheHostAndChangesTheStartTime(t *testing.T) {
	targets := strings.TrimSpace(os.Getenv("OPSAGI_LIVE_TARGETS"))
	if targets == "" {
		t.Skip("OPSAGI_LIVE_TARGETS is unset; the live ops tier needs a host to repair")
	}

	container := strings.TrimSpace(os.Getenv("OPSAGI_LIVE_APPLY_CONTAINER"))
	if container == "" {
		t.Skip("OPSAGI_LIVE_APPLY_CONTAINER is unset; a live repair restarts whatever it names")
	}

	tools := NewOpsTools(&config.Config{OpsTargets: targets}).WithWrites(true)
	if !tools.IsAvailable() {
		t.Fatalf("OPSAGI_LIVE_TARGETS = %q, which names no usable host", targets)
	}

	before := liveContainerState(t, tools, container)

	result, err := tools.HandleApply(t.Context(), OpsApplyToolName, liveApplyArgs(t, container))
	if err != nil {
		t.Fatalf("HandleApply: %v", err)
	}
	if !strings.Contains(result, "== 变更结论：") {
		t.Errorf("the answer carries no verdict, got %q", result)
	}
	if strings.Contains(result, "（unverified") {
		t.Errorf("the restart could not be measured, so the loop proved nothing: %q", result)
	}

	after := liveContainerState(t, tools, container)
	if before == after {
		t.Errorf("the container state did not change: %q before and after", before)
	}
	if strings.Contains(before, "running") && !strings.Contains(after, "running") {
		t.Errorf("the container is not running after the restart: %q", after)
	}
}

// liveContainerState reads the state and start time through the read-only
// catalogue, so the comparison uses the same path an agent would.
func liveContainerState(t *testing.T, tools *opsTools, container string) string {
	t.Helper()

	raw, err := json.Marshal(map[string]string{"command": "docker_inspect", "container": container})
	if err != nil {
		t.Fatalf("marshal inspect args: %v", err)
	}

	out, err := tools.HandleExec(t.Context(), OpsExecToolName, raw)
	if err != nil {
		t.Fatalf("docker_inspect %s: %v", container, err)
	}

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "StartedAt") || strings.Contains(line, "启动") {
			return strings.TrimSpace(line)
		}
	}

	return strings.TrimSpace(out)
}

func liveApplyArgs(t *testing.T, container string) []byte {
	t.Helper()

	raw, err := json.Marshal(OpsApplyArgs{Action: "container_restart", Container: container})
	if err != nil {
		t.Fatalf("marshal apply args: %v", err)
	}

	return raw
}
