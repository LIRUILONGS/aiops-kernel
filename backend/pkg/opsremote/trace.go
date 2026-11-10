package opsremote

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TraceShape is the container the tracer runs in. /proc and /sys stay read-only,
// as in every other shape, and the host pid namespace is still not joined: what
// the BPF programs read is kernel state, which needs no host mount to observe.
// The three capabilities are the whole of what eBPF needs and nothing more --
// SYS_ADMIN, SYS_PTRACE and SYS_KILL are absent, so a trace cannot mount, cannot
// ptrace and cannot signal anything on the host.
const TraceShape = "docker run --rm -i" +
	" --cap-drop=ALL --cap-add=BPF --cap-add=PERFMON --cap-add=SYS_RESOURCE" +
	" --security-opt=no-new-privileges --pids-limit=256 --cpus=1 --memory=256m" +
	" --network=none --read-only" +
	" -v /sys/kernel/debug:/sys/kernel/debug:ro -v /sys/kernel/tracing:/sys/kernel/tracing:ro" +
	" -v /sys/kernel/btf:/sys/kernel/btf:ro"

// traceFlagsWithNetwork is the shape with egress, used only when the image has
// to fetch bpftrace or the trace itself needs it. It replaces the closed network
// rather than adding a second --network, which docker refuses.
const traceFlagsWithNetwork = "--network=bridge"

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
}

// traceCatalogue is ordered because the tool description lists it in this order.
var traceCatalogue = []TraceScript{
	{
		ID:          "runqlat",
		Description: "调度延迟直方图：每个进程从可运行到真正拿到 CPU 等了多久，用来区分「CPU 真的忙」和「进程在排队」",
		Program: `tracepoint:sched:sched_wakeup { @start[args->pid] = nsecs; }
tracepoint:sched:sched_switch /@start[args->prev_pid]/ { @us[args->prev_comm] = hist((nsecs - @start[args->prev_pid]) / 1000); delete(@start[args->prev_pid]); }`,
	},
	{
		ID:          "offcputime",
		Description: "off-CPU 时间：进程没在跑的时间按进程累计，用来找阻塞在 IO、锁或 futex 上的东西",
		Program: `tracepoint:sched:sched_switch { @start[args->prev_pid] = nsecs; }
tracepoint:sched:sched_switch /@start[args->next_pid]/ { @us[args->next_comm] = sum((nsecs - @start[args->next_pid]) / 1000); delete(@start[args->next_pid]); }`,
	},
	{
		ID:          "biolatency",
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

// TraceCommand is the remote command line for one trace: fixed flags, a
// validated image, a catalogue program and a duration. Nothing the agent wrote
// reaches the shell that starts docker -- the script travels on stdin.
func TraceCommand(image string, script TraceScript, seconds int, network bool) (string, error) {
	image = strings.TrimSpace(image)
	if !sandboxImagePattern.MatchString(image) {
		return "", fmt.Errorf("镜像名不合法：%q", image)
	}
	if seconds < 3 || seconds > TraceMaxSeconds {
		return "", fmt.Errorf("采样窗口必须在 3 到 %d 秒之间", TraceMaxSeconds)
	}

	flags := TraceShape
	if network {
		flags = strings.Replace(flags, "--network=none", traceFlagsWithNetwork, 1)
	}

	return flags + " " + image + " " + sandboxScriptPath, nil
}

// RunTrace runs one catalogue program inside the trace shape. A run that
// outlives its deadline is killed by name, because a trace that keeps running
// keeps costing the host it is watching.
func (c *Client) RunTrace(ctx context.Context, image string, script TraceScript, seconds int, network bool) (Result, error) {
	command, err := TraceCommand(image, script, seconds, network)
	if err != nil {
		return Result{}, err
	}

	name := "opsagi-trace-" + sandboxSuffix()
	if name != "opsagi-trace-" {
		command = strings.Replace(command, "docker run --rm -i", "docker run --rm -i --name "+name, 1)
	}
	res, err := c.RunStdin(ctx, command, TraceProgram(script, seconds))
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
	return fmt.Sprintf(`if ! command -v bpftrace >/dev/null 2>&1; then
  echo "bpftrace 不在镜像里。换一个带 bpftrace 的镜像（OPSAGI_BPF_IMAGE），或者让操作员放开出网后由本脚本安装。" >&2
  exit 127
fi
bpftrace -q -e '%s
interval:s:%d { exit(); }'
`, script.Program, seconds)
}
