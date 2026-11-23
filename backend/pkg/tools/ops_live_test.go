//go:build live

package tools

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"opsagi/pkg/config"
)

// These run against a real host named by OPSAGI_LIVE_TARGETS, which carries the
// same JSON OPSAGI_TARGETS does. The default tier skips them: a CI runner has no
// lab, and a green run there must not read as proof that these tools work.
func liveOpsTools(t *testing.T) *opsTools {
	t.Helper()

	targets := strings.TrimSpace(os.Getenv("OPSAGI_LIVE_TARGETS"))
	if targets == "" {
		t.Skip("OPSAGI_LIVE_TARGETS is unset; the live ops tier needs a host to probe")
	}

	tools := NewOpsTools(&config.Config{OpsTargets: targets})
	if !tools.IsAvailable() {
		t.Fatalf("OPSAGI_LIVE_TARGETS = %q, which names no usable host", targets)
	}

	return tools
}

var sampleIDPattern = regexp.MustCompile(`采样编号 (base-\d+)`)

func baselineOnce(t *testing.T, tools *opsTools, args map[string]string) (string, string) {
	t.Helper()

	payload, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	answer, err := tools.HandleBaseline(ctx, "ops_baseline", payload)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	sample := sampleIDPattern.FindStringSubmatch(answer)
	if len(sample) != 2 {
		t.Fatalf("answer = %q, want a sample id in it", answer)
	}

	return sample[1], answer
}

func TestOpsTools_HandleBaseline_LiveStatementSectionReachesTheDiff(t *testing.T) {
	tools := liveOpsTools(t)

	args := map[string]string{
		"target":    "lab",
		"container": "opslab-db",
		"database":  "bench",
		"statement": "order by abalance desc",
		"label":     "live-before",
	}

	before, answer := baselineOnce(t, tools, args)
	if !strings.Contains(answer, "语句段") {
		t.Fatalf("answer = %q, want the statement section in it", answer)
	}

	args["label"] = "live-after"
	after, _ := baselineOnce(t, tools, args)

	payload, err := json.Marshal(map[string]string{"before": before, "after": after})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	diff, err := tools.HandleDiff(ctx, "ops_diff", payload)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	if !strings.Contains(diff, "语句平均耗时") {
		t.Errorf("diff = %q, want the statement row the two samples carry", diff)
	}

	t.Logf("diff:\n%s", diff)
}
