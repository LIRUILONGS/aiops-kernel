package opsremote

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fingerprint is a compact, comparable record of one host at one moment.
// It exists so a before/after comparison is arithmetic instead of a model's
// recollection: an LLM asked to diff two dumps will invent deltas.
type Fingerprint struct {
	Label       string                          `json:"label"`
	CapturedAt  time.Time                       `json:"captured_at"`
	Host        string                          `json:"host"`
	Load1       float64                         `json:"load1"`
	Load5       float64                         `json:"load5"`
	Load15      float64                         `json:"load15"`
	CPUPercent  float64                         `json:"cpu_percent"`
	MemUsedMB   int                             `json:"mem_used_mb"`
	MemTotalMB  int                             `json:"mem_total_mb"`
	SwapUsedMB  int                             `json:"swap_used_mb"`
	Filesystems map[string]float64              `json:"filesystems"`
	Containers  map[string]FingerprintContainer `json:"containers"`
	TopCPU      []ProcessBrief                  `json:"top_cpu"`
	TopMem      []ProcessBrief                  `json:"top_mem"`
	Postgres    *PostgresBrief                  `json:"postgres,omitempty"`
}

// PostgresBrief is the database-side half of a fingerprint: the numbers a lock
// problem moves, which no host metric carries. It is absent unless the operator
// named a container and a database, because those are target-specific.
type PostgresBrief struct {
	Container         string  `json:"container"`
	Database          string  `json:"database"`
	BlockedSessions   int     `json:"blocked_sessions"`
	LongestWaitS      float64 `json:"longest_wait_s"`
	IdleInTransaction int     `json:"idle_in_transaction"`
}

// ParsePostgresBrief reads the three numbers the summary command prints. The
// command is a catalogue entry, so its shape is fixed here rather than guessed.
func ParsePostgresBrief(container, database, output string) (PostgresBrief, error) {
	fields := strings.Split(strings.TrimSpace(output), "|")
	if len(fields) != 3 {
		return PostgresBrief{}, fmt.Errorf("锁等待摘要应有三段，收到 %d 段：%q", len(fields), strings.TrimSpace(output))
	}

	blocked, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return PostgresBrief{}, fmt.Errorf("解析等待会话数失败：%w", err)
	}

	wait, err := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
	if err != nil {
		return PostgresBrief{}, fmt.Errorf("解析最长等待秒数失败：%w", err)
	}

	idle, err := strconv.Atoi(strings.TrimSpace(fields[2]))
	if err != nil {
		return PostgresBrief{}, fmt.Errorf("解析长事务会话数失败：%w", err)
	}

	return PostgresBrief{
		Container:         container,
		Database:          database,
		BlockedSessions:   blocked,
		LongestWaitS:      round3(wait),
		IdleInTransaction: idle,
	}, nil
}

func round3(value float64) float64 {
	return math.Round(value*1000) / 1000
}

// FingerprintContainer keeps only what changes between two samples.
type FingerprintContainer struct {
	State    string `json:"state"`
	Restarts int    `json:"restarts"`
	ExitCode int    `json:"exit_code"`
}

// ProcessBrief names a process and its share of the host.
type ProcessBrief struct {
	Command string  `json:"command"`
	PID     int     `json:"pid"`
	CPU     float64 `json:"cpu"`
	Memory  float64 `json:"mem"`
	RSSKB   int64   `json:"rss_kb"`
}

// SnapshotFingerprint reduces a SystemSnapshot plus a container list to the
// fields worth comparing across a change.
func SnapshotFingerprint(label string, snap SystemSnapshot, containers []Container) Fingerprint {
	fp := Fingerprint{
		Label:       label,
		CapturedAt:  snap.CollectedAt,
		Host:        snap.Hostname,
		Load1:       snap.Load1,
		Load5:       snap.Load5,
		Load15:      snap.Load15,
		CPUPercent:  snap.CPUUsage,
		MemUsedMB:   snap.Memory.TotalMB - snap.Memory.AvailableMB,
		MemTotalMB:  snap.Memory.TotalMB,
		SwapUsedMB:  snap.Memory.SwapUsedMB,
		Filesystems: map[string]float64{},
		Containers:  map[string]FingerprintContainer{},
	}

	for _, fs := range snap.Filesystems {
		fp.Filesystems[fs.MountPoint] = round1(fs.UsePercent)
	}
	for _, c := range containers {
		fp.Containers[c.Name] = FingerprintContainer{State: c.State, Restarts: c.RestartCount, ExitCode: c.ExitCode}
	}

	byCPU := make([]Process, len(snap.TopProcesses))
	copy(byCPU, snap.TopProcesses)
	sort.SliceStable(byCPU, func(i, j int) bool { return byCPU[i].CPU > byCPU[j].CPU })
	for i, p := range byCPU {
		if i >= 5 {
			break
		}
		fp.TopCPU = append(fp.TopCPU, ProcessBrief{Command: p.Command, PID: p.PID, CPU: round1(p.CPU), Memory: round1(p.Memory), RSSKB: p.RSSKB})
	}

	byMem := make([]Process, len(snap.TopProcesses))
	copy(byMem, snap.TopProcesses)
	sort.SliceStable(byMem, func(i, j int) bool { return byMem[i].RSSKB > byMem[j].RSSKB })
	for i, p := range byMem {
		if i >= 5 {
			break
		}
		fp.TopMem = append(fp.TopMem, ProcessBrief{Command: p.Command, PID: p.PID, CPU: round1(p.CPU), Memory: round1(p.Memory), RSSKB: p.RSSKB})
	}

	return fp
}

// Delta is one compared field. Direction is from the caller's point of view:
// a higher CPU is worse, a higher free memory is better.
type Delta struct {
	Field    string `json:"field"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Change   string `json:"change"`
	Verdict  string `json:"verdict"`
	Material bool   `json:"material"`
}

// CompareNumeric turns two values into a delta row with a materiality floor, so
// noise does not read as improvement.
func compareNumeric(field string, before, after, floor float64, unit string, higherIsWorse bool) Delta {
	change := after - before
	pct := 0.0
	if before != 0 {
		pct = change / math.Abs(before) * 100
	}

	verdict := "持平"
	material := math.Abs(change) >= floor
	if material {
		worse := change > 0
		if !higherIsWorse {
			worse = change < 0
		}
		if worse {
			verdict = "恶化"
		} else {
			verdict = "改善"
		}
	}

	return Delta{
		Field:    field,
		Before:   fmt.Sprintf("%.1f%s", before, unit),
		After:    fmt.Sprintf("%.1f%s", after, unit),
		Change:   fmt.Sprintf("%+.1f%s (%+.0f%%)", change, unit, pct),
		Verdict:  verdict,
		Material: material,
	}
}

// CompareFingerprints diffs two samples. Every row is deterministic: the same
// two inputs always produce the same table, which is what makes it usable as
// evidence rather than as narration.
func CompareFingerprints(before, after Fingerprint) []Delta {
	deltas := []Delta{
		compareNumeric("load1", before.Load1, after.Load1, 0.10, "", true),
		compareNumeric("load5", before.Load5, after.Load5, 0.10, "", true),
		compareNumeric("cpu", before.CPUPercent, after.CPUPercent, 2.0, "%", true),
		compareNumeric("内存占用", float64(before.MemUsedMB), float64(after.MemUsedMB), 32, "MB", true),
		compareNumeric("swap 使用", float64(before.SwapUsedMB), float64(after.SwapUsedMB), 1, "MB", true),
	}

	if before.Postgres != nil && after.Postgres != nil {
		deltas = append(deltas,
			compareNumeric("锁等待会话", float64(before.Postgres.BlockedSessions), float64(after.Postgres.BlockedSessions), 1, "", true),
			compareNumeric("最长锁等待", before.Postgres.LongestWaitS, after.Postgres.LongestWaitS, 0.5, "s", true),
			compareNumeric("长事务会话", float64(before.Postgres.IdleInTransaction), float64(after.Postgres.IdleInTransaction), 1, "", true),
		)
	}

	mounts := unionKeys(before.Filesystems, after.Filesystems)
	for _, mount := range mounts {
		b, bOK := before.Filesystems[mount]
		a, aOK := after.Filesystems[mount]
		if !bOK || !aOK {
			continue
		}
		deltas = append(deltas, compareNumeric("磁盘 "+mount, b, a, 1.0, "%", true))
	}

	names := unionKeys(before.Containers, after.Containers)
	sort.Strings(names)
	for _, name := range names {
		b, bOK := before.Containers[name]
		a, aOK := after.Containers[name]
		switch {
		case !bOK:
			deltas = append(deltas, Delta{Field: "容器 " + name, Before: "不存在", After: a.State, Change: "新增", Verdict: "变化", Material: true})
		case !aOK:
			deltas = append(deltas, Delta{Field: "容器 " + name, Before: b.State, After: "不存在", Change: "消失", Verdict: "变化", Material: true})
		default:
			restartDelta := a.Restarts - b.Restarts
			row := Delta{
				Field:  "容器 " + name + " 重启",
				Before: fmt.Sprintf("%d", b.Restarts),
				After:  fmt.Sprintf("%d", a.Restarts),
				Change: fmt.Sprintf("%+d", restartDelta),
			}
			switch {
			case restartDelta > 0:
				row.Verdict, row.Material = "恶化", true
			case b.State != a.State:
				row.Verdict, row.Material = "变化", true
				row.Change += fmt.Sprintf("，状态 %s→%s", b.State, a.State)
			default:
				row.Verdict = "持平"
			}
			deltas = append(deltas, row)
		}
	}

	return deltas
}

// FormatDeltas renders the comparison as a fixed-width table. The material rows
// come first so a reader sees the conclusion before the noise.
func FormatDeltas(before, after Fingerprint, deltas []Delta) string {
	var b strings.Builder

	fmt.Fprintf(&b, "对比：%s（%s）→ %s（%s）\n",
		before.Label, before.CapturedAt.Format("2006-01-02 15:04:05"),
		after.Label, after.CapturedAt.Format("2006-01-02 15:04:05"))

	material, minor := 0, 0
	for _, d := range deltas {
		if d.Material {
			material++
		} else {
			minor++
		}
	}
	fmt.Fprintf(&b, "共 %d 项，其中显著变化 %d 项\n\n", len(deltas), material)

	b.WriteString("指标\t变化前\t变化后\t变化量\t判定\n")
	for _, d := range deltas {
		mark := ""
		if d.Verdict == "恶化" {
			mark = " ←"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s%s\n", d.Field, d.Before, d.After, d.Change, d.Verdict, mark)
	}

	if material == 0 {
		b.WriteString("\n没有显著变化：所有指标都在噪声范围内，本次变更未产生可测量的效果。\n")
	}

	return b.String()
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func unionKeys[M ~map[string]V, V any](a, b M) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
