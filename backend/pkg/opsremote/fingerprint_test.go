package opsremote

import (
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
