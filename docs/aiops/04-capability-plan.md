# 从角色白名单到任务级能力租约

> 读者是维护 OpsAGI 运行时的工程师。下面每个结论都能在文末的代码索引里找到落点；文中的“今天”指当前 `main`。

## 0. 结论

今天的权限是四道闸门叠出来的：角色工具表、只读命令目录、采集容器形状、环境变量。其中只有第一道是**按角色**不同的；“能连哪台主机、能跑哪条命令、能用哪种容器”对所有角色一视同仁，而且形状由模型自己挑。

所以越权的可能不在提示词里，而在三个参数上：`target`（模型填）、`ops_exec.command`（模型从全目录里挑）、`ops_sandbox.mode`（模型选 probe 还是 workshop）。提示词写“只查 lab”不会有任何强制力。

下一步不是再写一版提示词，而是把这四道闸门收进一个**任务级能力租约**：由结构化计划派生、由代码签发、在每次工具调用前强制、随子任务结束回收、可审计也能在重启后恢复。分四步走，先观测再强制，最后把强制点下沉到 SSH 与目标机边界。

## 0.5 这一轮已经落地的部分

开关的粒度是**每个 flow**，不是环境变量：新建 flow 时选“关闭 / 观察 / 强制”，值存在 `flows.capability_mode`（`off` 为默认，老 flow 全部落在 `off`）。改环境变量会一次性影响所有 flow，正好反过来：先在一条 flow 上试策略，再决定要不要默认打开。

| 层 | 位置 | 行为 |
|---|---|---|
| 开关 | `backend/migrations/sql/20261123120000_add_capability_leases.sql`、`pkg/controller/flow.go` | `off \| shadow \| enforce`，`CHECK` 兜底；未知值在写库前被 `capability.ParseMode` 归一到 `off` |
| 切面 | `pkg/tools/capability_guard.go` | `CapabilityGuard.Apply` 在 `GetCommanderExecutor` / `GetAnalystExecutor` 的最后一步包住 handler 表；`off` 时原样返回，不做任何包装，也不写一条记录 |
| 判定 | `pkg/capability` | `Lease.Authorize(call, now)` 依次判工具、主机、命令、形状、eBPF 程序；空列表不是通配，缺省拒绝；`*` 是唯一的“全部” |
| 签发 | `pkg/providers/capability.go` | 子任务第一次执行时按 `subtaskMode` 从策略里签发一条租约，写 `capability_leases`（`subtask_id` 唯一，重规划级联删除）；重启用同一行，不重新放宽 |
| 审计 | `capability_decisions` | 每条判定一条记录；`shadow` 下把 `deny` 存成 `would_deny`，`enforce` 下存 `deny` |
| 拒绝通道 | `pkg/tools/refusal.go` | 继续用 `Refusal`，模型看到的是“为什么被拒、怎么才能申请”，不是工具坏了 |

默认策略（`capability.DefaultPolicy`）刻意等于今天的语义：全部已配置主机、角色自己的全部工具、整个只读目录、`probe` 形状、8 小时有效期。打开 `enforce` 不会立刻改变一条正在工作的 flow 的行为——只有把策略改窄之后它才开始咬人。要把策略收紧，改 `DefaultPolicy` 一处即可，判定点不用动。

有一处取舍写在这里，因为它是刻意的：读不到 flow、读不到 subtask、签发失败时，切面**不安装**并打一条 `Warn`，而不是让整个子任务失败。租约是加法，数据库回答不了的时候不该把一次正常的诊断变成错误；代价是那一轮跑在无租约状态，所以这条日志必须是 Warn 级别，运维能看见。

还没做：`ops_tunnel` 之外的形状阶梯（`workshop` 仍由 `OPSAGI_SANDBOX_NETWORK` 与模型共同决定）、按角色的策略（今天策略只有 plan mode 两个维度，角色维度仍由工具表决定）。下面几节保留原计划与验收方式，作为后续的路线图。

## 0.6 这一轮的第二批：看得见，且不再静默

`shadow` 只有在数据能被读出来时才有意义，所以这一批先把“开关现在是什么、它判了什么”搬到界面上，再把 P0 的三处静默失败堵上。

| 面 | 位置 | 行为 |
|---|---|---|
| 读开关 | `pkg/graph/schema.graphqls` 的 `Flow.capabilityMode`、`pkg/database/converter` | flow 查询直接带回开关；库里出现未知值（老行、外部写入）时按 `off` 渲染，不让整条查询因为一个枚举值失败 |
| 读判定 | `query capabilityDecisions(flowId, verdict, limit)` | 只读一条 flow 的尾部：`limit` 缺省 200、上限 1000，`verdict` 为空表示不过滤；权限复用 `flows.view` 与已有的“flow 属于谁”校验 |
| 看界面 | `frontend/src/features/flows/capability/`、`pages/flows/flow.tsx` | 非 `off` 的 flow 在标题旁挂一个“租约·观察/强制”徽章；`能力租约` 面板列出每条判定，可按“拦截 / 将被拦截 / 放行”筛，`would_deny` 逐条给出拒绝原因——这就是把 `shadow` 收窄成 `enforce` 的依据 |
| 止血 | `pkg/config`、`pkg/opsremote/exec.go`、`pkg/opsremote/sandbox.go` | `OPSAGI_TARGETS` 非法或缺 `host` 时启动即失败；两条 `mysql_*` 命令补进公告目录，并加一条遍历 `catalog` 的守卫；非法 `mode` 在 dial 之前变成一条 `Refusal`，不再先去目标机拉镜像 |

面板本身只读判定；写操作在下一节。

## 0.7 这一轮的第三批：批准通道

`enforce` 只有在“被拒之后还能继续”时才可交付。一个模型被拒后只会重试，没有别的动作可做；操作员也没有按钮可点。这一批补上从拒绝到批准的那条线。

| 环节 | 位置 | 行为 |
|---|---|---|
| 归档 | `pkg/capability` 的 `Decision.NeedsApproval`、`pkg/tools/capability_guard.go` | 只有“能被一条批准解开”的拒绝才会归档：主机、命令、形状、eBPF 程序。工具不在角色表里、租约过期，都不归档——批准工具等于绕开角色工具表，批准过期等于取消有效期 |
| 请求 | `capability_requests` | 每次拦截写一条 `pending`，按 `(flow, tool, target, command, shape, trace_program)` 去重，模型连续重试不会堆一屏副本；拒绝语句里带上请求号（`已提交审批 #42`），模型和操作员说的是同一个东西 |
| 批准 | `mutation decideCapabilityRequest(requestId, approve)` | 批准写一条 `capability_grants`（有效期 4 小时），拒绝只改状态；两者都只对 `pending` 生效，重复点击返回“已不再是待批准状态”，不会写出第二条租约 |
| 生效 | `pkg/providers/capability.go` 的 `capabilityGrants` | 批准**不重启子任务**：切面每次调用都重读一遍这条 flow 的有效批准，所以模型的下一次重试就通过了。代价是每次工具调用多一次小查询，换来的是“批准即时生效”而不是“下条子任务才生效” |
| 边界 | 同上 | 批准是**加宽到具体调用**，不是放宽租约：`Grant.Permits` 要求工具与它写的每一维都相等，空维度不是通配；租约过期仍然硬拒，批准不能续命 |
| 界面 | `frontend/src/features/flows/capability/flow-capability-sheet.tsx` | 面板顶部是“待批准（n）”，每条给出工具·命令、主机、角色、子任务与原因，两个按钮；批准后列表消失、判定列表出现新的 `allow`，flow 在跑时 5 秒轮询 |

刻意没做同步等待：让工具调用挂住等操作员点按钮，会在没人看屏幕的夜里把子任务卡到超时，也把并发名额占住。批准是异步的，模型被拒后拿到的是“已提交审批”，它可以继续查别的，或者稍后重试；没有人批，它就一直是拒绝——这才是安全的方向。

## 0.8 这一轮的第四批：策略文件与预算

前面所有策略都写死在 `capability.DefaultPolicy` 里，改一次要改代码、重新编译。这一批把策略变成运维的输入，并给每个工具加上调用次数上限。

| 环节 | 位置 | 行为 |
|---|---|---|
| 策略文件 | `OPSAGI_POLICY`（YAML，示例见 `docs/aiops/policy.example.yml`） | 按 plan mode（`diagnosis` / `optimization`）给 `targets` / `tools` / `commands` / `shapes` / `trace_programs` / `budgets`，外加整体 `ttl` |
| 加载 | `pkg/capability/policyfile.go`、`pkg/config` 的 `LoadCapabilityPolicy` | 启动时读一次。读不到、YAML 坏、写了运行时不签发的 mode、缺 `diagnosis`（未知 mode 的兜底）、`ttl` 解析失败、预算 ≤ 0，全部启动失败——否则运维以为文件生效、实际跑的是内置默认值 |
| 预算 | `Lease.Budgets`、`pkg/tools` 的 `charge` | 每次工具调用记一次数，超过上限就拒，理由写明“已调用 N 次（上限 M），请基于已有证据下结论”。计数在切面里、按子任务一次执行算：重启从 0 开始，它是给重试环路设的闸，不是配额 |
| 不算批准 | 同上 | 预算耗尽**不能**被批准解开：答案是用已有证据下结论或改策略，不是把同一个子任务再放开一轮 |
| 生效范围 | `pkg/providers/capability.go` | 预算不进库：恢复执行的子任务按**当前**策略取预算，所以改小上限重启即生效，不用等“下一个新子任务” |

内置默认仍然是“今天的语义”：`DefaultPolicy` 不带预算、放开全部主机与目录。换句话说，不配置 `OPSAGI_POLICY` 时这套东西一行代码都不多跑；一旦配了，策略和预算同时生效，`enforce` 才开始按文件咬人。

## 0.9 这一轮的第五批：把租约本身也摆出来

判定日志回答“刚才拦了什么”，但操作员更常问的是另一句：“这个子任务现在到底被允许做什么、还剩多少额度”。这一批补上只读的租约面板。

| 环节 | 位置 | 行为 |
|---|---|---|
| 读租约 | `capabilityLeases(flowId, limit)` | 按 flow 取最近 20 张（最多 100），带 `targets` / `tools` / `commands` / `shapes` / `tracePrograms` / `actions` / `planMode` / `expiresAt` |
| 额度 | 同一查询的 `budgets` | 来自**当前**策略文件，与该子任务恢复执行时取预算的路径一致；没有策略文件时为空数组，面板不会凭空画出上限 |
| 已用 | 同一查询的 `used` | 按 `subtask_id` 从 `capability_decisions` 统计 `allow` + `would_deny`（shadow 下越界的调用确实执行了），所以是“审计日志里的真实次数”，不是内存里的计数器 |
| 面板 | `能力租约` 抽屉新增“租约”标签页 | 每张租约一行：子任务、角色、plan mode、到期倒计时；展开后是主机/形状/写操作范围、命令清单，以及 `ops_exec 12 / 60` 这种用量；超过上限标红 |

两个刻意的取舍：`budgets` 不从库里读，因为库里没有这一列（预算只属于“这一次执行”，进库会诱导人把它当成配额）；`used` 不从内存读，因为内存计数器随执行结束消失，而操作员要看的恰恰是“跑到这里为止用掉多少”。两者相加才是运维能拿来做判断的那张图。




## 1. 今天实际生效的四道闸门

| 闸门 | 代码 | 它拦住了什么 | 它没拦住什么 |
|---|---|---|---|
| 角色工具表 | `tools.go:256`（`analystToolset`）、`tools.go:903`（commander 硬编码） | 某角色根本拿不到某工具 | 没有任何“这个子任务只许查这台主机”的收窄；分析师拿到 `ops_exec` 就是全目录；同一次 flow 里 commander 与 change_analyst 的命令面一样宽 |
| 目标主机清单 | `ops.go:126`（`resolve`） | 连不到 `OPSAGI_TARGETS` 之外的地址 | 任何拿到 `ops_*` 的子任务都能打任意一台已配置主机；`target` 留空等于第一台。`OPSAGI_TARGETS` 解析失败曾静默变空表，现在启动即失败（见 0.6） |
| 只读命令目录 | `opsremote/exec.go:40`、`:77` | 只能跑 24 条写死在代码里的只读命令，参数按字符集校验 | 目录不按角色或任务切分；参数是正则字符集不是枚举值。`mysql_connections` / `mysql_processlist` 曾可调用但不在公告目录里，现已补齐并加守卫（见 0.6） |
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

- ~~新开关 `OPSAGI_LEASE_MODE=off|shadow|enforce`~~：开关落地成了每 flow 的选择（见 0.5），不再用环境变量；`shadow` 下按计划与策略算出应当允许的集合，与真实调用比对，把 `would_deny` 写进 `capability_decisions`，**不改变行为**。
- 修三个 fail-open：（已落地，见 0.6）
  - `OPSAGI_TARGETS` 解析失败在启动时报错退出，不再静默变空表（`config.go` 的 `ValidateOpsTargets`）。
  - `mysql_connections`、`mysql_processlist` 补进 `catalogOrder`，并让守卫测试遍历 `catalog` 而不是 `Commands()`（`registry_test.go:265`）。
  - `ops_sandbox` 的非法 `mode` 在 dial 之前就拒绝（今天会先在目标机上跑一次 docker）。
- 交付一份一天的 shadow 数据：`would_deny` 按原因分组（主机越界 / 命令越界 / 形状越界 / 预算超限），这张表决定 P1 的默认策略。

验收：shadow 模式下行为与今天逐字节一致，只有 `lease_decisions` 多出记录。

### P1 租约内核（8–10 人日）

- 新包 `backend/pkg/capability`：`Lease`、`Policy`、`Issue(plan, flow, policy) (Lease, error)`、`Authorize(lease, call) error`、`Revoke`。（已落地：`capability.Lease` / `capability.Policy` / `Lease.Authorize` / `Policy.Issue`；`Revoke` 由 `subtask_id` 级联删除替代。）
- 新迁移 `capability_leases`（见第 4 节），sqlc 再生成。（已落地，见 `20261123120000_add_capability_leases.sql`。）
- 签发点：`PerformAgentChain`（`provider.go:712` 之后、解析出 mode 的地方）在缺少租约时按策略补发一条，保证老数据也能跑，重启用同一行。（已落地；`createPlannedSubtask` 同期写租约没有做，因为懒签发已经覆盖重启与老数据。）
- 强制点：`CapabilityGuard.Apply` 在构造执行器时包住 handler 表，而不是改 `Execute`；`Execute` 仍是唯一漏斗，切面只是 handler 的外层。拒绝走 `Refuse(...)`，文案说清越界的是哪一维、该怎么办。（已落地。）
- 注入点：`GetCommanderExecutor`（`tools.go:903`）与 `GetAnalystExecutor`（`tools.go:1014`）都接受 `Capability`，分析师链路是独立执行器（`aiops_handlers.go:129`），漏了它 commander 的检查就形同虚设。（已落地。）
- 恢复：切点从 `LoadSubtaskWorker` 移到签发处——租约按 `subtask_id` 唯一，`running` 改回 `created` 后重新执行时读的是同一行；已过期就拒绝，不静默续期。（已落地。）
- 默认策略 = 今天的语义（全部主机、全目录、probe 优先），只有 `enforce` 才按策略表收窄，保证可以先上线再收紧。（已落地：`DefaultPolicy`。）

验收：`enforce` 下，计划只写了 `lab` 的子任务对 `prod` 调 `ops_exec` 得到拒绝、没有 SSH 握手、`lease_decisions` 记一条 denied；把拒绝那一行删掉，测试必须红。

### P2 形状与内核层（4–5 人日）

- `lease.Shapes` 决定 probe / workshop / trace，`ops_sandbox.mode` 从“模型选择”变成“租约允许的集合里挑”；不在集合里直接拒绝。
- `OpsSandboxImages` 现在是死字段（`config.go:155` 只有注释），要么真正启用成镜像白名单，要么删掉。
- `ops_trace` 的 `SYS_ADMIN` 重试改成租约里的能力阶梯：第一次必须最窄；重试要么已经写在租约里，要么走 P3 的批准通道。目标机 stderr 不再能单方面把容器抬权。
- 顺带把 flow 容器补上资源上限：今天 `WorkerSpec` 没有内存/CPU 限制、镜像默认用户是 root（`docker/worker_spec.go:18`、`docker/client.go:376`）。租约带 `memory`/`cpus`/`pids` 时为子任务单独建容器是可选项，先做限值。

验收：租约只允许 `probe` 时，模型请求 `workshop` 得到拒绝且目标机上没有 `docker run`；`OPSAGI_SANDBOX_BPF=false` 与租约不含 trace 时，`ops_trace` 都不在工具表里。

### P3 升级闸门（4–5 人日）

- 拒绝不是终点：`Authorize` 返回 `NeedsApproval` 时，切面把这次调用归档成一条 `capability_requests`，拒绝语句里带上请求号。**没有做同步等待**：子任务不进入 `waiting`，它接着干别的，或者稍后重试。
- 操作员在 `能力租约` 面板批准后写一条 `capability_grants`（4 小时），切面每次调用重读有效批准，所以模型的下一**次**重试就通过，不需要重启子任务。按子任务还是按 flow 生效在这里是同一个实现：批准行带 `subtask_id`，但读取按 flow 走，这样重规划出来的新子任务不会因为“换了 id 又变成没批过”再拦一次。
- 每次批准、拒绝都落库（`decided_by`、`decided_at`），加上 `capability_decisions` 里的 `allow/deny`，“谁放开了什么、什么时候用的”能倒查。批准过期后同一调用再次被拒，会重新归档一条新的 `pending`。

验收（已由测试钉住）：没有批准时越界调用永远是 `deny`；批准一次后同一调用通过，另一台主机、另一条命令仍然被拒；批准过期或被拒后重新被拦；重复点批准不会写出第二条租约。

### P4 下沉到 OS 边界（15–20 人日，单独立项）

前三步的强制都在应用层：绕过应用就等于绕过租约。真要让越权“在目标机上不成立”，得把限制搬到凭据和命令两处：

- **短期凭据**：由 SSH CA 为每个租约签一次证书，`permitopen` 限定端口、`command=` 限定入口；租约回收即吊销。比共享 `OPSAGI_TARGETS` 里的长期密码/私钥强一个数量级。
- **目标机侧代理**：部署一个 `opsagi-probe` 受限入口，只接受租约里的命令 ID，参数按同一套 `ParamKind` 校验；这样即使 agent 侧被绕过，目标机也只认那几条命令。
- 只读账号 + `sudoers` 白名单，替代现在的 root/密码模式。

这一步依赖 CA 或代理的部署，收益是“越权在目标机上不成立”，风险是运维复杂度；建议等 P1–P3 上线、shadow 数据显示收紧到什么程度之后再立项。

## 4. 数据模型与配置草案

已落地的两张表（`20261123120000_add_capability_leases.sql`）：租约一行对应一个子任务，`subtask_id` 唯一让“重新签发”变成 `ON CONFLICT DO UPDATE SET role = capability_leases.role`——重规划级联删除子任务时租约跟着走，重启后重读的是同一行。

```sql
ALTER TABLE flows ADD COLUMN capability_mode TEXT NOT NULL DEFAULT 'off'
    CHECK (capability_mode IN ('off', 'shadow', 'enforce'));

CREATE TABLE capability_leases (
    id              BIGSERIAL PRIMARY KEY,
    flow_id         BIGINT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
    subtask_id      BIGINT NOT NULL UNIQUE REFERENCES subtasks(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    mode            TEXT NOT NULL,           -- 计划模式：diagnosis / optimization
    targets         TEXT[] NOT NULL,
    tools           TEXT[] NOT NULL,
    commands        TEXT[] NOT NULL,
    shapes          TEXT[] NOT NULL,
    trace_programs  TEXT[] NOT NULL,
    issued_by       TEXT NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE capability_decisions (
    id          BIGSERIAL PRIMARY KEY,
    flow_id     BIGINT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
    subtask_id  BIGINT REFERENCES subtasks(id) ON DELETE CASCADE,
    lease_id    BIGINT REFERENCES capability_leases(id) ON DELETE SET NULL,
    role        TEXT NOT NULL,
    tool        TEXT NOT NULL,
    target      TEXT NOT NULL DEFAULT '',
    command     TEXT NOT NULL DEFAULT '',
    verdict     TEXT NOT NULL CHECK (verdict IN ('allow', 'deny', 'would_deny')),
    reason      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`max_calls`、`revoked_at` 与按 flow 类型的策略文件仍在路线图上；今天策略只有代码里的 `capability.DefaultPolicy` 一处，字段面就是上面这五行数组。

策略文件（`OPSAGI_POLICY`，YAML，按 flow_type 给默认行；缺省值就是今天的语义）——**尚未实现**，下面是目标格式：

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
| `backend/pkg/capability/capability.go` | `Authorize`、`Grant`、`ApprovedBy`：租约与批准的唯一判定点 |
| `backend/pkg/tools/capability_guard.go` | 切面：`Apply` 包装、`refusal` 归档并引用请求号 |
| `backend/pkg/providers/capability.go` | 签发租约、按调用重读批准、归档请求 |
| `backend/pkg/graph/capability.go` | 行数上限与批准有效期常量 |
| `backend/sqlc/models/capability.sql` | 租约、判定、请求、批准四张表的查询 |
| `frontend/src/features/flows/capability/flow-capability-sheet.tsx` | 判定列表与“待批准”面板 |
| `backend/pkg/capability/policyfile.go` | `OPSAGI_POLICY` 的解析与启动校验 |
| `backend/docs/ops-policy.md` | 策略文件的字段说明、示例与失败方式 |
| `docs/aiops/policy.example.yml` | 可直接拷贝的示例策略（含预算） |
| `backend/sqlc/models/capability.sql` 的 `ListCapabilityLeases` / `CountCapabilityUsesByTool` | 租约与用量的两条只读查询 |
| `frontend/src/features/flows/capability/flow-capability-sheet.tsx` | 抽屉里的“租约”标签页 |
| `backend/pkg/providers/provider.go:740` | 每次执行重读 subtask 行 |
| `backend/pkg/providers/provider.go:892` | 解析 mode、装配出口 |
| `backend/pkg/providers/aiops_handlers.go:129` | 分析师链路独立执行器 |
| `backend/pkg/controller/subtasks.go:106` | `createPlannedSubtask`，计划落库处 |
| `backend/pkg/controller/subtask.go:95` | 重启时 `running` 改回 `created` |
| `backend/pkg/docker/worker_spec.go:18` | flow 容器规格（无内存/CPU 限制） |
| `backend/pkg/tools/args.go:56` | 计划的机器可读字段 |
| `backend/pkg/capability/capability.go` | `Lease.Authorize`、`Mode`、`Verdict`，纯判定，不认识数据库 |
| `backend/pkg/capability/policy.go` | `DefaultPolicy`、`Policy.Issue`，今天的语义写在唯一一处 |
| `backend/pkg/tools/capability_guard.go` | `CapabilityGuard.Apply`，切面包裹、`off` 时原样返回 |
| `backend/pkg/providers/capability.go` | 签发点与审计写入，懒签发 + `Warn` 降级 |
| `backend/pkg/graph/flowtype.go` | `convertCapabilityMode`，缺省 `off` |
| `backend/pkg/graph/capability.go` | 判定列表的缺省与上限（200 / 1000） |
| `backend/pkg/database/converter/converter.go` | `ConvertCapabilityMode`、`ConvertCapabilityDecision`，未知枚举归一成 `off` |
| `backend/pkg/config/config.go` | `ValidateOpsTargets`，`OPSAGI_TARGETS` 在启动时校验 |
| `frontend/src/features/flows/capability/flow-capability-sheet.tsx` | 判定面板：按 verdict 筛、`would_deny` 展示拒绝原因 |
| `frontend/src/features/flows/capability/mode-label.ts` | 开关的中文标签，徽章与面板共用 |
