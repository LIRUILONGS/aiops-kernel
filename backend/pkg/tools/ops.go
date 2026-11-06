package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"opsagi/pkg/config"
	"opsagi/pkg/opsremote"
)

// OpsTargetArgs selects the host a read-only probe talks to. The target id is
// resolved from OPSAGI_TARGETS; an unknown id is answered with the list of
// configured ids rather than a connection error.
type OpsTargetArgs struct {
	Target string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机,example=lab"`
}

// OpsLogsArgs narrows a log query to one container or systemd unit.
type OpsLogsArgs struct {
	Target string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机"`
	Source string `json:"source,omitempty" jsonschema:"description=容器名或 systemd 单元名；留空读取系统错误日志,example=nginx"`
	Lines  int    `json:"lines,omitempty" jsonschema:"description=返回最近多少行，默认 100，上限 500,example=100"`
}

type opsTools struct {
	targets map[string]opsremote.Target
	order   []string
	timeout time.Duration
}

// NewOpsTools builds the read-only probe toolset from configuration. No targets
// configured means the tools stay unavailable instead of failing mid-flow.
func NewOpsTools(cfg *config.Config) *opsTools {
	tools := &opsTools{
		targets: make(map[string]opsremote.Target),
		timeout: 30 * time.Second,
	}
	if cfg == nil || strings.TrimSpace(cfg.OpsTargets) == "" {
		return tools
	}

	var parsed []opsremote.Target
	if err := json.Unmarshal([]byte(cfg.OpsTargets), &parsed); err != nil {
		return tools
	}
	for _, target := range parsed {
		if target.Host == "" {
			continue
		}
		if target.ID == "" {
			target.ID = target.Host
		}
		tools.targets[target.ID] = target
		tools.order = append(tools.order, target.ID)
	}
	sort.Strings(tools.order)

	return tools
}

func (o *opsTools) IsAvailable() bool { return len(o.targets) > 0 }

func (o *opsTools) resolve(id string) (opsremote.Target, error) {
	if len(o.targets) == 0 {
		return opsremote.Target{}, fmt.Errorf("未配置任何运维目标主机（OPSAGI_TARGETS 为空）")
	}
	if id == "" {
		return o.targets[o.order[0]], nil
	}
	target, ok := o.targets[id]
	if !ok {
		return opsremote.Target{}, fmt.Errorf("未知的目标主机 %q，可用目标：%s", id, strings.Join(o.order, ", "))
	}
	return target, nil
}

func (o *opsTools) dial(ctx context.Context, args json.RawMessage) (*opsremote.Client, opsremote.Target, error) {
	var parsed OpsTargetArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return nil, opsremote.Target{}, fmt.Errorf("解析参数失败：%w", err)
	}
	target, err := o.resolve(parsed.Target)
	if err != nil {
		return nil, target, err
	}

	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	client, err := opsremote.Dial(dialCtx, target)
	if err != nil {
		return nil, target, fmt.Errorf("连接目标主机 %s 失败：%w", target.ID, err)
	}
	return client, target, nil
}

func (o *opsTools) HandleSystem(ctx context.Context, name string, args json.RawMessage) (string, error) {
	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	snap, err := client.System(ctx)
	if err != nil {
		return "", fmt.Errorf("采集主机快照失败：%w", err)
	}
	return formatSystem(target, snap), nil
}

func (o *opsTools) HandleContainers(ctx context.Context, name string, args json.RawMessage) (string, error) {
	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	containers, err := client.Containers(ctx)
	if err != nil {
		return "", fmt.Errorf("采集容器状态失败：%w", err)
	}
	units, err := client.FailedUnits(ctx)
	if err != nil {
		units = nil
	}
	return formatContainers(target, containers, units), nil
}

func (o *opsTools) HandleLogs(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsLogsArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}

	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	lines := parsed.Lines
	if lines <= 0 {
		lines = 100
	}
	if lines > 500 {
		lines = 500
	}

	if parsed.Source == "" {
		res, err := client.JournalErrors(ctx, "-30 min", lines)
		if err != nil {
			return "", fmt.Errorf("读取系统错误日志失败：%w", err)
		}
		return formatLogs(target, "系统错误日志（最近 30 分钟）", res), nil
	}

	if !opsremote.ValidName(parsed.Source) {
		return "", fmt.Errorf("非法的日志来源 %q：只允许字母、数字、下划线、点和短横线", parsed.Source)
	}

	res, err := client.ContainerLogs(ctx, parsed.Source, lines)
	if err != nil {
		return "", fmt.Errorf("读取 %s 的日志失败：%w", parsed.Source, err)
	}
	return formatLogs(target, "容器 "+parsed.Source+" 日志", res), nil
}

func formatSystem(target opsremote.Target, snap opsremote.SystemSnapshot) string {
	var b strings.Builder

	fmt.Fprintf(&b, "目标主机：%s (%s:%d)\n", target.ID, target.Host, target.Port)
	fmt.Fprintf(&b, "主机名：%s\n", snap.Hostname)
	fmt.Fprintf(&b, "运行时长：%.1f 小时\n", snap.UptimeSec/3600)
	fmt.Fprintf(&b, "负载：%.2f / %.2f / %.2f（%d 核）\n", snap.Load1, snap.Load5, snap.Load15, snap.CPUCores)
	fmt.Fprintf(&b, "CPU 使用率：%.1f%%\n", snap.CPUUsage)
	fmt.Fprintf(&b, "内存：已用 %d MB / 共 %d MB（%.0f%%），Swap 已用 %d MB / 共 %d MB\n",
		snap.Memory.TotalMB-snap.Memory.AvailableMB, snap.Memory.TotalMB,
		snap.Memory.PercentUsed(), snap.Memory.SwapUsedMB, snap.Memory.SwapTotalMB)

	b.WriteString("\n磁盘：\n")
	for _, fs := range snap.Filesystems {
		fmt.Fprintf(&b, "  %s 已用 %d MB，可用 %d MB，使用率 %.0f%%\n",
			fs.MountPoint, fs.UsedKB/1024, fs.AvailKB/1024, fs.UsePercent)
	}

	b.WriteString("\nCPU 占用最高的进程：\n")
	for i, p := range snap.TopProcesses {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&b, "  pid=%d 命令=%s cpu=%.1f%% 内存=%.1f%% rss=%dMB\n",
			p.PID, p.Command, p.CPU, p.Memory, p.RSSKB/1024)
	}

	return b.String()
}

func formatContainers(target opsremote.Target, containers []opsremote.Container, failedUnits []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "目标主机：%s (%s:%d)\n", target.ID, target.Host, target.Port)
	if len(containers) == 0 {
		b.WriteString("没有检测到容器（可能未安装 Docker）\n")
	}
	for _, c := range containers {
		anomaly := containerAnomaly(c)
		fmt.Fprintf(&b, "  容器 %s：状态=%s 重启=%d 次 退出码=%d 镜像=%s%s\n",
			c.Name, c.State, c.RestartCount, c.ExitCode, c.Image, anomaly)
	}

	if len(failedUnits) > 0 {
		b.WriteString("\n失败的 systemd 单元：\n")
		for _, u := range failedUnits {
			fmt.Fprintf(&b, "  %s\n", u)
		}
	}

	return b.String()
}

// containerAnomaly turns the triple of state, restart count and exit code into
// the one signal the model should look at first.
func containerAnomaly(c opsremote.Container) string {
	switch {
	case c.RestartCount >= 3:
		return "  【异常：反复重启】"
	case c.State == "restarting":
		return "  【异常：正在重启】"
	case c.State == "exited" && c.ExitCode != 0:
		return fmt.Sprintf("  【异常：已退出，退出码 %d】", c.ExitCode)
	case c.State == "paused":
		return "  【异常：已暂停】"
	default:
		return ""
	}
}

func formatLogs(target opsremote.Target, label string, res opsremote.Result) string {
	var b strings.Builder

	fmt.Fprintf(&b, "目标主机：%s (%s:%d)\n", target.ID, target.Host, target.Port)
	fmt.Fprintf(&b, "%s（退出码 %d）\n\n", label, res.ExitCode)

	body := strings.TrimSpace(res.Stdout)
	if body == "" {
		if trimmed := strings.TrimSpace(res.Stderr); trimmed != "" {
			return b.String() + "（无标准输出）\n" + trimmed
		}
		return b.String() + "（无日志输出）"
	}

	const maxBody = 8000
	if len(body) > maxBody {
		body = "…（已截断，仅保留末尾内容）\n" + body[len(body)-maxBody:]
	}

	return b.String() + body
}
