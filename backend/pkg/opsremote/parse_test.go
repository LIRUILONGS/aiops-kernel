package opsremote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLoadAvg_ReadsThreeAverages(t *testing.T) {
	l1, l5, l15, err := ParseLoadAvg("0.84 0.29 0.10 1/121 112489\n")
	require.NoError(t, err)
	assert.InDelta(t, 0.84, l1, 1e-9)
	assert.InDelta(t, 0.29, l5, 1e-9)
	assert.InDelta(t, 0.10, l15, 1e-9)
}

func TestParseLoadAvg_RejectsShortInput(t *testing.T) {
	_, _, _, err := ParseLoadAvg("0.84 0.29")
	require.Error(t, err)
}

func TestParseMemInfo_ReadsMemAndSwapRows(t *testing.T) {
	const output = `              total        used        free      shared  buff/cache   available
Mem:           1740         365         344           2        1030        1206
Swap:             0           0           0
`

	mem, err := ParseMemInfo(output)
	require.NoError(t, err)
	assert.Equal(t, 1740, mem.TotalMB)
	assert.Equal(t, 365, mem.UsedMB)
	assert.Equal(t, 344, mem.FreeMB)
	assert.Equal(t, 1206, mem.AvailableMB)
	assert.Equal(t, 0, mem.SwapTotalMB)
	assert.InDelta(t, 30.7, mem.PercentUsed(), 0.1)
}

func TestParseMemInfo_ErrorsWithoutMemRow(t *testing.T) {
	_, err := ParseMemInfo("garbage\n")
	require.Error(t, err)
}

func TestParseDF_SkipsHeaderAndReadsMounts(t *testing.T) {
	const output = `Filesystem     1024-blocks    Used Available Capacity Mounted on
/dev/vda1         41933796 4616252  35177924      12% /
/dev/vdb1         10485760  102400  10383360       1% /data
`

	filesystems := ParseDF(output)
	require.Len(t, filesystems, 2)
	assert.Equal(t, "/", filesystems[0].MountPoint)
	assert.Equal(t, int64(41933796), filesystems[0].SizeKB)
	assert.InDelta(t, 12, filesystems[0].UsePercent, 1e-9)
	assert.Equal(t, "/data", filesystems[1].MountPoint)
}

func TestParsePS_ReadsCpuAndMemoryColumns(t *testing.T) {
	const output = ` 112489 sh               89.3  0.0  396
 114617 systemd           5.0  0.4 8420
 1460 argusagent         0.9  2.3 41200
`

	processes := ParsePS(output)
	require.Len(t, processes, 3)
	assert.Equal(t, 112489, processes[0].PID)
	assert.Equal(t, "sh", processes[0].Command)
	assert.InDelta(t, 89.3, processes[0].CPU, 1e-9)
	assert.Equal(t, "argusagent", processes[2].Command)
	assert.Equal(t, int64(41200), processes[2].RSSKB)
}

func TestParseCPUStat_ComputesBusyPercentFromTwoSamples(t *testing.T) {
	// First sample: 100 idle ticks of 200 total. Second: 50 more idle of 200
	// more total, so half of the delta was busy.
	const output = `cpu  100 0 0 100 0 0 0 0 0 0
cpu  150 0 0 150 0 0 0 0 0 0
`

	busy, err := ParseCPUStat(output)
	require.NoError(t, err)
	assert.InDelta(t, 50, busy, 1e-9)
}

func TestParseCPUStat_ErrorsWithoutTwoSamples(t *testing.T) {
	_, err := ParseCPUStat("cpu  100 0 0 100 0 0 0 0 0 0\n")
	require.Error(t, err)
}

func TestParseContainers_ReadsInspectFormat(t *testing.T) {
	const output = `/opslab-broken|10|restarting|1|nginx:latest
/opslab-slow|0|running|0|python:latest

`

	containers := ParseContainers(output)
	require.Len(t, containers, 2)
	assert.Equal(t, "opslab-broken", containers[0].Name)
	assert.Equal(t, 10, containers[0].RestartCount)
	assert.Equal(t, "restarting", containers[0].State)
	assert.Equal(t, 1, containers[0].ExitCode)
	assert.Equal(t, "nginx:latest", containers[0].Image)
}

func TestSplitSections_SeparatesMarkerBlocks(t *testing.T) {
	const output = `###opsagi:host
lab-host
###opsagi:load
0.5 0.4 0.3 1/100 42
###opsagi:mem
Mem: 1 2 3
`

	sections := SplitSections(output)
	assert.Equal(t, "lab-host", sections["host"])
	assert.Equal(t, "0.5 0.4 0.3 1/100 42", sections["load"])
	assert.Equal(t, "Mem: 1 2 3", sections["mem"])
	assert.NotContains(t, sections, "absent")
}

func TestValidName_RefusesShellMetacharacters(t *testing.T) {
	for _, name := range []string{"opslab-broken", "nginx_1", "a.b"} {
		assert.True(t, ValidName(name), "%q should be accepted", name)
	}
	for _, name := range []string{"", "bad name", "bad;rm -rf /", "bad$(id)", "bad|pipe", "a/b"} {
		assert.False(t, ValidName(name), "%q should be refused", name)
	}
}
