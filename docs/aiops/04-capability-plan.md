# 从角色白名单到任务级能力租约

> 读者是维护 OpsAGI 运行时的工程师。下面每个结论都能在文末的代码索引里找到落点；文中的“今天”指当前 `main`。

## 0. 结论

今天的权限是四道闸门叠出来的：角色工具表、只读命令目录、采集容器形状、环境变量。其中只有第一道是**按角色**不同的；“能连哪台主机、能跑哪条命令、能用哪种容器”对所有角色一视同仁，而且形状由模型自己挑。

所以越权的可能不在提示词里，而在三个参数上：`target`（模型填）、`ops_exec.command`（模型从全目录里挑）、`ops_sandbox.mode`（模型选 probe 还是 workshop）。提示词写“只查 lab”不会有任何强制力。

下一步不是再写一版提示词，而是把这四道闸门收进一个**任务级能力租约**：由结构化计划派生、由代码签发、在每次工具调用前强制、随子任务结束回收、可审计也能在重启后恢复。分四步走，先观测再强制，最后把强制点下沉到 SSH 与目标机边界。

## 1. 今天实际生效的四道闸门

| 闸门 | 代码 | 它拦住了什么 | 它没拦住什么 |
|---|---|---|---|
| 角色工具表 | `tools.go:256`（`analystToolset`）、`tools.go:903`（commander 硬编码） | 某角色根本拿不到某工具 | 没有任何“这个子任务只许查这台主机”的收窄；分析师拿到 `ops_exec` 就是全目录；同一次 flow 里 commander 与 change_analyst 的命令面一样宽 |
| 目标主机清单 | `ops.go:126`（`resolve`） | 连不到 `OPSAGI_TARGETS` 之外的地址 | 任何拿到 `ops_*` 的子任务都能打任意一台已配置主机；`target` 留空等于第一台；`OPSAGI_TARGETS` 解析失败会**静默**变空表（`ops.go:99`） |
| 只读命令目录 | `opsremote/exec.go:40`、`:77` | 只能跑 24 条写死在代码里的只读命令，参数按字符集校验 | 目录不按角色或任务切分；参数是正则字符集不是枚举值；`mysql_connections` / `mysql_processlist` 可调用但没进 `catalogOrder`（`:199`、`:205`），公告目录与真实目录已经漂移 |
| 采集容器形状 | `opsremote/sandbox.go:37`（probe）、`:50`（workshop）、`trace.go:14` | probe 只读、无网、256MB；宿主机 `/proc`、`/sys` 只读挂载 | 形状是模型选的（`ops.go:300`）；workshop 只要操作员放开出网就是可写 + 出网 + `SETUID/SETGID`；`ops_trace` 的 `SYS_ADMIN` 由目标机 stderr 里的两个子串触发（`trace.go:159`），越权升级的开关握在被检查的机器手里 |

这四道闸门之外，还有两处“看起来像权限、其实只是提示”：

- `Refusal`（`refusal.go:12`）把策略拒绝当正常结果回给模型，同一个调用可以立刻重试；没有任何计数、期限或审计维度。
- 工具表只在执行器构造时算一次（`provider.go:930`），子任务跑起来之后没有“这一轮还能不能再扩一点”的概念。

## 2. 目标模型：Capability Lease

一个租约 = 某个子任务在一段时间内、对某些资源、可以做的某几件事。

```go
type Lease struct {
    ID           int64
    FlowID       int64
    SubtaskID    int64
    Role         string   // commander / triage / log_analyst / metrics_analyst / change_analyst
    Mode         string   // diagnosis / optimization
    Targets      []string // OPSAGI_TARGETS 的子集
    Tools        []string // 工具名子集
    Commands     []string // opsremote 目录的子集
    Shapes       []string // probe / workshop
    TracePrograms []string
    MaxCalls     map[string]int // 每工具预算
    IssuedBy     string         // "policy" 或操作员 ID
    ExpiresAt    time.Time
    RevokedAt    *time.Time
}
```

三条不变量，缺一条这套东西就会退化回提示词：

1. **缺省拒绝。** 没有租约就是没有工具；租约过期、解析失败、策略缺失，一律拒绝，不允许“配置读不到就全放开”。今天 `OPSAGI_TARGETS` 的坏 JSON 静默清空就是反例。
2. **只能从结构化计划派生。** 计划的 `mode`、`assigned_to`、`expected_signal` 已经是机器可读字段（`args.go:56`），租约从这些字段和操作员策略表里算出来；**不从自然语言描述里解析权限**，也不接受模型在 `message` 里“申请”的权限。
3. **强制点在 `Execute`。** 工具表只决定“能不能出现在模型眼前”，真正的判定放在 `customExecutor.Execute`（`executor.go:359`，`handlers[name]` 查表与 `wrapHandler` 之间），因为那是每个角色、每次调用都必须经过的唯一漏斗，而且它已经握着 `flowID`、`taskID`、`subtaskID`。

和现有代码的对应关系：

- `analystToolset` 从“权限表”降级为“策略表里的默认行”，租约可以比它更窄，不能更宽。
- 一个出口的约束（`removeDefinition`，`tools.go:966`）保留，租约只管入口不管出口。
- `Refusal` 继续当拒绝通道，但它要多带三样东西：租约 ID、拒绝原因码、可审计的记录。
- 容器形状、命令目录、eBPF 程序集都变成租约字段的投影，而不是模型的选项。

## 3. 分阶段落地

### P0 观测与止血（3–5 人日）

先不拦，只记录“本该拦”。这样能在收紧之前知道收窄到什么程度不会把系统卡死。

- 新开关 `OPSAGI_LEASE_MODE=off|shadow|enforce`，默认 `off`；`shadow` 下按计划与策略表算出应当允许的集合，与真实调用比对，把 `would_deny` 写进新表 `lease_decisions`（或 `toolcalls` 的附加列），**不改变行为**。
- 修三个 fail-open：
  - `OPSAGI_TARGETS` 解析失败在启动时报错退出，不再静默变空表（`ops.go:99`）。
  - `mysql_connections`、`mysql_processlist` 补进 `catalogOrder`，并让守卫测试遍历 `catalog` 而不是 `Commands()`（`registry_test.go:265`）。
  - `ops_sandbox` 的非法 `mode` 在 dial 之前就拒绝（今天会先在目标机上跑一次 docker）。
- 交付一份一天的 shadow 数据：`would_deny` 按原因分组（主机越界 / 命令越界 / 形状越界 / 预算超限），这张表决定 P1 的默认策略。

验收：shadow 模式下行为与今天逐字节一致，只有 `lease_decisions` 多出记录。

### P1 租约内核（8–10 人日）

- 新包 `backend/pkg/capability`：`Lease`、`Policy`、`Issue(plan, flow, policy) (Lease, error)`、`Authorize(lease, call) error`、`Revoke`。
- 新迁移 `capability_leases`（见第 4 节），sqlc 再生成。
- 签发点两处：
  - `createPlannedSubtask`（`controller/subtasks.go:106`）写完 `plan_mode` 同期写租约；
  - `PerformAgentChain`（`provider.go:740` 之后、`:892` 解析出 mode 的地方）在缺少租约时按策略补发一条，保证老数据也能跑。
  - 重规划删除 `created` 行时（`subtasks.go:162`），撤销对应租约。
- 强制点一处：`customExecutor.Execute` 在 `handlers[name]` 之后、写消息日志之前调用 `Authorize`；拒绝走 `Refuse(...)`，文案说清“哪条策略、怎么申请”。
- 注入点：`GetCommanderExecutor`（`tools.go:903`）与 `GetAnalystExecutor`（`tools.go:1014`）都把 lease 放进 `customExecutor`；分析师链路是独立执行器（`aiops_handlers.go:129`），漏了它 commander 的检查就形同虚设。
- 恢复：`LoadSubtaskWorker` 把 `running` 改回 `created` 时（`subtask.go:95`），从表里重读租约；已过期就按失败处理并说明原因，不静默续期。
- 默认策略 = 今天的语义（全部主机、全目录、probe 优先），只有 `OPSAGI_LEASE_MODE=enforce` 才按策略表收窄，保证可以先上线再收紧。

验收：`enforce` 下，计划只写了 `lab` 的子任务对 `prod` 调 `ops_exec` 得到拒绝、没有 SSH 握手、`lease_decisions` 记一条 denied；把拒绝那一行删掉，测试必须红。

### P2 形状与内核层（4–5 人日）

- `lease.Shapes` 决定 probe / workshop / trace，`ops_sandbox.mode` 从“模型选择”变成“租约允许的集合里挑”；不在集合里直接拒绝。
- `OpsSandboxImages` 现在是死字段（`config.go:155` 只有注释），要么真正启用成镜像白名单，要么删掉。
- `ops_trace` 的 `SYS_ADMIN` 重试改成租约里的能力阶梯：第一次必须最窄；重试要么已经写在租约里，要么走 P3 的批准通道。目标机 stderr 不再能单方面把容器抬权。
- 顺带把 flow 容器补上资源上限：今天 `WorkerSpec` 没有内存/CPU 限制、镜像默认用户是 root（`docker/worker_spec.go:18`、`docker/client.go:376`）。租约带 `memory`/`cpus`/`pids` 时为子任务单独建容器是可选项，先做限值。

验收：租约只允许 `probe` 时，模型请求 `workshop` 得到拒绝且目标机上没有 `docker run`；`OPSAGI_SANDBOX_BPF=false` 与租约不含 trace 时，`ops_trace` 都不在工具表里。

### P3 升级闸门（4–5 人日）

- 拒绝不是终点：`Authorize` 返回“需要批准”时，把子任务置为 `waiting`（复用 `ask` 的链路，`provider.go:867`），操作员在界面上批准后签发一条补充租约（新行、`IssuedBy=操作员`、带上限），再回到 `running`。
- 批准界面：角色页显示当前租约与待批请求（两个 GraphQL 查询 + 一个 mutation），复用现有 `flows.edit` 权限。
- 每次批准、续期、拒绝都写 `lease_decisions`，让“谁放开了什么”能倒查。

验收：workshop 请求在没有批准时永远拿不到；批准一次后只有这一个子任务拿到，批准过期后再次请求又回到 waiting。

### P4 下沉到 OS 边界（15–20 人日，单独立项）

前三步的强制都在应用层：绕过应用就等于绕过租约。真要让越权“在目标机上不成立”，得把限制搬到凭据和命令两处：

- **短期凭据**：由 SSH CA 为每个租约签一次证书，`permitopen` 限定端口、`command=` 限定入口；租约回收即吊销。比共享 `OPSAGI_TARGETS` 里的长期密码/私钥强一个数量级。
- **目标机侧代理**：部署一个 `opsagi-probe` 受限入口，只接受租约里的命令 ID，参数按同一套 `ParamKind` 校验；这样即使 agent 侧被绕过，目标机也只认那几条命令。
- 只读账号 + `sudoers` 白名单，替代现在的 root/密码模式。

这一步依赖 CA 或代理的部署，收益是“越权在目标机上不成立”，风险是运维复杂度；建议等 P1–P3 上线、shadow 数据显示收紧到什么程度之后再立项。

## 4. 数据模型与配置草案

```sql
CREATE TABLE capability_leases (
    id              BIGSERIAL PRIMARY KEY,
    flow_id         BIGINT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
    subtask_id      BIGINT REFERENCES subtasks(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    mode            TEXT NOT NULL CHECK (mode IN ('diagnosis', 'optimization')),
    targets         TEXT[] NOT NULL DEFAULT '{}',
    tools           TEXT[] NOT NULL DEFAULT '{}',
    commands        TEXT[] NOT NULL DEFAULT '{}',
    shapes          TEXT[] NOT NULL DEFAULT '{}',
    trace_programs  TEXT[] NOT NULL DEFAULT '{}',
    max_calls       JSONB NOT NULL DEFAULT '{}',
    issued_by       TEXT NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX capability_leases_subtask_idx ON capability_leases (subtask_id) WHERE revoked_at IS NULL;
```

策略文件（`OPSAGI_POLICY`，YAML，按 flow_type 给默认行；缺省值就是今天的语义）：

```yaml
modes:
  diagnosis:
    targets: ["*"]
    tools: [ops_system, ops_containers, ops_logs, ops_baseline, ops_diff, ops_exec, ops_watch, prom_query, k8s_get, diagnosis]
    commands: ["*"]
    shapes: [probe]
  optimization:
    targets: ["*"]
    tools: [ops_system, ops_containers, ops_exec, ops_watch, prom_query, k8s_get, optimization_result]
    commands: [top_cpu, top_mem, psi, load, memory, unit_logs, docker_stats]
    shapes: [probe]
approval:
  workshop: operator
  trace: operator
limits:
  default_ttl: 30m
  max_calls_per_tool: 40
```

## 5. 验收场景

| 场景 | 今天 | P1 之后 |
|---|---|---|
| 计划只允许 `lab`，模型对 `prod` 调 `ops_exec` | 连得上，命令真跑（只要 `prod` 在 `OPSAGI_TARGETS` 里） | 拒绝，无 SSH 握手，审计一条 denied |
| 分析师调用目录外的命令 | 不可能（目录本身封闭），但可以调全部 24 条 | 只能调租约里那几条，越界即拒绝 |
| 模型请求 `workshop` | 操作员放开 `OPSAGI_SANDBOX_NETWORK` 就能拿到 | 需要租约允许 + 操作员批准，按次授权 |
| 重启后继续跑一个进行中的子任务 | 按 `created` 重跑，权限与之前相同（全量） | 按表里租约重跑；已过期则失败并说明 |
| 操作员想知道“这条命令谁批的” | 查日志 | `lease_decisions` 直接给租约 ID 与批准人 |

## 6. 风险与边界

- **不做自然语言授权解析。** 模型的“申请”只作为待批请求的说明文字，权限永远由结构化字段与策略表算出来。
- **租约过窄会让模型反复试错。** 用 P0 的 shadow 数据定默认值，并让拒绝文案直接写“怎么申请”，而不是让模型猜。
- **恢复语义必须 fail-closed。** 重启后租约过期就失败，不续期；续期只发生在 P3 的批准通道里。
- **P4 不是本轮目标。** 应用层强制能挡住“模型选错工具”，挡不住“进程被改过”；后者要等凭据与目标机代理就位。
- **工作量是估算。** P0–P3 合计约 20–25 人日，按一人全职、含测试与文档。

## 7. 代码索引

| 位置 | 内容 |
|---|---|
| `backend/pkg/tools/tools.go:256` | `analystToolset`，唯一的角色→工具表 |
| `backend/pkg/tools/tools.go:903` | `GetCommanderExecutor`，commander 工具硬编码 |
| `backend/pkg/tools/executor.go:359` | `customExecutor.Execute`，唯一调用漏斗 |
| `backend/pkg/tools/refusal.go:12` | `Refusal`，现有的策略拒绝通道 |
| `backend/pkg/tools/ops.go:126` | `resolve`，主机校验 |
| `backend/pkg/tools/ops.go:300` | `ops_sandbox.mode`，模型选形状 |
| `backend/pkg/opsremote/exec.go:77` | 只读命令目录 |
| `backend/pkg/opsremote/sandbox.go:37` | probe / workshop 参数 |
| `backend/pkg/opsremote/trace.go:159` | `SYS_ADMIN` 重试门 |
| `backend/pkg/providers/provider.go:740` | 每次执行重读 subtask 行 |
| `backend/pkg/providers/provider.go:892` | 解析 mode、装配出口 |
| `backend/pkg/providers/aiops_handlers.go:129` | 分析师链路独立执行器 |
| `backend/pkg/controller/subtasks.go:106` | `createPlannedSubtask`，计划落库处 |
| `backend/pkg/controller/subtask.go:95` | 重启时 `running` 改回 `created` |
| `backend/pkg/docker/worker_spec.go:18` | flow 容器规格（无内存/CPU 限制） |
| `backend/pkg/tools/args.go:56` | 计划的机器可读字段 |
