package opsremote

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MemoryInfo is `free -m` in megabytes.
type MemoryInfo struct {
	TotalMB     int `json:"total_mb"`
	UsedMB      int `json:"used_mb"`
	FreeMB      int `json:"free_mb"`
	AvailableMB int `json:"available_mb"`
	SwapTotalMB int `json:"swap_total_mb"`
	SwapUsedMB  int `json:"swap_used_mb"`
}

// PercentUsed is used memory over total, ignoring cache.
func (m MemoryInfo) PercentUsed() float64 {
	if m.TotalMB == 0 {
		return 0
	}
	return float64(m.TotalMB-m.AvailableMB) / float64(m.TotalMB) * 100
}

// Filesystem is one line of `df -P`.
type Filesystem struct {
	Device     string  `json:"device"`
	MountPoint string  `json:"mount"`
	SizeKB     int64   `json:"size_kb"`
	UsedKB     int64   `json:"used_kb"`
	AvailKB    int64   `json:"avail_kb"`
	UsePercent float64 `json:"use_percent"`
}

// Process is one row of `ps`.
type Process struct {
	PID     int     `json:"pid"`
	Command string  `json:"command"`
	CPU     float64 `json:"cpu_percent"`
	Memory  float64 `json:"mem_percent"`
	RSSKB   int64   `json:"rss_kb"`
}

// Container is one Docker container as seen from the host.
type Container struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Status       string `json:"status"`
	State        string `json:"state"`
	RestartCount int    `json:"restart_count"`
	ExitCode     int    `json:"exit_code"`
}

// SystemSnapshot is everything the system probe returns in one round trip.
type SystemSnapshot struct {
	Hostname     string       `json:"hostname"`
	UptimeSec    float64      `json:"uptime_sec"`
	Load1        float64      `json:"load1"`
	Load5        float64      `json:"load5"`
	Load15       float64      `json:"load15"`
	CPUCores     int          `json:"cpu_cores"`
	CPUUsage     float64      `json:"cpu_usage_percent"`
	Memory       MemoryInfo   `json:"memory"`
	Filesystems  []Filesystem `json:"filesystems"`
	TopProcesses []Process    `json:"top_processes"`
	CollectedAt  time.Time    `json:"collected_at"`
}

// Section separator used by the composite system command. A marker that cannot
// appear in normal command output keeps the parser honest.
const sectionPrefix = "###opsagi:"

// SplitSections turns marker-delimited output into a map. Unknown markers are
// kept; a missing section is simply absent, which callers treat as "the host
// does not answer that question".
func SplitSections(output string) map[string]string {
	sections := map[string]string{}
	current := ""
	var buf strings.Builder

	flush := func() {
		if current != "" {
			sections[current] = strings.TrimRight(buf.String(), "\n")
		}
		buf.Reset()
	}

	for _, line := range strings.Split(output, "\n") {
		if name, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), sectionPrefix); ok {
			flush()
			current = name
			continue
		}
		if current != "" {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	flush()
	return sections
}

// ParseLoadAvg reads "0.84 0.29 0.10 1/121 112489" from /proc/loadavg.
func ParseLoadAvg(s string) (load1, load5, load15 float64, err error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("loadavg: expected 3 fields, got %d", len(fields))
	}
	vals := make([]float64, 3)
	for i := 0; i < 3; i++ {
		v, convErr := strconv.ParseFloat(fields[i], 64)
		if convErr != nil {
			return 0, 0, 0, fmt.Errorf("loadavg field %d: %w", i, convErr)
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], nil
}

// ParseUptime reads "123456.78 987654.32" from /proc/uptime.
func ParseUptime(s string) (float64, error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return 0, fmt.Errorf("uptime: empty")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// ParseMemInfo reads `free -m` output, locating the Mem: and Swap: rows by
// label rather than by position.
func ParseMemInfo(s string) (MemoryInfo, error) {
	var info MemoryInfo
	found := false
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "Mem":
			info.TotalMB = atoi(fields[1])
			info.UsedMB = atoi(fields[2])
			info.FreeMB = atoi(fields[3])
			if len(fields) >= 7 {
				info.AvailableMB = atoi(fields[6])
			}
			found = true
		case "Swap":
			info.SwapTotalMB = atoi(fields[1])
			info.SwapUsedMB = atoi(fields[2])
		}
	}
	if !found {
		return info, fmt.Errorf("meminfo: no Mem: row")
	}
	return info, nil
}

// ParseDF reads `df -P` output. The header row and wrapped continuation lines
// are skipped.
func ParseDF(s string) []Filesystem {
	filesystems := make([]Filesystem, 0, 8)
	for i, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 6 || fields[0] == "Filesystem" {
			continue
		}
		mount := fields[len(fields)-1]
		fs := Filesystem{
			Device:     fields[0],
			SizeKB:     atoi64(fields[1]),
			UsedKB:     atoi64(fields[2]),
			AvailKB:    atoi64(fields[3]),
			UsePercent: parsePercent(fields[4]),
			MountPoint: mount,
		}
		filesystems = append(filesystems, fs)
	}
	return filesystems
}

// ParsePS reads `ps -eo pid,comm,pcpu,pmem,rss --no-headers` output.
func ParsePS(s string) []Process {
	processes := make([]Process, 0, 16)
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		cpu, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			continue
		}
		mem, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			continue
		}
		processes = append(processes, Process{
			PID:     atoi(fields[0]),
			Command: strings.Join(fields[1:len(fields)-3], " "),
			CPU:     cpu,
			Memory:  mem,
			RSSKB:   atoi64(fields[len(fields)-1]),
		})
	}
	return processes
}

// ParseCPUStat computes busy percentage from two /proc/stat first lines.
// The lines are expected newest-last, separated by a newline.
func ParseCPUStat(s string) (float64, error) {
	lines := make([]string, 0, 2)
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "cpu ") {
			lines = append(lines, line)
		}
	}
	if len(lines) < 2 {
		return 0, fmt.Errorf("cpustat: need two samples, got %d", len(lines))
	}
	prev := parseCPULine(lines[0])
	curr := parseCPULine(lines[1])
	totalDelta := curr.total - prev.total
	if totalDelta <= 0 {
		return 0, nil
	}
	idleDelta := curr.idle - prev.idle
	busy := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	return busy, nil
}

type cpuSample struct {
	total uint64
	idle  uint64
}

func parseCPULine(line string) cpuSample {
	fields := strings.Fields(line)[1:]
	var s cpuSample
	for i, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		s.total += v
		// idle and iowait are both "not busy" for our purposes.
		if i == 3 || i == 4 {
			s.idle += v
		}
	}
	return s
}

// ParseContainers reads the inspects format used by CollectContainers:
// "name|restarts|state|exit|image".
func ParseContainers(s string) []Container {
	containers := make([]Container, 0, 16)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		containers = append(containers, Container{
			Name:         strings.TrimPrefix(parts[0], "/"),
			RestartCount: atoi(parts[1]),
			State:        parts[2],
			ExitCode:     atoi(parts[3]),
			Status:       parts[2],
			Image:        parts[4],
		})
	}
	return containers
}

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func atoi64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func parsePercent(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	return v
}
