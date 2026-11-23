package opsremote

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseFingerprint() Fingerprint {
	return Fingerprint{
		Label:       "before",
		CapturedAt:  time.Date(2026, 11, 6, 5, 30, 0, 0, time.UTC),
		Host:        "lab-1",
		Load1:       1.1,
		Load5:       1.0,
		CPUPercent:  47.3,
		MemUsedMB:   554,
		MemTotalMB:  1740,
		SwapUsedMB:  0,
		Filesystems: map[string]float64{"/": 12.0},
		Containers: map[string]FingerprintContainer{
			"opslab-cpu": {State: "running", Restarts: 0},
			"caddy":      {State: "running", Restarts: 0},
		},
	}
}

func findDelta(deltas []Delta, field string) (Delta, bool) {
	for _, d := range deltas {
		if d.Field == field {
			return d, true
		}
	}
	return Delta{}, false
}

func TestCompareFingerprints_CallsLargeCpuDropAnImprovement(t *testing.T) {
	before := baseFingerprint()
	after := before
	after.Label = "after"
	after.CPUPercent = 2.0

	deltas := CompareFingerprints(before, after)

	cpu, ok := findDelta(deltas, "cpu")
	require.True(t, ok, "cpu row must be present")
	assert.Equal(t, "改善", cpu.Verdict)
	assert.True(t, cpu.Material)
	assert.Equal(t, "-45.3% (-96%)", cpu.Change)
}

func TestCompareFingerprints_TreatsSmallChangeAsNoise(t *testing.T) {
	before := baseFingerprint()
	after := before
	after.CPUPercent = 48.5 // 1.2 points, below the 2.0 materiality floor

	deltas := CompareFingerprints(before, after)

	cpu, ok := findDelta(deltas, "cpu")
	require.True(t, ok)
	assert.Equal(t, "持平", cpu.Verdict)
	assert.False(t, cpu.Material)
}

func TestCompareFingerprints_FlagsGrowingRestartCountAsWorse(t *testing.T) {
	before := baseFingerprint()
	after := before
	after.Containers = map[string]FingerprintContainer{
		"opslab-cpu": {State: "running", Restarts: 0},
		"caddy":      {State: "running", Restarts: 3},
	}

	deltas := CompareFingerprints(before, after)

	row, ok := findDelta(deltas, "容器 caddy 重启")
	require.True(t, ok)
	assert.Equal(t, "恶化", row.Verdict)
	assert.True(t, row.Material)
	assert.Equal(t, "+3", row.Change)
}

func TestCompareFingerprints_ReportsAppearingAndDisappearingContainers(t *testing.T) {
	before := baseFingerprint()
	after := before
	after.Containers = map[string]FingerprintContainer{
		"opslab-cpu": {State: "exited"},
		"newcomer":   {State: "running"},
	}

	deltas := CompareFingerprints(before, after)

	caddy, ok := findDelta(deltas, "容器 caddy")
	require.True(t, ok)
	assert.Equal(t, "消失", caddy.Change)

	newcomer, ok := findDelta(deltas, "容器 newcomer")
	require.True(t, ok)
	assert.Equal(t, "新增", newcomer.Change)

	cpu, ok := findDelta(deltas, "容器 opslab-cpu 重启")
	require.True(t, ok)
	assert.Contains(t, cpu.Change, "状态 running→exited")
}

func TestFormatDeltas_SaysNothingChangedWhenAllRowsAreNoise(t *testing.T) {
	before := baseFingerprint()
	after := before
	after.Label = "after"
	after.CPUPercent = before.CPUPercent + 0.5

	output := FormatDeltas(before, after, CompareFingerprints(before, after))

	assert.Contains(t, output, "没有显著变化")
}

func TestFormatDeltas_LeadsWithTheCountOfMaterialChanges(t *testing.T) {
	before := baseFingerprint()
	after := before
	after.Label = "after"
	after.CPUPercent = 2.0

	output := FormatDeltas(before, after, CompareFingerprints(before, after))

	assert.Contains(t, output, "显著变化 1 项")
	assert.NotContains(t, output, "没有显著变化")
}

func TestFingerprint_ParsePostgresBrief_ReadsTheThreeNumbers(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   PostgresBrief
	}{
		{
			name:   "a waiting session and its longest wait",
			output: "1|3.812|0\n",
			want:   PostgresBrief{Container: "opslab-db", Database: "shop", BlockedSessions: 1, LongestWaitS: 3.812},
		},
		{
			name:   "nothing waiting",
			output: "0|0|0\n",
			want:   PostgresBrief{Container: "opslab-db", Database: "shop"},
		},
		{
			name:   "two waiters and a long transaction",
			output: "2|41.5|3\n",
			want:   PostgresBrief{Container: "opslab-db", Database: "shop", BlockedSessions: 2, LongestWaitS: 41.5, IdleInTransaction: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePostgresBrief("opslab-db", "shop", tt.output)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got != tt.want {
				t.Errorf("brief = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFingerprint_ParsePostgresBrief_RefusesAnOutputItCannotRead(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "a psql error is not a summary", output: "ERROR:  relation pg_stat_activity does not exist", want: "三段"},
		{name: "a missing column", output: "1|3.8", want: "三段"},
		{name: "a non-numeric count", output: "many|3.8|0", want: "等待会话数"},
		{name: "a non-numeric wait", output: "1|soon|0", want: "最长等待秒数"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePostgresBrief("opslab-db", "shop", tt.output)
			if err == nil {
				t.Fatal("parsed an output that is not a summary")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// A lock fix moves numbers no host metric carries, so the comparison needs them.
func TestFingerprint_CompareFingerprints_ReportsWhatTheLockFixMoved(t *testing.T) {
	before := Fingerprint{Host: "lab-host", Label: "before", Postgres: &PostgresBrief{
		Container: "opslab-db", Database: "shop", BlockedSessions: 1, LongestWaitS: 3.8, IdleInTransaction: 2,
	}}
	after := Fingerprint{Host: "lab-host", Label: "after", Postgres: &PostgresBrief{
		Container: "opslab-db", Database: "shop",
	}}

	rows := map[string]Delta{}
	for _, delta := range CompareFingerprints(before, after) {
		rows[delta.Field] = delta
	}

	blocked, ok := rows["锁等待会话"]
	if !ok {
		t.Fatalf("no row for the blocked sessions, got %v", rows)
	}
	if !blocked.Material || blocked.Verdict != "改善" {
		t.Errorf("row = %+v, want a material improvement", blocked)
	}
	if blocked.Before != "1.0" || blocked.After != "0.0" {
		t.Errorf("before/after = %s/%s, want 1.0/0.0", blocked.Before, blocked.After)
	}

	wait := rows["最长锁等待"]
	if !wait.Material || wait.Verdict != "改善" {
		t.Errorf("row = %+v, want a material improvement", wait)
	}

	idle := rows["长事务会话"]
	if !idle.Material || idle.Verdict != "改善" {
		t.Errorf("row = %+v, want a material improvement", idle)
	}
}

func TestFingerprint_CompareFingerprints_LeavesOutThePostgresRowsWhenOneSideHasNoSection(t *testing.T) {
	before := Fingerprint{Host: "lab-host", Postgres: &PostgresBrief{Container: "opslab-db", Database: "shop", BlockedSessions: 3}}
	after := Fingerprint{Host: "lab-host"}

	for _, delta := range CompareFingerprints(before, after) {
		if strings.HasPrefix(delta.Field, "锁等待") || strings.HasPrefix(delta.Field, "最长锁等待") || delta.Field == "长事务会话" {
			t.Errorf("compared %q although only one sample carried a postgres section", delta.Field)
		}
	}
}

func TestFingerprint_ParseQueryBrief_PicksTheHeaviestStatementWithTheFragment(t *testing.T) {
	const output = "41|900.0|21.95|0|select count(*) from pgbench_accounts where region = $1\n" +
		"12|9782.1|815.18|38|select aid, bid, abalance from pgbench_accounts where region = $1 order by abalance desc limit 10"

	brief, err := ParseQueryBrief("order by abalance desc", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if brief.Calls != 12 || brief.MeanMS != 815.18 || brief.TotalMS != 9782.1 || brief.Rows != 38 {
		t.Errorf("brief = %+v, want the second row", brief)
	}
	if !strings.Contains(brief.Statement, "order by abalance desc") {
		t.Errorf("statement = %q, want the matched text kept for the next sample", brief.Statement)
	}
}

func TestFingerprint_ParseQueryBrief_RefusesAFragmentNoRowCarries(t *testing.T) {
	_, err := ParseQueryBrief("orders", "3|9.0|3.00|1|select 1")
	if err == nil {
		t.Fatal("parse an output with no match: want an error")
	}
	if !strings.Contains(err.Error(), "orders") {
		t.Errorf("error = %q, want it to name the fragment", err)
	}
}

func TestFingerprint_CompareFingerprints_JudgesAPerformanceChangeByTheCallsMadeBetween(t *testing.T) {
	statement := "select aid, bid, abalance from pgbench_accounts where region = $1 order by abalance desc limit 10"
	before := Fingerprint{Host: "lab-host", Load1: 12.2, Postgres: &PostgresBrief{
		Container: "opslab-db",
		Database:  "bench",
		Query:     &QueryBrief{Statement: statement, Calls: 12, TotalMS: 9782.1, MeanMS: 815.18, Rows: 38},
	}}
	after := Fingerprint{Host: "lab-host", Load1: 14.8, Postgres: &PostgresBrief{
		Container: "opslab-db",
		Database:  "bench",
		Query:     &QueryBrief{Statement: statement, Calls: 15, TotalMS: 9782.7, MeanMS: 652.18, Rows: 95},
	}}

	rows := map[string]Delta{}
	for _, delta := range CompareFingerprints(before, after) {
		rows[delta.Field] = delta
	}

	mean, ok := rows["语句平均耗时"]
	if !ok {
		t.Fatalf("no row for the statement, got %v", rows)
	}
	if mean.Verdict != "改善" || !mean.Material {
		t.Errorf("row = %+v, want a material improvement", mean)
	}
	if mean.Before != "815.18ms（累计）" || mean.After != "0.20ms（本区间）" {
		t.Errorf("before/after = %s/%s, want 815.18ms（累计）/0.20ms（本区间）", mean.Before, mean.After)
	}
	if !strings.Contains(mean.Change, "本区间 3 次") {
		t.Errorf("change = %q, want it to name the three calls the mean covers", mean.Change)
	}

	load := rows["load1"]
	if load.Verdict != "恶化" {
		t.Errorf("row = %+v, want the load row to stay honest about the build", load)
	}
}

func TestFingerprint_CompareFingerprints_RefusesToAverageWithoutNewCalls(t *testing.T) {
	statement := "select a from t where id = $1"
	before := Fingerprint{Host: "lab-host", Postgres: &PostgresBrief{Container: "db", Database: "bench",
		Query: &QueryBrief{Statement: statement, Calls: 7, TotalMS: 28, MeanMS: 4}}}
	after := Fingerprint{Host: "lab-host", Postgres: &PostgresBrief{Container: "db", Database: "bench",
		Query: &QueryBrief{Statement: statement, Calls: 7, TotalMS: 28, MeanMS: 4}}}

	rows := map[string]Delta{}
	for _, delta := range CompareFingerprints(before, after) {
		rows[delta.Field] = delta
	}

	mean, ok := rows["语句平均耗时"]
	if !ok {
		t.Fatalf("no row for the statement, got %v", rows)
	}
	if mean.After != "没有新调用" {
		t.Errorf("after = %q, want it to say there was nothing to average", mean.After)
	}
	if mean.Verdict == "改善" {
		t.Errorf("row = %+v, want no improvement claimed from zero calls", mean)
	}
}

func TestFingerprint_CompareFingerprints_SaysSoWhenTheMatchedStatementMoved(t *testing.T) {
	before := Fingerprint{Host: "lab-host", Postgres: &PostgresBrief{Container: "db", Database: "bench",
		Query: &QueryBrief{Statement: "select a from t where id = $1", MeanMS: 4}}}
	after := Fingerprint{Host: "lab-host", Postgres: &PostgresBrief{Container: "db", Database: "bench",
		Query: &QueryBrief{Statement: "select b from t where id = $1", MeanMS: 3}}}

	rows := map[string]Delta{}
	for _, delta := range CompareFingerprints(before, after) {
		rows[delta.Field] = delta
	}

	moved, ok := rows["语句"]
	if !ok {
		t.Fatalf("no row telling the statements differ, got %v", rows)
	}
	if moved.Verdict != "变化" || !moved.Material {
		t.Errorf("row = %+v, want a material change", moved)
	}
	if _, compared := rows["语句平均耗时"]; compared {
		t.Error("compared the timings of two different statements")
	}
}
