package opsremote

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// traceBaseShape is the container the tracer runs in. /proc and /sys stay
// read-only, as in every other shape, and the host pid namespace is still not
// joined: what the BPF programs read is kernel state, which needs no host mount
// to observe.
const traceBaseShape = "docker run --rm -i" +
	" --cap-drop=ALL --cap-add=BPF --cap-add=PERFMON --cap-add=SYS_RESOURCE" +
	" --security-opt=no-new-privileges --pids-limit=256 --cpus=1 --memory=256m" +
	" --network=none --read-only" +
	" -v /sys/kernel/debug:/sys/kernel/debug:ro -v /sys/kernel/tracing:/sys/kernel/tracing:ro" +
	" -v /sys/kernel/btf:/sys/kernel/btf:ro"

// traceSysAdminFlag is the fallback, not the default: kernels that do not wire
// CAP_PERFMON into the perf_event_open check want SYS_ADMIN, and it is the one
// capability that could remount those read-only mounts. A run starts without it
// and earns it only by being refused, so a host that does not need it never
// hands it out. Nothing the model writes reaches this shape either way: it
// picks a catalogue id, and the program text is a constant beside it.
const traceSysAdminFlag = " --cap-add=SYS_ADMIN"

// traceNetworkFlag replaces the closed network rather than adding a second
// --network, which docker refuses. Egress is only for an image that has to
// fetch bpftrace, and only when the operator allowed it.
const traceNetworkFlag = "--network=bridge"

// TraceDefaultSeconds bounds one trace when the caller gives no duration: long
// enough for a histogram to fill, short enough to stay inside a diagnosis.
const TraceDefaultSeconds = 10

// TraceMaxSeconds caps one run. A trace is a sample, not a monitor.
const TraceMaxSeconds = 30

// TraceScript is one instrument an agent may run by name. The bpftrace program
// is a constant here: an agent chooses which question to ask, never the text
// that runs with kernel privileges.
type TraceScript struct {
	ID          string
	Description string
	Program     string
	Scratch     string
}

// traceCatalogue is ordered because the tool description lists it in this order.
var traceCatalogue = []TraceScript{
	{
		ID:          "runqlat",
		Scratch:     "@start",
		Description: "调度延迟直方图：每个进程从可运行到真正拿到 CPU 等了多久，用来区分「CPU 真的忙」和「进程在排队」",
		Program: `tracepoint:sched:sched_wakeup { @start[args->pid] = nsecs; }
tracepoint:sched:sched_switch /@start[args->prev_pid]/ { @us[args->prev_comm] = hist((nsecs - @start[args->prev_pid]) / 1000); delete(@start[args->prev_pid]); }`,
	},
	{
		ID:          "offcputime",
		Scratch:     "@start",
		Description: "off-CPU 时间：进程没在跑的时间按进程累计，用来找阻塞在 IO、锁或 futex 上的东西",
		Program: `tracepoint:sched:sched_switch { @start[args->prev_pid] = nsecs; }
tracepoint:sched:sched_switch /@start[args->next_pid]/ { @us[args->next_comm] = sum((nsecs - @start[args->next_pid]) / 1000); delete(@start[args->next_pid]); }`,
	},
	{
		ID:          "biolatency",
		Scratch:     "@start",
		Description: "块设备 IO 延迟直方图：按设备看一次读写从下达到完成花了多久，用来判断慢在磁盘还是慢在上层",
		Program: `tracepoint:block:block_rq_issue { @start[args->dev, args->sector] = nsecs; }
tracepoint:block:block_rq_complete /@start[args->dev, args->sector]/ { @us[args->dev] = hist((nsecs - @start[args->dev, args->sector]) / 1000); delete(@start[args->dev, args->sector]); }`,
	},
	{
		ID:          "execsnoop",
		Description: "进程创建：窗口内谁起了什么进程，用来抓重启脚本、被反复拉起的短命进程",
		Program: `tracepoint:syscalls:sys_enter_execve { @[comm, str(args->filename)] = count(); }`,
	},
	{
		ID:          "tcpconnlat",
		Scratch:     "@start",
		Description: "TCP 建连延迟直方图：连出去花了多久，用来区分对端慢、DNS 慢和本机网络栈的问题",
		Program: `kprobe:tcp_v4_connect { @start[tid] = nsecs; }
kretprobe:tcp_v4_connect /@start[tid]/ { @us[comm] = hist((nsecs - @start[tid]) / 1000); delete(@start[tid]); }`,
	},
}

// TraceScripts returns the catalogue in listing order.
func TraceScripts() []TraceScript { return append([]TraceScript(nil), traceCatalogue...) }

// LookupTrace resolves a script id.
func LookupTrace(id string) (TraceScript, bool) {
	for _, script := range traceCatalogue {
		if script.ID == id {
			return script, true
		}
	}

	return TraceScript{}, false
}

// TraceIDs lists the ids for an error message that has to tell the caller what
// it could have asked for.
func TraceIDs() string {
	ids := make([]string, 0, len(traceCatalogue))
	for _, script := range traceCatalogue {
		ids = append(ids, script.ID)
	}

	return strings.Join(ids, "、")
}

// ClampTraceSeconds bounds the window; zero means the default.
func ClampTraceSeconds(seconds int) int {
	switch {
	case seconds <= 0:
		return TraceDefaultSeconds
	case seconds > TraceMaxSeconds:
		return TraceMaxSeconds
	}

	return seconds
}

// TraceOptions carries the two decisions the operator makes for a run: whether
// the box may reach out, and whether it starts with the wider capability.
type TraceOptions struct {
	Network  bool
	Escalate bool
}

// TraceCommand is the remote command line for one trace: fixed flags, a
// validated image, a catalogue program and a duration. Nothing the agent wrote
// reaches the shell that starts docker -- the script travels on stdin.
func TraceCommand(image string, script TraceScript, seconds int, opts TraceOptions) (string, error) {
	image = strings.TrimSpace(image)
	if !sandboxImagePattern.MatchString(image) {
		return "", fmt.Errorf("镜像名不合法：%q", image)
	}
	if seconds < 3 || seconds > TraceMaxSeconds {
		return "", fmt.Errorf("采样窗口必须在 3 到 %d 秒之间", TraceMaxSeconds)
	}

	flags := traceBaseShape
	if opts.Network {
		flags = strings.Replace(flags, "--network=none", traceNetworkFlag, 1)
	}
	if opts.Escalate {
		flags = strings.Replace(flags, " --security-opt=", traceSysAdminFlag+" --security-opt=", 1)
	}

	return flags + " " + image + " " + sandboxScriptPath, nil
}

// TraceGate reports the one failure a wider capability actually fixes: the
// kernel refusing perf_event_open to a container that holds BPF and PERFMON but
// not SYS_ADMIN. Every other error is left alone, because escalating on a bad
// image or a bad program would only buy the same failure with more privilege.
func TraceGate(res Result) bool {
	out := res.Stderr + res.Stdout

	return strings.Contains(out, "perf_event_open") && strings.Contains(out, "Permission denied")
}

// RunTrace runs one catalogue program, and retries once with SYS_ADMIN when the
// kernel asked for it. The first attempt is the narrow one on every host, so a
// kernel that honours CAP_PERFMON never sees the wider shape; the retry costs
// one failed start, which happens before the sampling window opens.
func (c *Client) RunTrace(ctx context.Context, image string, script TraceScript, seconds int, opts TraceOptions) (Result, error) {
	res, err := c.runTrace(ctx, image, script, seconds, opts)
	if opts.Escalate || ctx.Err() != nil || !TraceGate(res) {
		return res, err
	}

	opts.Escalate = true

	return c.runTrace(ctx, image, script, seconds, opts)
}

// runTrace is one attempt. A run that outlives its deadline is killed by name,
// because a trace that keeps running keeps costing the host it is watching.
func (c *Client) runTrace(ctx context.Context, image string, script TraceScript, seconds int, opts TraceOptions) (Result, error) {
	command, err := TraceCommand(image, script, seconds, opts)
	if err != nil {
		return Result{}, err
	}

	name := "opsagi-trace-" + sandboxSuffix()
	if name != "opsagi-trace-" {
		command = strings.Replace(command, "docker run --rm -i", "docker run --rm -i --name "+name, 1)
	}

	run := c.runStdin
	if run == nil {
		run = c.RunStdin
	}

	res, err := run(ctx, command, TraceProgram(script, seconds))
	if err != nil && ctx.Err() != nil && name != "opsagi-trace-" {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_, _ = c.Run(cleanup, "docker kill "+name+" >/dev/null 2>&1; docker rm -f "+name+" >/dev/null 2>&1")
	}

	return res, err
}

// TraceProgram is what the tracer reads on stdin. bpftrace has to exist in the
// image: the shape has no network unless the operator allowed egress, and even
// then installing on every run would double the cost of a ten-second sample.
func TraceProgram(script TraceScript, seconds int) string {
	scratch := ""
	if script.Scratch != "" {
		scratch = "clear(" + script.Scratch + "); "
	}

	return fmt.Sprintf(`if ! command -v bpftrace >/dev/null 2>&1; then
  echo "bpftrace 不在镜像里。换一个带 bpftrace 的镜像（OPSAGI_BPF_IMAGE），或者让操作员放开出网后由本脚本安装。" >&2
  exit 127
fi
bpftrace -q -e '%s
interval:s:%d { %sexit(); }'
`, script.Program, seconds, scratch)
}
