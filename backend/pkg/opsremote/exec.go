package opsremote

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ParamKind decides how one parameter of a catalogue command is validated
// before it reaches the command line.
type ParamKind int

const (
	// ParamName accepts a container or unit name: letters, digits and . _ -
	ParamName ParamKind = iota
	// ParamPath accepts an absolute path: letters, digits and . _ - /
	ParamPath
	// ParamCount accepts a whole number inside a range
	ParamCount
	// ParamImage accepts an image reference, which needs : and / to carry a tag
	ParamImage
)

// Param is one placeholder of a catalogue command. Template names it as
// %<Template>s, and the validator is what stands between the argument and the
// shell, so a param that cannot be described by one of these kinds does not
// belong in the catalogue.
type Param struct {
	Template string
	Kind     ParamKind
	Min      int
	Max      int
	Default  int
}

// Command is one read-only command an agent may run by name. The template is a
// constant in this package: an agent chooses the entry, never the text.
type Command struct {
	ID          string
	Description string
	Template    string
	Params      []Param
}

var (
	namePattern  = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)
	pathPattern  = regexp.MustCompile(`^/[a-zA-Z0-9_.\-/]{1,255}$`)
	imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-/:@]{0,200}$`)
	catalogOrder = []string{
		"listen_ports",
		"established",
		"disk_usage",
		"inode_usage",
		"memory",
		"load",
		"psi",
		"top_cpu",
		"top_mem",
		"unit_status",
		"unit_logs",
		"kernel_log",
		"docker_inspect",
		"docker_stats",
		"file_stat",
		"package_history",
		"unit_activated_at",
		"container_image",
		"image_history",
		"pg_stat_activity",
		"pg_stat_statements",
		"pg_explain",
	}
	catalog = map[string]Command{
		"listen_ports": {
			ID:          "listen_ports",
			Description: "正在监听的 TCP/UDP 端口及其所属进程（ss -lntup）",
			Template:    `ss -lntup 2>/dev/null`,
		},
		"established": {
			ID:          "established",
			Description: "处于 established 状态的 TCP 连接与对端地址（ss -tanp）",
			Template:    `ss -tanp state established 2>/dev/null`,
		},
		"disk_usage": {
			ID:          "disk_usage",
			Description: "各文件系统容量与使用率（df -hP）",
			Template:    `df -hP 2>/dev/null`,
		},
		"inode_usage": {
			ID:          "inode_usage",
			Description: "各文件系统 inode 使用率（df -iP）",
			Template:    `df -iP 2>/dev/null`,
		},
		"memory": {
			ID:          "memory",
			Description: "内存与 Swap 的 MB 用量（free -m）",
			Template:    `free -m 2>/dev/null`,
		},
		"load": {
			ID:          "load",
			Description: "1/5/15 分钟平均负载与可运行进程数（/proc/loadavg 与 nproc）",
			Template:    `cat /proc/loadavg 2>/dev/null; nproc 2>/dev/null`,
		},
		"psi": {
			ID:          "psi",
			Description: "PSI 压力停顿：CPU、IO、内存各自有多少时间被拖住，以及最短停顿 10 秒与 60 秒的占比（/proc/pressure）",
			Template:    `for f in cpu io memory; do printf "%s: " "$f"; cat /proc/pressure/$f 2>/dev/null || echo unavailable; done`,
		},
		"top_cpu": {
			ID:          "top_cpu",
			Description: "按 CPU 占用排序的前 N 个进程（默认 15）",
			Template:    `ps -eo pid,ppid,pcpu,pmem,rss,etime,comm --sort=-pcpu 2>/dev/null | head -n %Lines%`,
			Params:      []Param{{Template: "Lines", Kind: ParamCount, Min: 1, Max: 200, Default: 15}},
		},
		"top_mem": {
			ID:          "top_mem",
			Description: "按常驻内存排序的前 N 个进程（默认 15）",
			Template:    `ps -eo pid,ppid,pcpu,pmem,rss,etime,comm --sort=-rss 2>/dev/null | head -n %Lines%`,
			Params:      []Param{{Template: "Lines", Kind: ParamCount, Min: 1, Max: 200, Default: 15}},
		},
		"unit_status": {
			ID:          "unit_status",
			Description: "一个 systemd 单元的状态、主进程与最近日志（systemctl status）",
			Template:    `systemctl status %Unit% --no-pager -l 2>&1`,
			Params:      []Param{{Template: "Unit", Kind: ParamName}},
		},
		"unit_logs": {
			ID:          "unit_logs",
			Description: "一个 systemd 单元最近 N 行日志（默认 100）",
			Template:    `journalctl -u %Unit% -n %Lines% --no-pager -o short-iso 2>/dev/null`,
			Params: []Param{
				{Template: "Unit", Kind: ParamName},
				{Template: "Lines", Kind: ParamCount, Min: 1, Max: 2000, Default: 100},
			},
		},
		"kernel_log": {
			ID:          "kernel_log",
			Description: "内核环形缓冲区的最近 N 行，带时间戳（默认 100）",
			Template:    `dmesg -T 2>/dev/null | tail -n %Lines%`,
			Params:      []Param{{Template: "Lines", Kind: ParamCount, Min: 1, Max: 500, Default: 100}},
		},
		"docker_inspect": {
			ID:          "docker_inspect",
			Description: "一个容器的运行状态：状态、退出码、重启次数、OOMKilled、启动与结束时刻、重启策略、挂载（bind 的宿主机源路径与读写属性）",
			Template:    `docker inspect %Container% --format '{"status":"{{.State.Status}}","running":{{.State.Running}},"exit_code":{{.State.ExitCode}},"oom_killed":{{.State.OOMKilled}},"restart_count":{{.RestartCount}},"started_at":"{{.State.StartedAt}}","finished_at":"{{.State.FinishedAt}}","restart_policy":"{{.HostConfig.RestartPolicy.Name}}","pid":{{.State.Pid}},"mounts":{{json .Mounts}}}' 2>&1`,
			Params:      []Param{{Template: "Container", Kind: ParamName}},
		},
		"docker_stats": {
			ID:          "docker_stats",
			Description: "各容器一次性的 CPU、内存、网络与块设备用量采样（docker stats --no-stream）",
			Template:    `docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}\t{{.PIDs}}' 2>/dev/null`,
		},
		"file_stat": {
			ID:          "file_stat",
			Description: "一个绝对路径的元数据：大小、属主、权限与三个时间（stat）",
			Template:    `stat %Path% 2>&1`,
			Params:      []Param{{Template: "Path", Kind: ParamPath}},
		},
		"package_history": {
			ID:          "package_history",
			Description: "最近安装与升级的软件包及其时刻，带原始时间戳（dpkg 或 dnf 历史）",
			Template:    `if [ -r /var/log/dpkg.log ]; then grep -h ' install \| upgrade ' /var/log/dpkg.log /var/log/dpkg.log.1 2>/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf history --reverse 2>/dev/null; else echo 'no package history on this host'; fi | tail -n %Lines%`,
			Params:      []Param{{Template: "Lines", Kind: ParamCount, Min: 1, Max: 500, Default: 40}},
		},
		"unit_activated_at": {
			ID:          "unit_activated_at",
			Description: "一个 systemd 单元的激活、失活与主进程启动时刻，以及它的单元文件路径（systemctl show）",
			Template:    `systemctl show %Unit% -p Id -p FragmentPath -p ActiveState -p SubState -p ActiveEnterTimestamp -p InactiveEnterTimestamp -p ExecMainStartTimestamp -p NRestarts 2>&1`,
			Params:      []Param{{Template: "Unit", Kind: ParamName}},
		},
		"container_image": {
			ID:          "container_image",
			Description: "一个容器所用的镜像引用、镜像 ID 与容器创建时刻，用来对齐变更时间线",
			Template:    `docker inspect %Container% --format 'image={{.Config.Image}} image_id={{.Image}} container_created={{.Created}}' 2>&1`,
			Params:      []Param{{Template: "Container", Kind: ParamName}},
		},
		"image_history": {
			ID:          "image_history",
			Description: "一个镜像的各层及其构建命令与时刻（docker history），用来判断镜像何时被换掉",
			Template:    `docker history --no-trunc --format '{{.CreatedAt}}  {{.Size}}  {{.CreatedBy}}' %Image% 2>&1 | head -n %Lines%`,
			Params: []Param{
				{Template: "Image", Kind: ParamImage},
				{Template: "Lines", Kind: ParamCount, Min: 1, Max: 50, Default: 15},
			},
		},
		"pg_stat_activity": {
			ID:          "pg_stat_activity",
			Description: "Postgres 容器里此刻还在跑的查询：会话状态、已跑多久、等待事件与语句开头（需要库名）",
			Template:    `docker exec %Container% psql -U postgres -d %Database% -Atc "select pid, state, round(extract(epoch from (now() - query_start))::numeric, 1) secs, coalesce(wait_event_type, '-') wait, left(translate(query, chr(10) || chr(13) || chr(9), '   '), 120) query from pg_stat_activity where state <> 'idle' order by query_start" 2>&1`,
			Params: []Param{
				{Template: "Container", Kind: ParamName},
				{Template: "Database", Kind: ParamName},
			},
		},
		"mysql_connections": {
			ID:          "mysql_connections",
			Description: "MySQL 容器的连接与容量：max_connections、Threads_connected、Threads_running、Aborted_connects（连接被拒的次数），用来区分“连不上”是网络问题还是连接数打满",
			Template:    `docker exec %Container% sh -c 'mysql -u root -p"$MYSQL_ROOT_PASSWORD" -N -e "show variables like \"max_connections\"; show status where Variable_name in (\"Threads_connected\",\"Threads_running\",\"Aborted_connects\",\"Connection_errors_max_connections\")"' 2>&1 | grep -v "Using a password"`,
			Params:      []Param{{Template: "Container", Kind: ParamName}},
		},
		"mysql_processlist": {
			ID:          "mysql_processlist",
			Description: "MySQL 容器里此刻的连接清单：来源地址、用户、库、命令、已执行秒数与语句开头（按时间倒序），用来看谁占着连接",
			Template:    `docker exec %Container% sh -c 'mysql -u root -p"$MYSQL_ROOT_PASSWORD" -e "select id, host, user, db, command, time, state, left(replace(info, char(10), \" \"), 90) info from information_schema.processlist order by time desc limit %Lines%"' 2>&1 | grep -v "Using a password"`,
			Params: []Param{
				{Template: "Container", Kind: ParamName},
				{Template: "Lines", Kind: ParamCount, Min: 1, Max: 100, Default: 20},
			},
		},
		"pg_stat_statements": {
			ID:          "pg_stat_statements",
			Description: "Postgres 容器里累计耗时最高的 N 条语句：调用次数、总耗时、平均耗时、返回行数与语句开头（需要库名与 pg_stat_statements 扩展）",
			Template:    `docker exec %Container% psql -U postgres -d %Database% -Atc "select calls, round(total_exec_time::numeric, 1) total_ms, round(mean_exec_time::numeric, 2) mean_ms, rows, left(translate(query, chr(10) || chr(13) || chr(9), '   '), 160) query from pg_stat_statements order by total_exec_time desc limit %Lines%" 2>&1`,
			Params: []Param{
				{Template: "Container", Kind: ParamName},
				{Template: "Database", Kind: ParamName},
				{Template: "Lines", Kind: ParamCount, Min: 1, Max: 50, Default: 10},
			},
		},
		"pg_explain": {
			ID:          "pg_explain",
			Description: "解释 Postgres 里累计耗时第 N 条语句的执行计划，序号与 pg_stat_statements 的输出顺序一致：只取计划、不执行（不加 ANALYZE，带参数的语句走 EXPLAIN GENERIC_PLAN，需要 PostgreSQL 16 及以上）。需要 container 与 database，rank 从 1 起",
			Template: `docker exec -i %Container% psql -U postgres -d %Database% -At -v ON_ERROR_STOP=0 -f - 2>&1 <<'SQL'
select coalesce(
  (select 'explain (format text, verbose off, generic_plan) ' || query
   from pg_stat_statements
   where dbid = (select oid from pg_database where datname = current_database())
   order by total_exec_time desc
   offset %Rank% - 1
   limit 1),
  'select ''pg_stat_statements 里没有第 %Rank% 条语句'''
) \gexec
SQL`,
			Params: []Param{
				{Template: "Container", Kind: ParamName},
				{Template: "Database", Kind: ParamName},
				{Template: "Rank", Kind: ParamCount, Min: 1, Max: 50, Default: 1},
			},
		},
	}
)

// Commands lists the catalogue in a stable order.
func Commands() []Command {
	out := make([]Command, 0, len(catalogOrder))
	for _, id := range catalogOrder {
		out = append(out, catalog[id])
	}
	return out
}

// Lookup returns the catalogue entry with this id.
func Lookup(id string) (Command, bool) {
	command, ok := catalog[id]
	return command, ok
}

// CommandIDs lists the ids the caller may pass, sorted for a deterministic error message.
func CommandIDs() []string {
	ids := make([]string, 0, len(catalog))
	for id := range catalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Render turns a catalogue id plus arguments into the command line to run. Every
// value is validated against the kind its Param declares and every template is a
// constant of this package, so no caller-supplied text is ever concatenated into
// a shell fragment.
func Render(id string, args map[string]string) (string, error) {
	command, ok := catalog[id]
	if !ok {
		return "", fmt.Errorf("未知的只读命令 %q，可用命令：%s", id, strings.Join(CommandIDs(), ", "))
	}

	rendered := command.Template
	for _, param := range command.Params {
		raw := strings.TrimSpace(args[param.Template])
		placeholder := "%" + param.Template + "%"

		value, err := renderParam(param, raw)
		if err != nil {
			return "", err
		}
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}

	if idx := strings.Index(rendered, "%"); idx >= 0 {
		return "", fmt.Errorf("命令模板 %q 还留着未替换的占位符", id)
	}

	return rendered, nil
}

// Watch bounds: a watch is a short window, not a monitor. The upper end is what
// one agent turn can wait for without the host being sampled by a process nobody
// is watching any more.
const (
	WatchMaxSamples  = 30
	WatchMaxInterval = 30
	WatchMaxSeconds  = 300
)

// RenderWatch wraps a catalogue command in a sampling loop that runs on the host,
// so one round trip covers the whole window. The loop text is fixed and the inner
// command comes from Render, so nothing caller-supplied reaches the shell.
func RenderWatch(id string, args map[string]string, samples, interval int) (string, error) {
	if samples < 2 {
		return "", fmt.Errorf("采样次数至少 2 次，收到 %d", samples)
	}
	if samples > WatchMaxSamples {
		return "", fmt.Errorf("采样次数最多 %d 次，收到 %d", WatchMaxSamples, samples)
	}
	if interval < 1 {
		return "", fmt.Errorf("采样间隔至少 1 秒，收到 %d", interval)
	}
	if interval > WatchMaxInterval {
		return "", fmt.Errorf("采样间隔最多 %d 秒，收到 %d", WatchMaxInterval, interval)
	}
	if seconds := (samples - 1) * interval; seconds > WatchMaxSeconds {
		return "", fmt.Errorf("采样窗口 %d 秒超过上限 %d 秒：减少次数或间隔", seconds, WatchMaxSeconds)
	}

	command, err := Render(id, args)
	if err != nil {
		return "", err
	}

	step := strconv.Itoa(interval)

	return "for i in $(seq 1 " + strconv.Itoa(samples) + "); do " +
		"echo \"== sample $i t=$(date +%s)\"; " + command + "; " +
		"if [ $i -lt " + strconv.Itoa(samples) + " ]; then sleep " + step + "; fi; done", nil
}

func renderParam(param Param, raw string) (string, error) {
	switch param.Kind {
	case ParamName:
		if raw == "" {
			return "", fmt.Errorf("缺少参数 %s：需要一个容器名或 systemd 单元名", param.Template)
		}
		name := strings.TrimPrefix(raw, "/")
		if !namePattern.MatchString(name) {
			return "", fmt.Errorf("参数 %s 取值 %q 非法：只允许字母、数字与 . _ -，最长 128 个字符", param.Template, raw)
		}
		return name, nil

	case ParamPath:
		if raw == "" {
			return "", fmt.Errorf("缺少参数 %s：需要一个绝对路径", param.Template)
		}
		if !pathPattern.MatchString(raw) {
			return "", fmt.Errorf("参数 %s 取值 %q 非法：必须是绝对路径，只允许字母、数字与 . _ - /，最长 255 个字符", param.Template, raw)
		}
		return raw, nil

	case ParamImage:
		if raw == "" {
			return "", fmt.Errorf("缺少参数 %s：需要一个镜像引用，例如 nginx:1.27-alpine", param.Template)
		}
		if !imagePattern.MatchString(raw) {
			return "", fmt.Errorf("参数 %s 取值 %q 非法：只允许字母、数字与 . _ - / : @，必须以字母或数字开头，最长 201 个字符", param.Template, raw)
		}
		return raw, nil

	case ParamCount:
		if raw == "" {
			return strconv.Itoa(param.Default), nil
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return "", fmt.Errorf("参数 %s 取值 %q 非法：需要一个整数", param.Template, raw)
		}
		if value < param.Min || value > param.Max {
			return "", fmt.Errorf("参数 %s 取值 %d 超出范围：允许 %d 到 %d", param.Template, value, param.Min, param.Max)
		}
		return strconv.Itoa(value), nil
	}

	return "", fmt.Errorf("参数 %s 的类型未知", param.Template)
}
