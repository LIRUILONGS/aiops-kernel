<p align="center">
  <img src="assets/logo.svg" alt="OpsAGI" width="112">
</p>

<h1 align="center">OpsAGI</h1>

<p align="center"><b>一个多 Agent 协作的 AIOps 智能体内核：面向 SRE 的真实主机故障定位与性能优化，交付物是经过代码校验的结构化结论，不是聊天记录。</b></p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/LIRUILONGS/aiops-kernel" alt="License"></a>
  <a href="backend/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/LIRUILONGS/aiops-kernel?filename=backend/go.mod" alt="Go"></a>
  <img src="https://img.shields.io/badge/%E7%95%8C%E9%9D%A2-%E4%B8%AD%E6%96%87-f59e0b" alt="界面中文">
  <a href="https://github.com/LIRUILONGS/aiops-kernel/commits/main"><img src="https://img.shields.io/github/last-commit/LIRUILONGS/aiops-kernel" alt="Last commit"></a>
</p>

<p align="center"><b>导航：</b> <a href="#它是什么">它是什么</a> · <a href="#一次诊断怎么走">一次诊断怎么走</a> · <a href="#多角色如何协作">多角色如何协作</a> · <a href="#角色">角色</a> · <a href="#工具">工具</a> · <a href="#它不做什么">它不做什么</a> · <a href="#约束落在哪里">约束落在哪里</a> · <a href="#界面">界面</a> · <a href="#快速开始">快速开始</a> · <a href="#验证记录">验证记录</a> · <a href="#开发">开发</a> · <a href="#目录">目录</a> · <a href="#文档">文档</a> · <a href="#许可">许可</a></p>

---

## 它是什么

OpsAGI 是 [PentAGI](https://github.com/vxcontrol/pentagi)（MIT 许可）的分支，上游是一个自主渗透测试平台。

OpsAGI 是一个多 Agent 协作的 AIOps 智能体内核：你给它一条告警，或者一句"这台机器为什么慢"，它自己规划、到真实主机上取证、自己下结论，必要时在授权范围内动手修，交回来的是一份经过代码校验的结构化结论，不是一段聊天记录。

- **面向谁。** SRE 与平台工程师。他们要的是"根因是什么、证据在哪、这个改动到底有没有用"，不是一个会聊天的助手。
- **怎么协作。** 一个 flow 先由诊断规划器把目标拆成几条可证伪的假设（通常 3–5 条，硬上限 5 条）；事故指挥逐条执行，自己能一次查清的就自己查，需要专业视角时派给四个分析师。一次委派不是一句"帮我看看"，而是起一条完整的专家 chain——自己的 prompt、自己的模型槽位、自己的工具白名单、自己的 transcript。专家用它的出口交回 finding，finding 作为这次工具调用的返回值回到指挥官的链上；两边各留一份记录，不共享上下文。分析师是叶子：工具表里只有取证工具、可选的 `ask` 和自己的出口，没有另一个分析师，所以往下不再分叉，指挥官是唯一看过全局的那个。
- **拿什么取证。** 真实主机上的只读 SSH 命令（命令目录写死在代码里，模型只能选 ID 和参数，选不了命令文本）、一次性采集容器（只读无网的 `probe`、可写层与出网的 `workshop`、独立的 eBPF 追踪容器）、Prometheus 兼容的指标接口、Kubernetes API。
- **拿什么收尾。** 两种结构化结论之一，都过代码校验：`diagnosis`（一句话根因、至少两条照 query 能复现的证据、0–1 的置信度、影响面、带风险等级与回滚方式的建议动作、是否需要人工介入）或 `optimization_result`（改前基线、复测实测、判定与逐项差值）。不合格的那次工具调用被拒绝，模型自己重填；`no_measurable_effect`（测过但没变化）是合法结论。
- **写是例外。** 修复只有一个通道 `ops_apply`，而且只在 flow 的租约是 `enforce` 时才出现在工具表里。它只能从代码里的动作目录挑一条（`unit_restart`、`container_restart`、`config_restore`、`disk_cleanup`、`k8s_rollout_restart`），租约没列的动作只提交审批、不动手。每次调用默认在同一个回合里先采基线、再动手、再复测，由代码判定这次改动到底有没有用。
- **怎么跑起来。** 后端 Go，对外 GraphQL 与 REST；前端 React，中文为主的界面、默认深色主题；状态落在 PostgreSQL + pgvector；模型提供商可换，从 OpenAI 到本地 Ollama。
- **平台侧的能力也在。** 会话模板、资源、知识库、向量库与交互式助手都在同一套界面里。

## 一次诊断怎么走

1. **建一条 flow。** 输入告警或性能目标，选 flow 类型：`diagnosis`（故障定位）、`optimization`（性能优化）或 `auto`（由计划自己决定每条子任务的类型）。
2. **规划器拆假设。** `generator` 把目标拆成几条可证伪的假设（硬上限 5 条），每条写清 `mode`、建议执行者 `assigned_to`、期望信号 `expected_signal` 与排除条件。计划是数据，不是一段散文。
3. **指挥官执行或委派。** `commander` 逐条推进：一次调用能回答的自己查；需要日志、指标、变更或分诊视角时，调用对应的分析师工具。一次委派会跑完一条完整的专家 chain，拿回它的 finding。
4. **分析师取证并收口。** 分析师用只读工具取证，用 `triage_result` / `log_finding` / `metric_finding` / `change_finding` 交回结论；每条结论都带可复现的 `query` 与原始 `excerpt`。
5. **指挥官收尾。** 用 `diagnosis` 或 `optimization_result` 结束，代码校验证据数量、字段与判定的一致性；不合格就退回重填。需要真实变更时，`ops_apply` 只在 flow 的租约是 `enforce`、且策略放行了这条动作（或操作员批准了这次实例）时才执行；它在同一个调用里先采基线、再动手、再复测，返回代码算出的改善 / 恶化 / 无变化 / 无法判定。

flow 类型是显式的（`diagnosis` / `optimization`）就覆盖计划里的 `mode`；只有 `auto` 才听计划的。所以用户说要优化，不会因为规划器猜成诊断而拿到一份诊断结论。

一条 flow 可以有多条 task；每个 task 的子任务按计划顺序执行（`OPSAGI_SUBTASK_CONCURRENCY` 可并行）。子任务完成后 `refiner` 按结果调整剩余计划，`reporter` 把 task 的结果写成报告。

## 多角色如何协作

多角色不是"让几个模型各答一遍再拼起来"。诊断链上的角色之间只有三样接口：一份结构化的计划、一次带 `question` 的委派、一张类型化的 finding。除此之外，它们不共享对话上下文，也不共享工具表。

### 一次委派的时序

```mermaid
sequenceDiagram
    autonumber
    participant G as generator
    participant C as commander
    participant A as 分析师

    G->>C: subtask_list（mode / assigned_to / expected_signal）
    C->>C: 取下一条子任务，决定自己查还是派活
    C->>A: 委派（question + 已知上下文）
    A->>A: 起一条独立 chain：自己的 prompt / 模型槽位 / 工具表
    A-->>C: triage_result / log_finding / metric_finding / change_finding
    C->>C: 合并证据，写 diagnosis 或 optimization_result
```

`reporter` 不在委派链里：子任务结出结果后，任务运行时才叫它写报告。图中也省略了任务运行时：`generator` 的 `subtask_list` 落库后，由它把每条子任务交给 `commander`。

三条硬边界：

1. **计划是数据，不是提示词。** `generator` 交的 `subtask_list` 每条都带 `mode`（`diagnosis` / `optimization`）、`assigned_to`（建议由谁执行）和 `expected_signal`（假设成立时应看到什么）。事故指挥拿到的是可逐条核对的计划，不是一段"帮我查查"的散文。
2. **委派是子 chain，不是转贴。** 指挥官调用 `triage` / `log_analyst` / `metrics_analyst` / `change_analyst` 中的一个，委派 handler 会为这次调用新建一个执行器：分析师的提示词、`analyst` 模型槽位、这一角色的工具白名单、这一角色的出口。它跑完自己的循环，把 finding 作为这次工具调用的返回值交回指挥官；两边各留一份记录，角色页才能把委派画成边。
3. **结论是类型化的。** 分析师只能用自己那张表收口（`triage_result` / `log_finding` / `metric_finding` / `change_finding`），指挥官只能用它这条链路的出口（`diagnosis` 或 `optimization_result`，二选一）。finding 的字段由代码校验：`query` 必须点名工具，`excerpt` 不能为空；至于是不是原文，代码验不了，只能靠提示词约束和事后审阅。不合格的那次调用被拒绝，模型重填。

分析师没有委派工具，也没有终端、文件、浏览器；它拿到的工具表就是它这一轮能看的全部证据。所以委派只向下分叉一层，指挥官始终是唯一看过所有 finding 的角色。

### 一次实跑的轨迹：flow 136

下面这张图按 flow 136 的角色页与交接文档重绘；原始页面见[界面](#界面)那张角色图（[明色](assets/screenshots/flow-roles-light.png) / [暗色](assets/screenshots/flow-roles-dark.png)）。flow 136 是 `scripts/scenarios/s5-scheduling-latency-trace.json` 的实跑，四个子任务全部收口：

```mermaid
flowchart TB
    G["generator<br/>1 chain：拆成 4 条假设"] --> C["commander<br/>4 chains：一条子任务一条"]
    R["refiner<br/>2 chains：按结果重规划"] --> C
    C -->|委派| T["triage"]
    C -->|委派| L["log_analyst<br/>2 chains / 132.3K tok"]
    C -->|委派| M["metrics_analyst"]
    C -->|委派| H["change_analyst"]
    T -.->|triage_result| C
    L -.->|log_finding| C
    M -.->|metric_finding| C
    H -.->|change_finding| C
    C --> D["diagnosis ×4<br/>4/4 accepted"]
```

实线是计划 / 委派，虚线是 finding 回流。`reporter` 与 `summarizer` 不由指挥官调用，在下面的表里单列。

| 角色 | 在 flow 136 里做了什么 | 记录 |
|---|---|---|
| `generator` | 把 CPU 调度延迟拆成四条可验证假设 | 1 条 chain |
| `refiner` | 按已完成的结果调整剩余计划 | 2 条 chain |
| `commander` | 四条子任务各跑一条 chain，四次 `diagnosis` 全部被接受 | 4 条 chain / 1.3M token |
| `triage` / `log_analyst` / `metrics_analyst` / `change_analyst` | 被委派取证，用各自的 finding 交回；日志分析员跑了两条 chain | 四个分析师都有委派边；`log_analyst` 2 条 chain / 132.3K token |
| `summarizer` | 压缩长链上下文 | 25 条 chain |
| `reporter` | 任务结束后写最终报告 | 1 条 chain |

整条 flow 21 次工具调用，其中 `ops_trace` 7 次、`diagnosis` 4 次（四次都成功）。结论没有迎合报障假设：CPU 排队 p99 只有 756.0µs，毫秒级尖峰出现在块设备 IO 侧（p99 12.00ms），所以报告把根因标成"未闭合"，置信度 0.60，`needs_human=true`。

角色页上的 `ch` 是 chain 数，边是该角色在本 flow 里实际发生的委派；节点上还带日志条数、token 用量与最后活动时间，一分钟内有动作的角色描绿边。

### 另一种轨迹：finding 回到指挥官

flow 94 是另一条常见的形状：指挥官派了两次活，`log_analyst` 与 `metrics_analyst` 各回一张 finding，两张都被接收；`diagnosis` 的 `evidence` 直接引用前者给出的 `query`。分析师说"取不到"时，finding 里也写明取不到——日志面写"真实首发取不到，日志缓冲已滚动"，指标面写"这条指标在这个后端不存在"——而不是编一个值。

这就是多角色协作的验收标准：计划能被逐条核对，委派能追到具体角色，finding 能追到具体工具，结论能追到 finding 的 `query`。

## 角色

十二个角色，全部来自 `MsgchainType` 枚举（`backend/pkg/database/models.go`），GraphQL 侧镜像为 `AgentType`：

| 角色 | 中文名 | 职责 | 正常出口 |
|---|---|---|---|
| `commander` | 事故指挥 | 执行计划、委派分析师、合并证据、收尾 | `diagnosis` / `optimization_result` |
| `generator` | 诊断规划器 | 把目标拆成可证伪假设 | `subtask_list` |
| `refiner` | 计划修订器 | 按已完成的结果调整剩余计划 | `subtask_patch` |
| `triage` | 分诊员 | 分级、影响面、是否命中已知故障模式 | `triage_result` |
| `log_analyst` | 日志分析员 | 日志面：错误模板、首次出现、实例分布 | `log_finding` |
| `metrics_analyst` | 指标分析员 | 指标面：拐点、先后顺序、未动的序列 | `metric_finding` |
| `change_analyst` | 变更分析员 | 变更面：变更时间线、与起始点的间隔 | `change_finding` |
| `reporter` | 事故报告员 | 把任务结果写成报告 | `report_result` |
| `reflector` | 反思员 | 工具调用接近上限时给出收尾建议 | — |
| `summarizer` | 摘要员 | 压缩长链上下文 | — |
| `tool_call_fixer` | 工具调用修复 | 修复格式错误的工具调用参数 | — |
| `assistant` | 助手 | 挂在 flow 上的交互式助手，共享容器与 provider 设置；不参与诊断计划 | — |

分析师只拿一个证据面，工具表是硬边界：

| 分析师 | 能用的取证工具 |
|---|---|
| `triage` | `ops_containers`、`ops_system`、`ops_logs`、`ops_exec`、`k8s_get` |
| `log_analyst` | `ops_logs`、`ops_containers`、`k8s_get` |
| `metrics_analyst` | `ops_system`、`ops_containers`、`prom_query`、`k8s_get` |
| `change_analyst` | `ops_containers`、`ops_system`、`ops_exec`、`k8s_get` |

表里的可选工具还要后端可用才出现：没配集群就没有 `k8s_get`，没配 Prometheus 就没有 `prom_query`。

指挥官的工具面更宽：`ops_watch`、`ops_sandbox`、`ops_trace`、`ops_baseline` / `ops_diff`、四个委派工具和出口都在它手里；分析师没有这些。

## 工具

### 只读取证（`OPSAGI_TARGETS` 里的主机，走 SSH）

| 工具 | 做什么 |
|---|---|
| `ops_system` | 主机概况：运行时长、负载、CPU、内存与 Swap、文件系统、CPU 占用最高的进程 |
| `ops_containers` | 容器清单：状态、重启次数、退出码、镜像，以及 failed 的 systemd 单元 |
| `ops_logs` | 日志尾部：按容器名读 `docker logs`，或读最近 30 分钟的系统错误日志 |
| `ops_baseline` | 采一次带时间戳的指纹，性能类目标的"改前"；可选带上容器与数据库的锁等待指标 |
| `ops_diff` | 对比两次指纹，逐项给变化量、百分比与"改善 / 恶化 / 持平"判定 |
| `ops_exec` | 按 ID 执行一条预置的只读诊断命令，覆盖其他工具没到的角度 |
| `ops_watch` | 把一条只读命令在时间窗内重复采样，抓"正在发生"的东西 |

`ops_exec` 的命令目录写死在代码里：监听端口、已建立连接、容量、inode、内存、负载、PSI、Top 进程、systemd 单元状态与日志、内核日志、容器 inspect / stats、文件元数据、软件包安装历史、镜像层历史、Postgres 的 `pg_stat_activity` / `pg_stat_statements` / `pg_locks` / `pg_lock_wait_summary` / `pg_explain`、MySQL 的连接与进程清单。模型只能选 ID 和参数，参数只允许字母、数字与 `._-/`，选不了命令文本。

### 自由命令，边界在容器形状

- `ops_sandbox` — 在目标主机上起一次性容器执行脚本。`probe`（默认：`--network=none --read-only --memory=256m`）与 `workshop`（可写层、可出网、512MB，用来装工具；另外加回 `SETUID` / `SETGID`，否则包管理器取包时 `setgroups` 失败）。两者都 `--cap-drop=ALL --security-opt=no-new-privileges --pids-limit=256 --cpus=1`，宿主机的 `/proc`、`/sys` 只读挂在 `/host/proc`、`/host/sys`。脚本从 stdin 进来，不拼进启动 docker 的那条 shell。开了租约时默认策略只放行 `probe`，`workshop` 要策略文件里显式加上。

### 内核层（`OPSAGI_SANDBOX_BPF` 打开时）

- `ops_trace` — 五个固定程序（`runqlat` / `offcputime` / `biolatency` / `execsnoop` / `tcpconnlat`），程序正文在代码里，模型只选 ID 与采样窗口。容器丢光 capability 后加回 `BPF`、`PERFMON`、`SYS_RESOURCE`，`/sys/kernel/debug`、`/sys/kernel/tracing`、`/sys/kernel/btf` 只读挂载，不进宿主机 PID 命名空间；默认无网络，操作员放开 `OPSAGI_SANDBOX_NETWORK` 时才改成 bridge（用于拉取带 `bpftrace` 的镜像）；老内核因为缺 `SYS_ADMIN` 拒绝 `perf_event_open` 时，自动补这一个能力重试一次——第一次永远是最窄的形状。

### 旁路（直接问监控系统，不经过主机 shell）

- `prom_query`（`PROMETHEUS_URL`）、`k8s_get`（`K8S_KUBECONFIG` 或 `K8S_API_URL`）。

### 写入（唯一的写通道）

- `ops_apply` — 按 ID 执行一条预置修复动作：`unit_restart`、`container_restart`、`config_restore`、`disk_cleanup`、`k8s_rollout_restart`。命令文本来自代码目录，参数只允许字母、数字与 `._-/`，路径参数限制在模板声明的目录之下；租约里没有这条动作时只提交审批，不动手。

### 记录与出口

- `record_finding`（交互式助手）把一条结论写进事故记录（标题、含义、原始输出）——聊天记录会滚走，它不会。
- 诊断链的出口是 `diagnosis`、`optimization_result`、`triage_result`、`log_finding`、`metric_finding`、`change_finding`。

## 它不做什么

- **不做自由写入。** 唯一的写通道是 `ops_apply`，它只能执行代码里的修复动作目录（重启单元、重启容器、用快照覆盖配置、清理过期文件、重启 k8s 工作负载），不接自由命令；默认还要一道审批，租约里没有这条动作就只提交审批、不动手。修复之后必须重新采集、再与变更前的基线对比，没带复测的改善判定过不了校验。
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

四道闸门叠起来，任何一道都能独立拦住一次调用：

1. **角色工具表**（代码）。每个角色能看见哪些工具写在 `pkg/tools` 里，表外的东西模型根本选不到。分析师的表只有取证工具、可选的 `ask` 和自己的出口，没有终端、文件、浏览器，也没有另一个分析师。
2. **固定目录与固定形状。** `ops_exec` 只能从只读命令目录里挑 ID 与参数，不接自由命令；`ops_sandbox` 的两种形状都把宿主机 `/proc`、`/sys` 只读挂载，`--cap-drop=ALL`，不进宿主机 PID 命名空间——能读到宿主机的一切，却改不了它。
3. **能力租约**（`backend/pkg/capability`）。每条 flow 创建时选"关闭 / 观察 / 强制"。`shadow` 只记录"这条会被拒绝"，`enforce` 才真的拒绝；判定逐条写入 `capability_decisions`，切面在工具表最外层包住 handler；租约按子任务签发、落库、重启后恢复，不会重新放宽。默认策略贴近租约出现之前的语义：放开所有已配置主机、角色自己的工具、整份只读目录，只放行 `probe` 形状，不预置任何写动作。所以打开 `enforce` 通常不会让一条只读诊断突然失败；要用 `workshop` 或写动作，需要在策略文件里显式放开。租约外的调用如果落在可批准的一维（主机、命令、形状、eBPF 程序、写操作实例），会提交一条 `capability_requests`；操作员批准后写一条 4 小时有效的 `capability_grants`，下一次重试就通过，不用重启子任务。
4. **部署侧策略。** 能连哪台主机、能不能出网、能不能拉镜像、开不开 eBPF，都由操作员配置，不写在提示词里。

### 部署侧开关

| 变量 | 默认 | 作用 |
|---|---|---|
| `OPSAGI_TARGETS` | 空 | 目标主机清单（JSON 数组：`id` / `host` / `port` / `user` / `password` 或 `key_file`，也可以只写 `credential_id`；可加 `host_key_fingerprint` 钉住主机密钥）。空则所有 `ops_*` 工具不出现 |
| `OPSAGI_POLICY` | 空 | 能力租约策略文件（YAML，示例见 `docs/aiops/policy.example.yml`）；不配则用内置默认策略 |
| `OPSAGI_SECRET_KEY` | 空 | 凭据主密钥（32 字节，hex 或 base64）；空则凭据接口返回 503 |
| `OPSAGI_DEFAULT_SANDBOX_IMAGE` | 空 | 流程自带终端 / 工具容器的镜像；空则回落到 `DOCKER_DEFAULT_IMAGE_FOR_PENTEST` 或内置镜像 |
| `PROMETHEUS_URL` | 空 | 配了才有 `prom_query` |
| `K8S_KUBECONFIG` | 空 | 配了才有 `k8s_get` |
| `K8S_API_URL` | 空 | 直接给集群 API 地址，与 kubeconfig 二选一 |
| `K8S_INSECURE_SKIP_TLS_VERIFY` | `false` | 跳过集群证书校验 |
| `OPSAGI_SANDBOX_NETWORK` | `false` | 允许采集容器出网（`workshop` 形状的前提） |
| `OPSAGI_SANDBOX_PULL` | `false` | 允许拉取镜像 |
| `OPSAGI_SANDBOX_BPF` | `false` | 打开 `ops_trace`；关着时工具不存在 |
| `OPSAGI_BPF_IMAGE` | `quay.io/iovisor/bpftrace:latest` | 追踪容器镜像，里面要有 `bpftrace` |
| `ASK_USER` | `false` | 打开 `ask` 工具（人工闸门） |
| `UNATTENDED` | `false` | 无人值守：`ask` 自答，不阻塞 chain |
| `OPSAGI_SUBTASK_CONCURRENCY` | `1` | 同一任务内子任务并发度 |

没配的东西不会出现在工具表里，而不是出现了再报错。

**凭据**（设置 → 凭据）是另一条线：目标主机可以不在 `OPSAGI_TARGETS` 里写密码，而是写 `credential_id` 引用库里的一条凭据。凭据用 AES-256-GCM 密封后存库，密钥只在启动时从 `OPSAGI_SECRET_KEY` 读入内存、从不落库，所以只备份数据库并不能恢复凭据；接口永远只回元数据，明文仅在建立 SSH 连接的那一刻解开。凭据是全局的——所有 flow 共用同一份，增删与轮换都在界面上完成。

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

一个 flow 在桌面上分成左右两栏：左边是自动化对话、助手对话与概览，右边是可切换的观察面——终端、任务、智能体、角色、工具调用、向量库、文件、截图。窄屏下这些页签合并成一条。

**角色**页回答"这一轮是谁在干活"。上方是按层摆放的十二个角色拓扑——规划（生成 / 精炼）在上，指挥官居中，四个取证分析师在下，消费与修补型角色在底；节点上带该角色在本 flow 的 chain 数、日志条数、token 用量与最后活动时间，一分钟内有动作的角色描一圈绿边，发生过的委派用带次数的箭头连起来。下方是按角色过滤的日志：点节点或点角色标签，只看这个角色写下或应答的条目。图用 [`@xyflow/react`](https://reactflow.dev) 画，数据来自 `msgchains` 与 `agentlogs` 两条聚合。

**能力租约**。非 `off` 的会话在标题旁挂一个"租约·观察 / 强制"徽章；菜单里的"能力租约"抽屉列出每条判定，可按"拦截 / 将被拦截 / 放行"筛，`would_deny` 逐条给出拒绝原因——这就是把 `shadow` 收窄成 `enforce` 的依据。抽屉里还有待批准请求、已签发的租约与额度。关闭租约时会话不签发、不检查、不记录。

## 快速开始

OpsAGI 需要两样东西：一个带 pgvector 的 PostgreSQL，和一个 OpenAI 兼容的模型端点。要取证，还要有 `OPSAGI_TARGETS` 里的目标主机；要跑流程自带的终端容器，还要 Docker。

### 方式一：Docker Compose（含界面）

```bash
cp .env.example .env
# 至少填一个模型提供商的 key，例如 OPEN_AI_KEY=sk-...
# 把 COOKIE_SIGNING_SALT 从默认的 salt 改成随机值（默认的 salt 会禁用 API token 创建）
docker build -t local/opsagi:latest .
docker compose up -d
```

打开 `https://localhost:8443`，用 `admin@opsagi.com` / `admin` 登录，首次登录会要求改密码。数据库、pgvector、scraper 等由 compose 提供；`docker-compose-observability.yml`、`docker-compose-langfuse.yml`、`docker-compose-graphiti.yml` 分别加上监控、LLM 分析与知识图谱。

### 方式二：从源码运行后端

```bash
cd backend
go generate ./cmd/installer/files/   # 首次克隆，或改动 cmd/installer/files 之后
go build -trimpath -o opsagi ./cmd/opsagi

export DATABASE_URL='postgres://opsagi:opsagi@localhost:5432/opsagi?sslmode=disable'
export COOKIE_SIGNING_SALT="$(openssl rand -hex 16)"
export LLM_SERVER_URL='http://127.0.0.1:8000/v1'
export LLM_SERVER_KEY='sk-...'
export LLM_SERVER_MODEL='your-model'
export OPSAGI_TARGETS='[{"id":"lab","host":"10.0.0.5","port":22,"user":"root","password":"..."}]'

./opsagi
```

API 与 GraphQL 在 `http://localhost:8080`（`SERVER_PORT` 可改）；把前端构建产物交给 `STATIC_DIR` 就能在同一端口打开界面（例如先在 `frontend/` 跑 `pnpm run build`，再用 `STATIC_DIR=../frontend/dist ./opsagi` 启动）。数据库初始化后自带 `admin@opsagi.com` / `admin`，首次登录要改密码；也可以建一个自己的管理员：

```bash
go run ./cmd/opsagi-seed -mail ops@example.com -name "值班" -password-stdin
```

然后在界面上新建会话，或直接用命令行跑一次并取回结论：

```bash
export OPSAGI_URL='http://127.0.0.1:8080'
export OPSAGI_MAIL='ops@example.com'
export OPSAGI_PASSWORD='...'
python scripts/opsagi_run.py "lab 上的 WordPress 站点 500，帮我定位根因"
```

### 验收自检

不需要真实主机和真实模型也能跑：起一个假模型、起服务、建管理员，断言一条流程以合法的诊断收尾、另一条以带判定的性能结论收尾：

```bash
export DATABASE_URL='postgres://...'
python scripts/aiops_acceptance/run.py
```

需要 Go（构建服务与 seed）、Node（假模型）、一个带 pgvector 的 PostgreSQL（`DATABASE_URL`）和 Docker（流程自带的终端容器）。首次要等服务与容器起来，通常几分钟内完成；任何一步不成立都会非零退出。

## 验证记录

[`scripts/scenarios/`](scripts/scenarios/) 里有八个场景，都是这套内核被真模型、真主机跑过之后留下来的：靶场要摆成什么样、对产品说哪一句话、什么算答对、什么算糊弄，各写在一份 JSON 里。

| 场景 | 验的是 | 实跑记录 |
|---|---|---|
| `s1-dependency-container-exit` | 健康检查绿着、接口 500 时，根因能不能落到依赖容器的退出上 | flow 125 |
| `s2-slow-query-missing-index` | 性能结论里的毫秒数是不是量出来的，没达目标敢不敢填 `target_met=false` | flow 126 |
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

真模型、真靶场下留下的记录：

- **故障定位**：容器依赖退出（健康检查仍在 200）、真实行锁等待、sshd 日志归档回放、eBPF 排队延迟（结论否定了报障假设）。
- **性能优化**：CPU 饱和（load1 1.98 → 0.41，`improved`）、慢查询（mean 167.29ms → 0.22ms，flow 60 自己收口）；没有复测条件时如实给出 `unverified`，测过没变化时 `no_measurable_effect` 是合法结论。
- **纯日志**：六个 Loghub 真实样本（HDFS / Linux 认证 / Apache / Spark / Zookeeper / OpenStack），含"全 INFO 应当答健康"的对照组；证据不足的样本被如实标成 `failed` / `waiting`，而不是硬下结论。
- **界面**：容器终端实时回显、角色拓扑、会话结论渲染、租约判定面板。

尚未系统验证：多目标主机并发、长时间运行下的资源泄漏、非 Debian 系的宿主机取证、k8s 崩溃循环场景、Prometheus 旁路在大规模指标下的表现。

## 开发

**后端**（`backend/`，Go 1.26.5）：

```bash
go generate ./cmd/installer/files/          # 首次克隆，或改动 cmd/installer/files 之后
go build -trimpath -o opsagi ./cmd/opsagi
go test ./...
golangci-lint run --timeout=5m
```

`go test ./...` 不包含 `postgres` 构建标签下的那一层：

```bash
OPSAGI_TEST_DSN='postgres://…' go test -p 1 -tags postgres ./pkg/database/... ./pkg/timezone/... ./migrations/
```

**前端**（`frontend/`，Node 24.17 + pnpm 11.8）：

```bash
nvm use
corepack enable
pnpm install
pnpm run dev        # http://localhost:8000
pnpm run build
pnpm run typescript
pnpm run test
pnpm run e2e        # Playwright，默认 mock tier
```

在 `frontend/.env` 里把 `VITE_API_URL` 指向后端（`127.0.0.1:8080`）。视觉快照只在 Playwright 容器里跑：`pnpm run e2e:visual`。

**代码生成。** 改 `backend/pkg/graph/schema.graphqls` 后要跑 gqlgen、`swag init` 与 `pnpm run graphql:generate`；改 `backend/sqlc/models/*.sql` 或迁移后要跑 `sqlc generate`（需要一个真实 PostgreSQL）。生成物随改动一起提交。

## 目录

```
backend/                 Go 服务：REST + GraphQL
  cmd/opsagi/            服务入口
  cmd/opsagi-seed/       管理员与密码种子
  cmd/opsagi-probe/      只读取证工具的命令行验证器
  pkg/controller/        flow / task / subtask 运行时
  pkg/capability/        能力租约：判定、策略、策略文件
  pkg/credentials/       凭据的 AES-256-GCM 密封与存储
  pkg/tools/             工具表与执行器；ops.go / aiops.go 是取证与出口层
  pkg/opsremote/         只读命令目录、SSH 执行、采集容器与 eBPF 程序
  pkg/templates/prompts/ 角色提示词（中文）
frontend/                React + TypeScript 界面
docs/aiops/              设计、交接、验收记录
docs/platform-reference.md  提供商与模型目录、部署与配置细节
scripts/                 命令行工具与批测脚本
scripts/scenarios/       实验场景：靶场怎么摆、问什么、什么算答对
observability/           可选监控栈配置
```

## 文档

- [总览](docs/aiops/00-overview.md) — 目标、分层与阶段
- [交接文档](docs/aiops/03-handover.md) — 当前状态、验证到什么程度、哪里还没验证（另有 [.docx](docs/aiops/03-handover.docx)）
- [能力租约](docs/aiops/04-capability-plan.md) — 判定点、审计、批准通道、策略文件与预算
- [平台参考](docs/platform-reference.md) — 提供商与模型目录、部署与配置细节
- [实验场景](scripts/scenarios/README.md) — 场景字段、判卷口径与新增场景的写法
- [前端测试手册](frontend/docs/e2e.md) — e2e 三层、cassette 与常见陷阱

## 许可

代码遵循 MIT（见 [LICENSE](LICENSE)）。[EULA](EULA.md) 与 [NOTICE](NOTICE) 说明了随仓库分发的许可条款与第三方组件。
