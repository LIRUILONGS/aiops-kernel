//go:build live

package opsremote

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// These run against the host named by OPSAGI_LIVE_TARGETS, which carries the
// same JSON OPSAGI_TARGETS does. The default tier skips them: a runner has no
// lab, and a green run there must not read as proof that a pinned handshake
// works.
func liveTarget(t *testing.T) Target {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("OPSAGI_LIVE_TARGETS"))
	if raw == "" {
		t.Skip("OPSAGI_LIVE_TARGETS is unset; the live ops tier needs a host to reach")
	}

	var targets []Target
	if err := json.Unmarshal([]byte(raw), &targets); err != nil || len(targets) == 0 {
		t.Fatalf("OPSAGI_LIVE_TARGETS = %q, which is not a usable target list", raw)
	}

	return targets[0]
}

// hostPins reads the fingerprints the host publishes. A host usually has one
// key per algorithm and presents only one of them, so the test learns which pin
// is the right one by trying each.
func hostPins(t *testing.T, target Target) []string {
	t.Helper()

	probe, err := Dial(context.Background(), target)
	if err != nil {
		t.Fatalf("dial without a pin: %v", err)
	}
	defer func() { _ = probe.Close() }()

	listed, err := probe.Run(context.Background(), "for f in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf \"$f\" 2>/dev/null; done")
	if err != nil {
		t.Fatalf("read the host key fingerprints: %v", err)
	}

	pins := make([]string, 0, 3)
	for _, line := range strings.Split(listed.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.Contains(fields[1], ":") {
			pins = append(pins, fields[1])
		}
	}
	if len(pins) == 0 {
		t.Fatalf("the host published no host key fingerprint: %q", listed.Stdout)
	}

	return pins
}

func TestOpsRemote_HostKey_LiveThePresentedKeyIsTheOnlyOneAccepted(t *testing.T) {
	target := liveTarget(t)

	accepted := 0
	for _, pin := range hostPins(t, target) {
		pinned := target
		pinned.HostKeyFingerprint = pin

		client, err := Dial(context.Background(), pinned)
		if err != nil {
			continue
		}
		_ = client.Close()
		accepted++
	}

	if accepted != 1 {
		t.Fatalf("%d of the host's own pins were accepted, want exactly the one key the host presents", accepted)
	}
}

func TestOpsRemote_HostKey_LiveAStrangePinIsRefused(t *testing.T) {
	target := liveTarget(t)
	target.HostKeyFingerprint = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err := Dial(context.Background(), target)
	if err == nil {
		t.Fatal("a pin that matches nothing was accepted")
	}
	if !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("refusal = %v, want it to name the mismatch", err)
	}
}
