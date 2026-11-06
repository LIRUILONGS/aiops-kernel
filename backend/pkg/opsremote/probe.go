package opsremote

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// systemCommand collects the host snapshot in one round trip. The one-second
// sleep exists to sample /proc/stat twice: a single sample cannot yield a
// utilisation percentage.
const systemCommand = `echo "###opsagi:host"; hostname 2>/dev/null
echo "###opsagi:uptime"; cat /proc/uptime 2>/dev/null
echo "###opsagi:load"; cat /proc/loadavg 2>/dev/null
echo "###opsagi:cores"; nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo
echo "###opsagi:cpustat"; head -1 /proc/stat; sleep 1; head -1 /proc/stat
echo "###opsagi:mem"; free -m 2>/dev/null
echo "###opsagi:df"; df -P -x tmpfs -x devtmpfs -x overlay 2>/dev/null || df -P
echo "###opsagi:ps"; ps -eo pid,comm,pcpu,pmem,rss --sort=-pcpu --no-headers 2>/dev/null | head -15`

// containersCommand prints one line per container. Restart count is the field
// that turns "a container is up" into "a container is crash-looping".
const containersCommand = `if command -v docker >/dev/null 2>&1; then docker ps -aq 2>/dev/null | xargs -r docker inspect --format '{{.Name}}|{{.RestartCount}}|{{.State.Status}}|{{.State.ExitCode}}|{{.Config.Image}}' 2>/dev/null; fi`

const failedUnitsCommand = `systemctl --failed --no-legend --plain 2>/dev/null | head -20`

// safeName guards the one place a caller-supplied string reaches a shell.
var safeName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// ValidName reports whether a container or unit name may be passed to a shell
// command. Anything with whitespace, quotes, or shell metacharacters is refused.
func ValidName(s string) bool { return safeName.MatchString(s) }

// System collects the host snapshot.
func (c *Client) System(ctx context.Context) (SystemSnapshot, error) {
	var snap SystemSnapshot
	res, err := c.RunCtx(ctx, systemCommand)
	if err != nil {
		return snap, err
	}
	if res.ExitCode != 0 && res.Stdout == "" {
		return snap, fmt.Errorf("system probe failed with exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}

	sections := SplitSections(res.Stdout)
	snap.CollectedAt = time.Now().UTC()
	snap.Hostname = strings.TrimSpace(sections["host"])

	if v, err := ParseUptime(sections["uptime"]); err == nil {
		snap.UptimeSec = v
	}
	if l1, l5, l15, err := ParseLoadAvg(sections["load"]); err == nil {
		snap.Load1, snap.Load5, snap.Load15 = l1, l5, l15
	}
	if v, err := strconv.Atoi(strings.TrimSpace(sections["cores"])); err == nil {
		snap.CPUCores = v
	}
	if v, err := ParseCPUStat(sections["cpustat"]); err == nil {
		snap.CPUUsage = v
	}
	if mem, err := ParseMemInfo(sections["mem"]); err == nil {
		snap.Memory = mem
	}
	snap.Filesystems = ParseDF(sections["df"])
	snap.TopProcesses = ParsePS(sections["ps"])

	return snap, nil
}

// Containers lists Docker containers, including stopped ones.
func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	res, err := c.RunCtx(ctx, containersCommand)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 && strings.TrimSpace(res.Stdout) == "" {
		return nil, fmt.Errorf("container probe failed with exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return ParseContainers(res.Stdout), nil
}

// FailedUnits lists systemd units in a failed state. A host without systemd
// (or without permission) returns an empty list rather than an error.
func (c *Client) FailedUnits(ctx context.Context) ([]string, error) {
	res, err := c.RunCtx(ctx, failedUnitsCommand)
	if err != nil {
		return nil, err
	}
	units := make([]string, 0, 8)
	for _, line := range strings.Split(res.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		units = append(units, fields[0])
	}
	return units, nil
}

// ContainerLogs returns the tail of one container's logs. The name is validated
// against safeName before it reaches the shell.
func (c *Client) ContainerLogs(ctx context.Context, name string, lines int) (Result, error) {
	if !ValidName(name) {
		return Result{}, fmt.Errorf("invalid container name %q", name)
	}
	if lines <= 0 || lines > 2000 {
		lines = 300
	}
	cmd := fmt.Sprintf(`docker logs --timestamps --tail %d %s 2>&1`, lines, name)
	return c.RunCtx(ctx, cmd)
}

// JournalErrors returns recent error-priority journal entries. "since" accepts
// journalctl syntax such as "-30 min"; empty means the last 200 entries.
func (c *Client) JournalErrors(ctx context.Context, since string, lines int) (Result, error) {
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	sinceArg := ""
	if since != "" {
		if !regexp.MustCompile(`^[-0-9a-zA-Z: ]+$`).MatchString(since) {
			return Result{}, fmt.Errorf("invalid since value %q", since)
		}
		sinceArg = fmt.Sprintf(`--since "%s"`, since)
	}
	cmd := fmt.Sprintf(`journalctl -p err %s -n %d --no-pager -o short-iso 2>/dev/null`, sinceArg, lines)
	return c.RunCtx(ctx, cmd)
}

// Report is the full read-only evidence set for one host. Collection errors are
// recorded per section: a host without Docker still produces a usable report.
type Report struct {
	Target      Target         `json:"target"`
	System      SystemSnapshot `json:"system"`
	Containers  []Container    `json:"containers"`
	FailedUnits []string       `json:"failed_units"`
	Errors      []string       `json:"errors,omitempty"`
	CollectedAt time.Time      `json:"collected_at"`
}

// Collect gathers the full read-only snapshot. Individual probe failures are
// reported in Report.Errors instead of aborting the whole collection.
func (c *Client) Collect(ctx context.Context) Report {
	report := Report{Target: c.target, CollectedAt: time.Now().UTC()}

	if snap, err := c.System(ctx); err != nil {
		report.Errors = append(report.Errors, "system: "+err.Error())
	} else {
		report.System = snap
	}
	if containers, err := c.Containers(ctx); err != nil {
		report.Errors = append(report.Errors, "containers: "+err.Error())
	} else {
		report.Containers = containers
	}
	if units, err := c.FailedUnits(ctx); err != nil {
		report.Errors = append(report.Errors, "failed_units: "+err.Error())
	} else {
		report.FailedUnits = units
	}

	return report
}
