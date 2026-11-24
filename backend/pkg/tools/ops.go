package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"opsagi/pkg/config"
	"opsagi/pkg/opsremote"

	"github.com/sirupsen/logrus"
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
	Lines  int    `json:"lines,omitempty" jsonschema:"description=返回最近多少行，默认 100，上限 500；单次结果约 8KB，超出时只保留末尾并在开头写明总行数,example=100"`
}

type opsTools struct {
	targets map[string]opsremote.Target
	order   []string
	timeout time.Duration

	samples baselineSamples

	sandboxPull    bool
	sandboxNetwork bool
	sandboxBPF     bool
	bpfImage       string
}

// baselineSamplesKept bounds what one flow keeps in memory. A flow takes a handful of
// samples around a change; past that the old ones are only useful for a comparison that
// already happened.
const baselineSamplesKept = 16

// baselineSamples holds the fingerprints ops_baseline returned so ops_diff can take two
// ids instead of two blobs. Retyping a fingerprint into a string argument is where the
// model that asked for the diff got it wrong: the id it can copy.
type baselineSamples struct {
	mx   sync.Mutex
	seq  int
	keep []string
	byID map[string]opsremote.Fingerprint
}

func (s *baselineSamples) put(fp opsremote.Fingerprint) string {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.byID == nil {
		s.byID = make(map[string]opsremote.Fingerprint)
	}

	s.seq++
	id := fmt.Sprintf("base-%d", s.seq)
	s.byID[id] = fp
	s.keep = append(s.keep, id)
	if len(s.keep) > baselineSamplesKept {
		delete(s.byID, s.keep[0])
		s.keep = s.keep[1:]
	}

	return id
}

func (s *baselineSamples) get(id string) (opsremote.Fingerprint, bool) {
	s.mx.Lock()
	defer s.mx.Unlock()

	fp, ok := s.byID[strings.TrimSpace(id)]

	return fp, ok
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
		// Startup validation rejects this, so reaching it means the value was set
		// after boot or the validator was bypassed. Say so: an empty toolset with
		// no explanation reads as "this deployment has no hosts".
		logrus.WithError(err).Error("OPSAGI_TARGETS cannot be parsed; every ops probe tool is disabled")

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
	tools.sandboxPull = cfg.OpsSandboxPull
	tools.sandboxNetwork = cfg.OpsSandboxNetwork
	tools.sandboxBPF = cfg.OpsSandboxBPF
	tools.bpfImage = cfg.OpsBPFImage

	return tools
}
func (o *opsTools) IsAvailable() bool { return len(o.targets) > 0 }

// TargetIDs lists the configured targets, sorted, so a prompt can name the ids
// a model may pass as `target` instead of leaving it to guess.
func (o *opsTools) TargetIDs() []string { return o.order }

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

// OpsContainersArgs lists containers, optionally narrowed by a substring. The
// filter exists so a count comes from the tool: a model that counts rows by eye
// gets the number wrong on a list of fifty.
type OpsContainersArgs struct {
	Target string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机,example=lab"`
	Filter string `json:"filter,omitempty" jsonschema:"description=只看名字里包含这段文字的容器（子串匹配，不是正则）,example=seal-engraver-"`
}

func (o *opsTools) HandleContainers(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsContainersArgs
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

	containers, err := client.Containers(ctx)
	if err != nil {
		return "", fmt.Errorf("采集容器状态失败：%w", err)
	}
	units, err := client.FailedUnits(ctx)
	if err != nil {
		units = nil
	}
	filter := strings.TrimSpace(parsed.Filter)
	return formatContainers(target, containers, filterContainers(containers, filter), units, filter), nil
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

// OpsExecArgs runs one entry of the read-only command catalogue. The agent picks
// the entry and its arguments; the command line itself comes from the catalogue.
type OpsExecArgs struct {
	Target    string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机,example=lab"`
	Command   string `json:"command" jsonschema:"description=只读命令 ID，取值见工具描述；不接受自由文本,example=listen_ports"`
	Unit      string `json:"unit,omitempty" jsonschema:"description=systemd 单元名，unit_status 与 unit_logs 必填,example=nginx.service"`
	Container string `json:"container,omitempty" jsonschema:"description=容器名，docker_inspect 必填,example=opslab-broken"`
	Path      string `json:"path,omitempty" jsonschema:"description=绝对路径，file_stat 必填,example=/etc/nginx/nginx.conf"`
	Image     string `json:"image,omitempty" jsonschema:"description=镜像引用，image_history 必填,example=nginx:1.27-alpine"`
	Database  string `json:"database,omitempty" jsonschema:"description=数据库名，pg_stat_activity、pg_stat_statements 与 pg_explain 必填,example=shop"`
	Lines     int    `json:"lines,omitempty" jsonschema:"description=行数，见各命令允许的范围,example=100"`
	Rank      int    `json:"rank,omitempty" jsonschema:"description=pg_explain 的语句序号，按累计耗时从高到低排第几，从 1 起,example=1"`
}

func (o *opsTools) HandleExec(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsExecArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}
	if parsed.Command == "" {
		return "", fmt.Errorf("缺少 command：必须是只读命令 ID 之一（%s）", strings.Join(opsremote.CommandIDs(), ", "))
	}

	execArgs := catalogueArgs(parsed)

	// Rendered before dialing: an unknown id or a value carrying a shell is a
	// mistake the model can fix, and a 20s connection timeout teaches it nothing.
	command, err := opsremote.Render(parsed.Command, execArgs)
	if err != nil {
		return "", err
	}

	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	res, err := client.RunCtx(ctx, command)
	if err != nil {
		return "", fmt.Errorf("执行 %s 失败：%w", parsed.Command, err)
	}

	label := parsed.Command
	if entry, ok := opsremote.Lookup(parsed.Command); ok {
		label = entry.Description
	}

	rendered := formatExec(target, label, res)
	if advice := opsremote.Advice(parsed.Command, res.Stdout); advice != "" {
		rendered += "\n" + advice
	}

	return rendered, nil
}

// OpsSandboxArgs runs a shell script inside the probe box on the target. The
// script is free text: the guarantee is the container it runs in, not a list of
// permitted commands.
type OpsSandboxArgs struct {
	Target  string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机,example=lab"`
	Script  string `json:"script" jsonschema:"description=在采集容器里执行的 shell 脚本。容器只读、无网络、无特权；宿主机的进程与网络要通过 /host/proc/<pid> 读，例如 /host/proc/1/net/tcp。脚本走 stdin，不经过目标机的 shell,example=cat /host/proc/pressure/io"`
	Image   string `json:"image,omitempty" jsonschema:"description=容器镜像，由你决定：要装工具就选一个有包管理器的（debian/ubuntu 用 apt、alpine 用 apk）。留空用 debian:latest；目标机上没有而操作员没放开拉取时，报错里会列出目标机上已有的镜像,example=alpine:latest"`
	Mode    string `json:"mode,omitempty" jsonschema:"description=容器形状：probe（默认，无网络且只读根文件系统，只能读）或 workshop（可写自己的容器层、允许出网，用来装你要的工具——宿主机仍然只读）。workshop 需要操作员放开出网,example=probe"`
	Timeout int    `json:"timeout_seconds,omitempty" jsonschema:"description=脚本超时秒数，默认 60，上限 300,example=60"`
}

func (o *opsTools) HandleSandbox(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsSandboxArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}
	if strings.TrimSpace(parsed.Script) == "" {
		return "", fmt.Errorf("缺少 script：要执行的只读脚本")
	}

	image, err := opsremote.ValidateSandboxImage(parsed.Image)
	if err != nil {
		return "", Refuse("%s", err.Error())
	}

	mode := opsremote.SandboxMode(parsed.Mode)
	// Refuse an unknown shape before dialling: the check used to happen when the
	// command line was built, which is after EnsureSandboxImage had already pulled
	// an image onto the host for a box that would never run.
	if err := opsremote.ValidateSandboxMode(mode); err != nil {
		return "", Refuse("%s", err.Error())
	}
	if mode == opsremote.SandboxModeWorkshop && !o.sandboxNetwork {
		return "", Refuse("作业形状需要出网，而操作员没有放开（OPSAGI_SANDBOX_NETWORK=false）：用 ask 申请安装，或继续用 probe 形状")
	}

	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	if err := client.EnsureSandboxImage(ctx, image, o.sandboxPull); err != nil {
		return "", Refuse("%s", err.Error())
	}

	timeout := time.Duration(parsed.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defaultSandboxTimeout
	}
	if timeout > maxSandboxTimeout {
		timeout = maxSandboxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := client.RunSandbox(ctx, image, parsed.Script, mode)
	if err != nil {
		return "", fmt.Errorf("采集容器执行失败：%w", err)
	}

	label := "采集容器（只读 / 无网络 / 无特权）"
	if mode == opsremote.SandboxModeWorkshop {
		label = "作业容器（可写容器层 / 可出网 / 宿主机只读无特权）"
	}

	return formatExec(target, label, res), nil
}

// defaultSandboxTimeout is what the tool description promises when the model
// gives no timeout: long enough for a sampling window, short enough that a
// forgotten loop does not hold the host.
const defaultSandboxTimeout = 60 * time.Second

// maxSandboxTimeout caps one probe run: a script that outlives it is a script
// that is watching, and watching is what ops_baseline and ops_diff are for.
const maxSandboxTimeout = 300 * time.Second

// OpsTraceArgs runs one catalogue eBPF program inside the trace shape. The
// program is a constant in opsremote: what the model chooses is which question
// to ask the kernel, never the code that runs with BPF capabilities.
type OpsTraceArgs struct {
	Target  string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机,example=lab"`
	Script  string `json:"script" jsonschema:"description=要跑的 eBPF 程序 ID（目录见工具说明）,example=runqlat"`
	Seconds int    `json:"seconds,omitempty" jsonschema:"description=采样窗口秒数，3 到 30，默认 10,example=10"`
	Image   string `json:"image,omitempty" jsonschema:"description=带 bpftrace 的镜像，留空用操作员配置的 OPSAGI_BPF_IMAGE,example=debian:bookworm"`
}

// traceIDs lists the catalogue for the description, which has to tell the model
// what it could ask for.
func traceIDs() []string {
	return strings.Split(opsremote.TraceIDs(), "、")
}

// TraceAvailable reports whether the operator turned kernel tracing on. The tool
// is absent until they do, rather than present and refused at call time.
func (o *opsTools) TraceAvailable() bool { return o.IsAvailable() && o.sandboxBPF }

func (o *opsTools) HandleTrace(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsTraceArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}

	script, ok := opsremote.LookupTrace(parsed.Script)
	if !ok {
		return "", Refuse("没有这个 eBPF 程序：%q；可选的是 %s", parsed.Script, opsremote.TraceIDs())
	}

	image := strings.TrimSpace(parsed.Image)
	if image == "" {
		image = o.bpfImage
	}
	image, err := opsremote.ValidateSandboxImage(image)
	if err != nil {
		return "", Refuse("%s", err.Error())
	}

	seconds := opsremote.ClampTraceSeconds(parsed.Seconds)

	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	if err := client.EnsureSandboxImage(ctx, image, o.sandboxPull); err != nil {
		return "", Refuse("%s", err.Error())
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Duration(seconds)*time.Second+traceOverhead)
	defer cancel()

	res, err := client.RunTrace(ctx, image, script, seconds, opsremote.TraceOptions{Network: o.sandboxNetwork})
	if err != nil {
		return "", fmt.Errorf("内核追踪执行失败：%w", err)
	}

	label := fmt.Sprintf("eBPF %s（%s，采样 %d 秒）", script.ID, script.Description, seconds)
	rendered := formatExec(target, label, res)
	if summary := opsremote.SummarizeTrace(res.Stdout); summary != "" {
		rendered += "\n" + summary
	}

	return rendered, nil
}

// traceOverhead is the room a container start and bpftrace's own startup need on
// top of the sampling window, twice over: a kernel that refuses the narrow shape
// costs one failed start before the retry opens the window.
const traceOverhead = 60 * time.Second

// OpsWatchArgs samples one catalogue command across a short window. The command
// is a catalogue id, not a shell line: what the model chooses is which evidence
// and how often, never what gets executed.
type OpsWatchArgs struct {
	Target    string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机,example=lab"`
	Command   string `json:"command" jsonschema:"description=要重复采样的只读命令 ID（与 ops_exec 同一套目录）,example=mysql_connections"`
	Container string `json:"container,omitempty" jsonschema:"description=命令需要的容器名,example=wp-db"`
	Database  string `json:"database,omitempty" jsonschema:"description=命令需要的库名（Postgres 命令用）,example=orders"`
	Unit      string `json:"unit,omitempty" jsonschema:"description=命令需要的 systemd 单元名,example=nginx.service"`
	Path      string `json:"path,omitempty" jsonschema:"description=命令需要的绝对路径,example=/etc/nginx/nginx.conf"`
	Image     string `json:"image,omitempty" jsonschema:"description=命令需要的镜像名,example=nginx:latest"`
	Lines     int    `json:"lines,omitempty" jsonschema:"description=命令需要输出的行数上限,example=20"`
	Samples   int    `json:"samples" jsonschema:"description=采样次数，2 到 30,example=10"`
	Interval  int    `json:"interval_seconds" jsonschema:"description=相邻两次采样的间隔秒数，1 到 30,example=3"`
	Timeout   int    `json:"timeout_seconds,omitempty" jsonschema:"description=整个采样窗口的超时秒数，默认按次数与间隔推算，上限 300,example=60"`
}

func (o *opsTools) HandleWatch(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsWatchArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}

	command, err := opsremote.RenderWatch(parsed.Command, catalogueArgsOf(parsed), parsed.Samples, parsed.Interval)
	if err != nil {
		return "", Refuse("%s", err.Error())
	}

	client, target, err := o.dial(ctx, args)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	timeout := time.Duration(parsed.Timeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(parsed.Samples*parsed.Interval)*time.Second + o.timeout
	}
	if timeout > maxSandboxTimeout {
		timeout = maxSandboxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := client.Run(ctx, command)
	if err != nil {
		return "", fmt.Errorf("采样执行失败：%w", err)
	}

	label := fmt.Sprintf("采样 %s（%d 次 / 间隔 %d 秒）", parsed.Command, parsed.Samples, parsed.Interval)

	return formatExec(target, label, res), nil
}

func catalogueArgsOf(args OpsWatchArgs) map[string]string {
	out := map[string]string{}
	for key, value := range map[string]string{
		"Container": args.Container,
		"Database":  args.Database,
		"Unit":      args.Unit,
		"Path":      args.Path,
		"Image":     args.Image,
	} {
		if value != "" {
			out[key] = value
		}
	}
	if args.Lines > 0 {
		out["Lines"] = fmt.Sprintf("%d", args.Lines)
	}

	return out
}

// OpsBaselineArgs captures a named sample of a host for later comparison.
type OpsBaselineArgs struct {
	Target    string `json:"target,omitempty" jsonschema:"description=目标主机 ID，留空使用默认主机"`
	Label     string `json:"label,omitempty" jsonschema:"description=本次采样的标签，例如 before-change / after-change,example=before-change"`
	Container string `json:"container,omitempty" jsonschema:"description=Postgres 所在容器名。与 database 一起给出时，指纹里会多一段数据库指标（锁等待会话数、最长等待秒数、长事务会话数）,example=opslab-db"`
	Database  string `json:"database,omitempty" jsonschema:"description=数据库名，与 container 成对出现,example=shop"`
	Statement string `json:"statement,omitempty" jsonschema:"description=语句片段。给出时（需要 container 与 database）把 pg_stat_statements 里包含这段文字、累计耗时最高的那条语句的调用次数与耗时一起记进指纹——性能场景的改动要靠它判断，主机负载只会因为建索引本身变差,example=where region = $1"`
}

// OpsDiffArgs compares two samples taken by ops_baseline. Each side is either the
// sample id ops_baseline returned or the fingerprint JSON it printed.
type OpsDiffArgs struct {
	Before string `json:"before" jsonschema_description:"ops_baseline 返回的采样编号（形如 base-3）或它的指纹 JSON"`
	After  string `json:"after" jsonschema_description:"ops_baseline 返回的采样编号（形如 base-5）或它的指纹 JSON"`
}

func (o *opsTools) HandleBaseline(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsBaselineArgs
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

	snap, err := client.System(ctx)
	if err != nil {
		return "", fmt.Errorf("采集基线失败：%w", err)
	}
	containers, err := client.Containers(ctx)
	if err != nil {
		containers = nil
	}

	label := parsed.Label
	if label == "" {
		label = "sample-" + time.Now().Format("150405")
	}

	fp := opsremote.SnapshotFingerprint(label, snap, containers)

	postgresNote := ""
	if parsed.Container != "" || parsed.Database != "" {
		brief, captureErr := o.capturePostgres(ctx, client, parsed.Container, parsed.Database)
		if captureErr != nil {
			postgresNote = "Postgres 段未采集：" + captureErr.Error() + "\n"
		} else {
			fp.Postgres = &brief
			postgresNote = fmt.Sprintf("Postgres 段（%s/%s）：等待锁的会话 %d 个，最长等待 %.3f 秒，idle in transaction %d 个。\n",
				brief.Container, brief.Database, brief.BlockedSessions, brief.LongestWaitS, brief.IdleInTransaction)

			if fragment := strings.TrimSpace(parsed.Statement); fragment != "" {
				query, queryErr := o.captureStatement(ctx, client, parsed.Container, parsed.Database, fragment)
				if queryErr != nil {
					postgresNote += "语句段未采集：" + queryErr.Error() + "\n"
				} else {
					fp.Postgres.Query = &query
					postgresNote += fmt.Sprintf("语句段（匹配 %q）：调用 %d 次，平均 %.2f ms，累计 %.1f ms，返回 %d 行。\n",
						fragment, query.Calls, query.MeanMS, query.TotalMS, query.Rows)
				}
			}
		}
	}

	payload, err := json.Marshal(fp)
	if err != nil {
		return "", fmt.Errorf("序列化基线失败：%w", err)
	}

	var b strings.Builder
	sampleID := o.samples.put(fp)
	fmt.Fprintf(&b, "已采集基线 %q（采样编号 %s，目标 %s，主机 %s）\n", label, sampleID, target.ID, fp.Host)
	fmt.Fprintf(&b, "负载 %.2f，CPU %.1f%%，内存 %d/%d MB，磁盘根分区 %.0f%%\n",
		fp.Load1, fp.CPUPercent, fp.MemUsedMB, fp.MemTotalMB, fp.Filesystems["/"])
	if postgresNote != "" {
		b.WriteString(postgresNote)
	}
	b.WriteString("\n做完变更后再调一次 ops_baseline，然后用 ops_diff 把第一次的编号传给 before、第二次的传给 after（编号比指纹 JSON 可靠，别手抄 JSON）：\n")
	if fp.Postgres != nil && fp.Postgres.Query != nil {
		b.WriteString("两次采样都带同一个 statement 时，ops_diff 会多出「语句平均耗时」与「语句累计耗时」两行——性能改动要看这两行，主机负载会因为建索引本身变差。\n")
	}
	b.WriteString("指纹原文（需要时再读，不必整段抄进工具参数）：\n")
	b.WriteString(string(payload))
	b.WriteString("\n")

	return b.String(), nil
}

// capturePostgres adds the database half of a fingerprint through the same
// catalogue command the agent may call, so the SQL and its validation live in one
// place. A failure is reported, not fatal: the host half is still worth keeping.
func (o *opsTools) capturePostgres(ctx context.Context, client *opsremote.Client, container, database string) (opsremote.PostgresBrief, error) {
	if container == "" || database == "" {
		return opsremote.PostgresBrief{}, fmt.Errorf("采集 Postgres 段需要同时给出 container 与 database")
	}

	command, err := opsremote.Render("pg_lock_wait_summary", map[string]string{
		"Container": container,
		"Database":  database,
	})
	if err != nil {
		return opsremote.PostgresBrief{}, err
	}

	res, err := client.RunCtx(ctx, command)
	if err != nil {
		return opsremote.PostgresBrief{}, fmt.Errorf("执行锁等待摘要失败：%w", err)
	}

	return opsremote.ParsePostgresBrief(container, database, res.Stdout)
}

// captureStatement takes one statement's timing into the same fingerprint. The
// catalogue command lists the heaviest statements, so the fragment picks the
// most expensive one that carries it — the statement a change was made for.
func (o *opsTools) captureStatement(ctx context.Context, client *opsremote.Client, container, database, fragment string) (opsremote.QueryBrief, error) {
	command, err := opsremote.Render("pg_stat_statements", map[string]string{
		"Container": container,
		"Database":  database,
		"Lines":     "50",
	})
	if err != nil {
		return opsremote.QueryBrief{}, err
	}

	res, err := client.RunCtx(ctx, command)
	if err != nil {
		return opsremote.QueryBrief{}, fmt.Errorf("执行语句列表失败：%w", err)
	}

	return opsremote.ParseQueryBrief(fragment, res.Stdout)
}


func (o *opsTools) HandleDiff(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsed OpsDiffArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}
	if strings.TrimSpace(parsed.Before) == "" || strings.TrimSpace(parsed.After) == "" {
		return "", fmt.Errorf("before 与 after 都是必填：请分别填入两次 ops_baseline 返回的采样编号（形如 base-3）")
	}

	before, err := o.fingerprintOf("before", parsed.Before)
	if err != nil {
		return "", err
	}
	after, err := o.fingerprintOf("after", parsed.After)
	if err != nil {
		return "", err
	}
	if before.Host == "" || after.Host == "" {
		return "", fmt.Errorf("指纹里缺少主机信息，请确认传入的是 ops_baseline 的采样编号或输出")
	}
	if before.Host != after.Host {
		return "", fmt.Errorf("两次采样来自不同主机（%s 与 %s），无法对比", before.Host, after.Host)
	}

	return opsremote.FormatDeltas(before, after, opsremote.CompareFingerprints(before, after)), nil
}

// fingerprintOf resolves one side of a diff: the sample id ops_baseline returned, or the
// fingerprint JSON itself for a caller that pasted the whole result.
func (o *opsTools) fingerprintOf(side, value string) (opsremote.Fingerprint, error) {
	if fp, ok := o.samples.get(value); ok {
		return fp, nil
	}

	var fp opsremote.Fingerprint
	if err := json.Unmarshal([]byte(extractJSON(value)), &fp); err != nil {
		return opsremote.Fingerprint{}, fmt.Errorf("解析 %s 失败（%q）：它既不是指纹 JSON，也不是这条任务采集过的采样编号——编号只在采集它的那条 flow 里有效（本 flow 还留着最近 %d 次），请先用 ops_baseline 采一次再比对",
			side, clip(value, 60), baselineSamplesKept)
	}

	return fp, nil
}

// clip shortens a value for an error message: a pasted fingerprint would otherwise fill
// the refusal with the thing the model already has.
func clip(value string, limit int) string {
	if len(value) <= limit {
		return value
	}

	return value[:limit] + "…"
}

// extractJSON tolerates a fingerprint pasted together with the sentence around
// it: models copy the whole tool result rather than the bare object.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
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

// filterContainers narrows a listing to the names carrying the substring. An
// empty filter returns every container.
func filterContainers(containers []opsremote.Container, filter string) []opsremote.Container {
	if filter == "" {
		return containers
	}

	shown := make([]opsremote.Container, 0, len(containers))
	for _, c := range containers {
		if strings.Contains(c.Name, filter) {
			shown = append(shown, c)
		}
	}

	return shown
}

func formatContainers(target opsremote.Target, all, shown []opsremote.Container, failedUnits []string, filter string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "目标主机：%s (%s:%d)\n", target.ID, target.Host, target.Port)

	switch {
	case len(all) == 0:
		b.WriteString("没有检测到容器（可能未安装 Docker）\n")
	case filter != "" && len(shown) == 0:
		fmt.Fprintf(&b, "名字里包含 %q 的容器 0 个（这台主机上共 %d 个）；换一个子串，或者不带 filter 看全部。\n", filter, len(all))
	case filter != "":
		fmt.Fprintf(&b, "名字里包含 %q 的容器 %d 个（这台主机上共 %d 个）\n", filter, len(shown), len(all))
	default:
		fmt.Fprintf(&b, "容器 %d 个\n", len(all))
	}

	for _, c := range shown {
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
		body = cutTail(body, maxBody)
	}

	return b.String() + body
}

// cutTail answers "how much of it did I get": an agent that asked for 500 lines and
// silently receives 60 concludes the rest of the log is healthy.
func cutTail(body string, maxBody int) string {
	if len(body) <= maxBody {
		return body
	}

	tail := body[len(body)-maxBody:]
	if idx := strings.IndexByte(tail, '\n'); idx >= 0 && idx+1 < len(tail) {
		tail = tail[idx+1:]
	}

	return fmt.Sprintf("…（日志过长：共 %d 行，这里只保留最后 %d 行；要更早的内容就缩小 range 或换个来源再取一次）\n%s",
		strings.Count(body, "\n")+1, strings.Count(tail, "\n")+1, tail)
}

// formatExec prints what was run before what came back, so a conclusion can name
// the command that produced its evidence instead of describing it from memory.
func formatExec(target opsremote.Target, label string, res opsremote.Result) string {
	var b strings.Builder

	fmt.Fprintf(&b, "目标主机：%s (%s:%d)\n", target.ID, target.Host, target.Port)
	fmt.Fprintf(&b, "命令：%s（退出码 %d，耗时 %s）\n", label, res.ExitCode, res.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "原始命令：%s\n\n", res.Command)

	body := strings.TrimSpace(res.Stdout)
	if body == "" {
		if trimmed := strings.TrimSpace(res.Stderr); trimmed != "" {
			return b.String() + "（无标准输出，以下是标准错误）\n" + trimmed
		}
		return b.String() + "（没有任何输出，说明该命令在这台主机上没有可读的数据）"
	}

	const maxBody = 8000
	if len(body) > maxBody {
		body = body[:maxBody] + "\n…（已截断）"
	}

	return b.String() + body
}

// catalogueArgs maps the tool's fields onto the placeholders the catalogue
// declares. A field that is not named here is a parameter the model can supply
// and the renderer never sees, which reads to the model as "missing argument".
func catalogueArgs(args OpsExecArgs) map[string]string {
	out := map[string]string{
		"Unit":      args.Unit,
		"Container": args.Container,
		"Path":      args.Path,
		"Image":     args.Image,
		"Database":  args.Database,
	}
	if args.Lines > 0 {
		out["Lines"] = fmt.Sprintf("%d", args.Lines)
	}
	if args.Rank > 0 {
		out["Rank"] = fmt.Sprintf("%d", args.Rank)
	}
	return out
}
