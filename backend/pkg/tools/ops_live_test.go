//go:build live

package tools

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"opsagi/pkg/config"
	"opsagi/pkg/opsremote"
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

	tools := NewOpsTools(&config.Config{
		OpsTargets:  targets,
		OpsBPFImage: liveBPFImage(),
	})
	if !tools.IsAvailable() {
		t.Fatalf("OPSAGI_LIVE_TARGETS = %q, which names no usable host", targets)
	}

	return tools
}

// liveBPFImage mirrors the envDefault the binary boots with: a hand-built config
// leaves the field empty, and an empty image falls back to a plain base image
// that carries no bpftrace.
func liveBPFImage() string {
	if image := strings.TrimSpace(os.Getenv("OPSAGI_BPF_IMAGE")); image != "" {
		return image
	}

	return "quay.io/iovisor/bpftrace:latest"
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

func TestOpsTools_HandleContainers_LiveFilterStatesTheCountItMatched(t *testing.T) {
	tools := liveOpsTools(t)

	payload, err := json.Marshal(map[string]string{"target": "lab", "filter": "opslab-"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	answer, err := tools.HandleContainers(ctx, "ops_containers", payload)
	if err != nil {
		t.Fatalf("containers: %v", err)
	}

	header := regexp.MustCompile(`名字里包含 "opslab-" 的容器 (\d+) 个（这台主机上共 (\d+) 个）`).FindStringSubmatch(answer)
	if len(header) != 3 {
		t.Fatalf("answer = %q, want the match count in the header", answer)
	}

	matched, total := header[1], header[2]
	rows := strings.Count(answer, "\n  容器 ")
	if strconv.Itoa(rows) != matched {
		t.Errorf("header says %s matches but the listing carries %d rows:\n%s", matched, rows, answer)
	}
	if matched == total {
		t.Errorf("filter matched every one of the %s containers, so it proves nothing", total)
	}

	t.Logf("containers (%s of %s):\n%s", matched, total, answer)
}

func TestOpsTools_HandleTrace_LiveRunqlatPrintsTheSummary(t *testing.T) {
	tools := liveOpsTools(t)

	payload, err := json.Marshal(map[string]any{"target": "lab", "script": "runqlat", "seconds": 5})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	answer, err := tools.HandleTrace(ctx, "ops_trace", payload)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}

	if !strings.Contains(answer, "延迟汇总") {
		t.Errorf("answer = %q, want the summary line the report quotes", answer)
	}

	t.Logf("trace:\n%s", answer[max(0, len(answer)-700):])
}

// liveClient dials the first target the env names, for the few tests that have to
// change what the lab looks like before the tool can be exercised.
func liveClient(t *testing.T) *opsremote.Client {
	t.Helper()

	var targets []opsremote.Target
	if err := json.Unmarshal([]byte(os.Getenv("OPSAGI_LIVE_TARGETS")), &targets); err != nil {
		t.Fatalf("parse OPSAGI_LIVE_TARGETS: %v", err)
	}
	if len(targets) == 0 {
		t.Fatal("OPSAGI_LIVE_TARGETS names no target")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := opsremote.Dial(ctx, targets[0])
	if err != nil {
		t.Fatalf("dial %s: %v", targets[0].ID, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// The statement is written with a constant because psql has to be able to parse
// it; pg_stat_statements then normalises that constant into $1, which is the
// shape EXPLAIN (GENERIC_PLAN) cannot type.
const plantUntypablePlaceholder = `docker exec opslab-db psql -U postgres -d bench -q ` +
	`-c "select count(*) from pgbench_accounts where round(extract(epoch from now())) > 0" ` +
	`-c "select count(*) from pgbench_accounts where round(extract(epoch from now())) > 0" ` +
	`-c "select count(*) from pgbench_accounts where round(extract(epoch from now())) > 0"`

// The rank is read back rather than assumed: the same filter the tool applies,
// so a busy lab cannot shift the row out from under the assertion.
const plantedRankQuery = `docker exec opslab-db psql -U postgres -d bench -tAc "` +
	`select rank from (select row_number() over (order by total_exec_time desc) rank, query from pg_stat_statements ` +
	`where dbid = (select oid from pg_database where datname = current_database()) ` +
	`and ltrim(lower(query)) ~ '^(select|with|insert|update|delete|values|table)\y' ` +
	`and position('pg_stat_statements' in query) = 0) t where query like '%round(extract(%' limit 1"`

func TestOpsTools_HandleExec_LiveExplainSaysWhyAPlaceholderHasNoPlan(t *testing.T) {
	tools := liveOpsTools(t)
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if _, err := client.Run(ctx, plantUntypablePlaceholder); err != nil {
		t.Fatalf("plant: %v", err)
	}

	res, err := client.Run(ctx, plantedRankQuery)
	if err != nil {
		t.Fatalf("rank: %v", err)
	}

	rank, err := strconv.Atoi(strings.TrimSpace(res.Stdout))
	if err != nil {
		t.Fatalf("rank = %q, want the planted statement to have one (stdout %q)", res.Stdout, res.Stderr)
	}

	payload, err := json.Marshal(map[string]any{
		"target": "lab", "command": "pg_explain", "container": "opslab-db", "database": "bench", "rank": rank,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	answer, err := tools.HandleExec(ctx, "ops_exec", payload)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}

	// The label already says GENERIC_PLAN, so the assertion names the sentence
	// itself; otherwise a missing advice line would pass on the label alone.
	if !strings.Contains(answer, "推不出它们的类型") {
		t.Fatalf("answer = %q, want the sentence that explains the placeholder", answer)
	}

	t.Logf("explain (rank %d):\n%s", rank, answer[max(0, len(answer)-600):])
}
