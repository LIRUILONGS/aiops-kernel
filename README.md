<p align="center">
  <img src="assets/logo.svg" alt="OpsAGI" width="112">
</p>

<h1 align="center">OpsAGI</h1>

<p align="center"><b>一个会自己取证、自己下结论的 AIOps 智能体内核：面向真实主机的故障定位与性能优化，交付物是经过代码校验的结构化结论，不是聊天记录。</b></p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/LIRUILONGS/aiops-kernel" alt="License"></a>
  <a href="backend/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/LIRUILONGS/aiops-kernel?filename=backend/go.mod" alt="Go"></a>
  <img src="https://img.shields.io/badge/%E7%95%8C%E9%9D%A2-%E5%85%A8%E4%B8%AD%E6%96%87-f59e0b" alt="界面全中文">
  <a href="https://github.com/LIRUILONGS/aiops-kernel/commits/main"><img src="https://img.shields.io/github/last-commit/LIRUILONGS/aiops-kernel" alt="Last commit"></a>
</p>

<p align="center"><b>导航：</b> <a href="#它是什么">它是什么</a> · <a href="#一次诊断怎么走">一次诊断怎么走</a> · <a href="#界面">界面</a> · <a href="#它不做什么">它不做什么</a> · <a href="#约束落在哪里">约束落在哪里</a> · <a href="#工具">工具</a> · <a href="#快速开始">快速开始</a> · <a href="#验证记录">验证记录</a> · <a href="#文档">文档</a></p>

---

## 它是什么

OpsAGI 是 [PentAGI](https://github.com/vxcontrol/pentagi)（MIT）的分支。上游是自主渗透测试平台；这棵树保留它的运行时——flow / task / subtask 调度、模型提供商层、工具调用执行器、GraphQL API 与 React 界面——把智能体、工具与提示词换成值班工程师的那一套：SSH 上的只读取证、可回溯的诊断结论、带实测判定的性能改动。

一次运行留下的不是一段话，而是两种结构化结论之一：`diagnosis`（根因一句话、至少两条能复现的证据查询、置信度、影响面、建议动作与回滚方式、是否需要人工介入）或 `optimization_result`（改前基线、复测实测、判定与差值）。两者都由代码校验，不合格会被退回重填。

界面全中文，默认深色主题。

## 一次诊断怎么走

1. 输入一条告警（或一句"这台机器为什么慢"），指挥官把它拆成几条**可验证的假设**：计划是数据，每条假设带 `diagnosis` 或 `optimization` 类型。
2. 每个证据面有一位分析师，以**工具**的形式挂在指挥官手里：`triage`（分级）、`log_analyst`（日志）、`metrics_analyst`（指标）、`change_analyst`（变更）。调用一个分析师就是跑它自己的 chain，返回它的 finding。
3. 分析师用工具取证：目标主机上的只读 SSH 命令、一次性采集容器、eBPF 追踪，或者直接问监控系统。
4. 指挥官用 `diagnosis` 或 `optimization_result` 收尾。结论由代码校验：证据必须点名它来自哪个工具，性能判定必须带基线与复测，不合格的那次工具调用被拒绝，模型自己重填。

## 界面

图来自本项目自己在参照靶场上的真实运行（[交接文档第 5 节](docs/aiops/03-handover.md)）：会话标题是当时贴进去的告警原文，角色视图里的 token 与 chain 数是那几次运行留下的，不是演示夹具。图随 GitHub 主题切换。

<table>
  <tr>
    <td colspan="2" align="center">
      <a href="assets/screenshots/dashboard-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/dashboard-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/dashboard-light.png">
        <img src="assets/screenshots/dashboard-light.png" alt="概览" width="100%">
      </picture></a>
      <br>
      <sub><b>概览。</b> 会话活动、工具调用与 Token 用量按天铺开——哪几天真在干活，一眼看得出。</sub>
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <a href="assets/screenshots/flows-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/flows-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/flows-light.png">
        <img src="assets/screenshots/flows-light.png" alt="会话列表" width="100%">
      </picture></a>
      <br>
      <sub><b>会话列表。</b> 标题就是告警原文，状态、耗时与终端镜像都在列表上。</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <a href="assets/screenshots/flow-conclusion-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/flow-conclusion-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/flow-conclusion-light.png">
        <img src="assets/screenshots/flow-conclusion-light.png" alt="会话详情" width="100%">
      </picture></a>
      <br>
      <sub><b>会话详情。</b> 一份真跑出来的收尾报告：证据缺口逐条列明，置信度 0.60，`needs_human=true`。</sub>
    </td>
    <td width="50%" align="center">
      <a href="assets/screenshots/flow-roles-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/flow-roles-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/flow-roles-light.png">
        <img src="assets/screenshots/flow-roles-light.png" alt="角色视图" width="100%">
      </picture></a>
      <br>
      <sub><b>角色视图。</b> 十二个角色按层摆放，节点带各自的 chain 数、日志条数与 token 用量。</sub>
    </td>
  </tr>
</table>

一个 flow 的右侧是可切换的观察面：概览、终端、任务、智能体、**角色**、工具调用、向量库、文件、截图。

**角色**页回答"这一轮是谁在干活"。上方是按层摆放的十二个角色拓扑——规划（生成 / 精炼）在上，指挥官居中，四个取证分析师在下，消费与修补型角色在底；节点上带该角色在本 flow 的 chain 数、日志条数、token 用量与最后活动时间，一分钟内有动作的角色描一圈绿边，发生过的委派用带次数的箭头连起来。下方是按角色过滤的日志：点节点或点角色标签，只看这个角色写下或应答的条目。图用 [`@xyflow/react`](https://reactflow.dev) 画，数据来自 `msgchains` 与 `agentlogs` 两条聚合。

**租约**面板只对非 `off` 的 flow 出现：列出每一条能力判定，可按"拦截 / 将被拦截 / 放行"筛，`would_deny` 逐条给出拒绝原因——这就是把 `shadow` 收窄成 `enforce` 的依据。

## 它不做什么

- **不改线上状态。** 唯一的写通道是 `ops_apply`，它只能执行代码里的修复动作目录（重启单元、重启容器、用快照覆盖配置、清理过期文件、重启 k8s 工作负载），不接自由命令；默认还要一道审批，租约里没有这条动作就只提交审批、不动手。修复之后必须重新采集、再与故障前的基线对比，没带复测的改善判定过不了校验。
- **不做单点猜测。** 单一信号只能提示原因，下根因结论至少要两个独立来源交叉验证。
- **不假装查过。** 没调用的工具、没看到的日志行、没发生的对话，都不允许出现在报告里：结论里的每条证据都要点名它来自哪个工具，点不出来就被退回。
- **不为了让工单好看而编造故障。** 日志全绿时，正确答案是顶回去，并说明下一步该查哪里。

## 约束落在哪里

模型可以自由选工具，但它选不到的工具就是不存在；它想交的结论要过代码校验，过不了就重来。分层不是为了整洁，是让每条约束有个落点：

| 层 | 位置 | 职责 |
|---|---|---|
| L0 模型 | 任意 OpenAI 兼容端点 | 决定查什么、怎么查 |
| L1 Harness | `backend/pkg/tools` | 工具表与调用执行、容器生命周期 |
| L2 控制面 | `backend/pkg/controller` | flow / task / subtask 调度、状态机、日志与订阅 |
| L3 领域 | `backend/pkg/templates/prompts/*.tmpl`、`pkg/tools/aiops.go` | 角色提示词、出口 schema、校验规则 |

四道闸门叠起来，越权要同时穿过四道：

1. **角色工具表**（代码）：每个角色能看见哪些工具写在代码里，表外的东西模型根本选不到。
2. **固定目录与固定形状**：`ops_exec` 只能从只读命令目录里挑 ID 与参数，不接自由命令；`ops_sandbox` 的两种形状都把宿主机 `/proc`、`/sys` 只读挂载，`--cap-drop=ALL`，不进宿主机 PID 命名空间——能读到宿主机的一切，却改不了它。
3. **能力租约**（`backend/pkg/capability`）：每条 flow 创建时选"关闭 / 观察 / 强制"。`shadow` 只记录"这条会被拒绝"，`enforce` 才真的拒绝；判定逐条写入 `capability_decisions`，切面在工具表最外层包住 handler；租约按子任务签发、落库、重启后恢复，不会重新放宽。默认策略刻意等于租约出现之前的语义，所以打开 `enforce` 不会让一条正常工作的 flow 突然失败——只有把策略改窄之后它才开始咬人。
4. **策略即环境变量**：能连哪台主机、能不能出网、能不能拉镜像、开不开 eBPF，都由操作员配置，不写在提示词里。

### 策略即环境变量

| 变量 | 默认 | 作用 |
|---|---|---|
| `OPSAGI_TARGETS` | 空 | 目标主机清单（JSON 数组：id / host / port / user / password 或 key） |
| `OPSAGI_POLICY` | 空 | 能力租约策略文件（YAML）；不配则用内置默认策略 |
| `OPSAGI_DEFAULT_SANDBOX_IMAGE` | 空 | 采集容器镜像；空则回落到 `DOCKER_DEFAULT_IMAGE_FOR_PENTEST` 或内置镜像 |
| `PROMETHEUS_URL` | 空 | 配了才有 `prom_query` |
| `K8S_KUBECONFIG` | 空 | 配了才有 `k8s_get` |
| `K8S_API_URL` | 空 | 直接给集群 API 地址，与 kubeconfig 二选一 |
| `K8S_INSECURE_SKIP_TLS_VERIFY` | `false` | 跳过集群证书校验 |
| `OPSAGI_SANDBOX_NETWORK` | `false` | 允许采集容器出网（`workshop` 形状的前提） |
| `OPSAGI_SANDBOX_PULL` | `false` | 允许拉取镜像 |
| `OPSAGI_SANDBOX_BPF` | `false` | 打开 `ops_trace`；关着时工具不存在 |
| `OPSAGI_BPF_IMAGE` | `quay.io/iovisor/bpftrace:latest` | 追踪容器镜像，里面要有 `bpftrace` |
| `ASK_USER` | `false` | 打开 `ask` 工具（人工闸门） |
| `OPSAGI_SUBTASK_CONCURRENCY` | `1` | 同一任务内子任务并发度 |

没配的东西不会出现在工具表里，而不是出现了再报错。

## 工具

只读取证（`OPSAGI_TARGETS` 里的主机，走 SSH）：

| 工具 | 做什么 |
|---|---|
| `ops_system` | 主机概况：运行时长、负载、CPU、内存与 Swap、文件系统、CPU 占用最高的进程 |
| `ops_containers` | 容器清单：状态、重启次数、退出码、镜像，以及 failed 的 systemd 单元 |
| `ops_logs` | 日志尾部：按容器名读 `docker logs`，或读最近 30 分钟的系统错误日志 |
| `ops_baseline` | 采一次带时间戳的指纹，性能类目标的"改前" |
| `ops_diff` | 对比两次指纹，逐项给变化量、百分比与"改善 / 恶化 / 持平"判定 |
| `ops_exec` | 按 ID 执行一条预置的只读诊断命令，覆盖其他工具没到的角度 |
| `ops_watch` | 把一条只读命令在时间窗内重复采样，抓"正在发生"的东西 |

自由通道（边界在容器形状，不在命令名单）：

- `ops_sandbox` — 在目标主机上起一次性容器执行脚本。`probe`（默认：`--network=none --read-only --memory=256m`）与 `workshop`（可写层、可出网、512MB，用来装工具）。两者都 `--cap-drop=ALL --security-opt=no-new-privileges --pids-limit=256 --cpus=1`，宿主机的 `/proc`、`/sys` 只读挂在 `/host/proc`、`/host/sys`。

内核层（`OPSAGI_SANDBOX_BPF` 打开时）：

- `ops_trace` — 五个固定程序（`runqlat` / `offcputime` / `biolatency` / `execsnoop` / `tcpconnlat`），程序正文在代码里，模型只选 ID 与采样窗口。容器丢光 capability 后加回 `BPF`、`PERFMON`、`SYS_RESOURCE`，`/sys/kernel` 只读挂载，仍无网络、不进宿主机 PID 命名空间；老内核因为缺 `SYS_ADMIN` 拒绝 `perf_event_open` 时，自动补这一个能力重试一次——第一次永远是最窄的形状。

旁路（直接问监控系统，不经过主机 shell）：

- `prom_query`（`PROMETHEUS_URL`）、`k8s_get`（`K8S_KUBECONFIG` 或 `K8S_API_URL`）。

写入（唯一的写通道）：

- `ops_apply` — 按 ID 执行一条预置修复动作：`unit_restart`、`container_restart`、`config_restore`、`disk_cleanup`、`k8s_rollout_restart`。命令文本来自代码目录，参数只允许字母、数字与 `._-/`，路径参数限制在模板声明的目录之下；租约里没有这条动作时只提交审批，不动手。

记录与出口：

- `record_finding` 把一条结论写进事故记录（标题、含义、原始输出）——聊天记录会滚走，它不会。
- 出口是 `diagnosis`、`optimization_result`、`triage_result`、`log_finding`、`metric_finding`、`change_finding`。

## 快速开始

需要一个 PostgreSQL（带 pgvector）和一个 OpenAI 兼容的模型端点。

```bash
cd backend
go build -o opsagi ./cmd/opsagi

export DATABASE_URL='postgres://opsagi:opsagi@localhost:5432/opsagi?sslmode=disable'
export LLM_SERVER_URL='http://127.0.0.1:8000/v1'
export LLM_SERVER_KEY='sk-...'
export LLM_SERVER_MODEL='your-model'
export OPSAGI_TARGETS='[{"id":"lab","host":"10.0.0.5","port":22,"user":"root","password":"..."}]'

./opsagi
```

打开 `http://localhost:8080`（`SERVER_PORT` 可改），先建管理员：

```bash
go run ./cmd/opsagi-seed -mail ops@example.com -name "值班" -password-stdin
```

然后在界面上新建会话，或直接用命令行跑一次并取回结论：

```bash
python scripts/opsagi_run.py "lab 上的 WordPress 站点 500，帮我定位根因"
```

**验收自检**：不需要真实主机和真实模型也能跑——起一个假模型、起服务、建管理员，断言一条流程以合法的诊断收尾、另一条以带判定的性能结论收尾：

```bash
python scripts/aiops_acceptance/run.py
```

需要 Docker（流程自带的终端容器），通常不到一分钟（首次要等服务与容器起来，超时上限更宽），任何一步不成立都会非零退出。

## 验证记录

[`scripts/scenarios/`](scripts/scenarios/) 里有八个场景，都是这套内核被真模型、真主机跑过之后留下来的：靶场要摆成什么样、对产品说哪一句话、什么算答对、什么算糊弄，各写在一份 JSON 里。

| 场景 | 验的是 | 实跑记录 |
|---|---|---|
| `s1-dependency-container-exit` | 健康检查绿着、接口 500 时，根因能不能落到依赖容器的退出上 | flow 125 |
| `s2-slow-query-missing-index` | 性能结论里的毫秒数是不是量出来的，没改成敢不敢填 `goal_met=false` | flow 126 |
| `s3-row-lock-wait` | 现场等待链与日志里的历史等待分不分得开，阻塞方消失后会不会硬编一个 pid | flow 127 |
| `s4-log-archive-replay` | 读数是回放还是实时，能不能从时间戳上看出来并改口 | flow 128 |
| `s5-scheduling-latency-trace` | `ops_trace` 的直方图汇总能不能进结论，数字否定报障假设时敢不敢说 | flow 136 |
| `s6-statement-baseline` | `ops_baseline` → `ops_diff` 的闭环，自比对会不会被拦下 | flow 129 |
| `l1-hdfs-block-warnings` | 从真实日志原文里读次数、block 数与故障性质 | — |
| `l4-spark-healthy-control` | 对照组：日志全 INFO 时，敢不敢顶回"没病" | — |

八个里两个是控制组（`l4` 全 INFO，`s3` 现场已无阻塞方）。控制组比故障组更容易露馅：故障场景答错只是不准，控制场景编造就是硬伤。

跑其中一个：先把 `fixture` 里的命令执行掉，再把 `task` 交给产品——

```bash
python scripts/opsagi_run.py "$(python -c "import json;print(json.load(open('scripts/scenarios/s1-dependency-container-exit.json'))['task'])")"
```

`fixture` 的命令是给[参照靶场](docs/aiops/03-handover.md)用的；换环境按自己的容器名和路径改。场景的字段含义、判卷口径与新增场景的写法见 [`scripts/scenarios/README.md`](scripts/scenarios/README.md)。

真模型、真靶场下跑通过的：

- **故障定位**：容器依赖退出（健康检查仍在 200）、真实行锁等待、sshd 日志归档回放、eBPF 排队延迟（结论否定了报障假设）。
- **性能优化**：pgbench 五百万行的报表查询，基线 → 执行计划 → DDL → 复测 → `ops_diff` 判定；没测到改善时如实给出 `unverified`。
- **纯日志**：六个 Loghub 真实样本（HDFS / Linux 认证 / Apache / Spark / Zookeeper / OpenStack），含"全 INFO 应当答健康"的对照组。
- **界面**：容器终端实时回显、角色拓扑、会话结论渲染、租约判定面板。

尚未验证：多目标主机并发、长时间运行下的资源泄漏、非 Debian 系的宿主机取证、k8s 崩溃循环场景、Prometheus 旁路在大规模指标下的表现。

## 目录

```
backend/                 Go 服务：REST + GraphQL
  cmd/opsagi/            服务入口
  cmd/opsagi-seed/       管理员与密码种子
  pkg/controller/        flow / task / subtask 运行时
  pkg/capability/        能力租约：判定、策略、策略文件
  pkg/tools/             工具表与执行器；ops.go / aiops.go 是本项目新增的取证与出口层
  pkg/opsremote/         只读命令目录、SSH 执行、采集容器与 eBPF 程序
  pkg/templates/prompts/ 角色提示词（中文）
frontend/                React + TypeScript 界面
docs/aiops/              设计、交接、验收记录
docs/platform-reference.md  继承自 PentAGI 的平台文档（提供商与模型目录、部署细节）
scripts/                 命令行工具与批测脚本
scripts/scenarios/       实验场景：靶场怎么摆、问什么、什么算答对
```

## 文档

- [总览](docs/aiops/00-overview.md) — 这棵树要做什么，与上游的关系，分层为什么这样切
- [M0 计划](docs/aiops/01-m0-plan.md) — 只读诊断的端到端目标与验收口径
- [改造清单](docs/aiops/02-changes.md) — 相对上游的删 / 改 / 留 / 增，精确到路径
- [交接文档](docs/aiops/03-handover.md) — 当前状态、验证到什么程度、哪里还没验证（另有 [.docx](docs/aiops/03-handover.docx)）
- [能力租约](docs/aiops/04-capability-plan.md) — 每 flow 的开关、判定点、审计表与界面，以及还没做的部分
- [平台参考](docs/platform-reference.md) — 从上游继承的部署与提供商配置细节

## 许可

代码遵循 MIT（见 [LICENSE](LICENSE)），并受 [EULA](EULA.md) 约束；上游版权归 PentAGI 开发团队。
