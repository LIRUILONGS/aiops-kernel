# M0：只读诊断

## 1. 目标

针对**一类**故障（K8s Pod CrashLoopBackOff），端到端跑通：

```
告警输入 → 多路取证 → 根因假设 → 验证 → 带证据的结论
```

全程只读。不重启、不改配置、不扩容。

## 2. 输入与输出

输入是一个 JSON——告警原文加少量上下文：

```json
{
  "alert": "Pod payments-api-7d9f is in CrashLoopBackOff",
  "namespace": "prod-payments",
  "pod": "payments-api-7d9f",
  "first_seen": "2026-11-06T10:12:00Z",
  "cluster": "prod-shanghai"
}
```

输出是一个结构化对象，字段本身就是验收标准：

```json
{
  "root_cause": "一句话根因",
  "evidence": [
    {
      "source": "k8s|logs|metrics|changes",
      "query": "实际执行的查询",
      "timestamp": "2026-11-06T10:11:40Z",
      "excerpt": "原始输出片段（截断到 500 字内）"
    }
  ],
  "confidence": 0.78,
  "blast_radius": "影响支付下单，约 3% 请求",
  "recommendation": {
    "action": "回滚到 v2.3.1",
    "risk": "low",
    "rollback": "重新 apply v2.3.2"
  },
  "needs_human": true
}
```

## 3. 验收标准

| 指标 | 目标 | 怎么测 |
|---|---|---|
| 首份结论时间 | ≤ 5 分钟 | 从 flow 创建到 `diagnosis` 返回 |
| 根因正确率 | ≥ 60% | 20 条历史告警人工标注 |
| 证据可复核率 | **100%** | 每条 evidence 都能用 query 复现 |
| 单次成本 | ≤ $1 | token 记账 |
| 幻觉引用 | **0** | 抽查 evidence 是否由工具返回 |

最后两项是硬指标。前两项是软指标——60% 的根因正确率听起来低，但配合"证据链"已经能显著缩短 MTTR，因为人可以直接跳到验证环节。

## 4. 技术方案：M0 不碰沙箱

这是本阶段最重要的取舍。

PentAGI 的终端工具跑在它自己起的 Docker 容器里——那是给"攻击靶机"准备的。运维诊断要读的是**真实系统**，套在容器里既没必要，也连不上生产。

所以 M0 新增的取证工具**全部在进程内执行**，直接调外部 API：

| 工具 | 做什么 | 依赖 |
|---|---|---|
| `k8s_get` | 读 Pod / Event / Deployment（只读，`get`/`list` 动词） | kubeconfig 或 in-cluster |
| `loki_query` | 按时间窗和标签查日志 | Loki HTTP API |
| `prom_query` | PromQL 即时/区间查询 | Prometheus HTTP API |
| `change_query` | 查发布与变更记录 | M0 用文件 mock，M1 接 CMDB |

沙箱只在 M2 需要执行动作时才引入（SSH / K8s 写操作），那时再抽 `ExecutionBackend`。

**这个取舍把 M0 的改造面从"重写工具执行层"缩小到"新增四个只读工具"。**

## 5. 角色（5 个）

| 角色 | 出口 | 职责 |
|---|---|---|
| `commander` | `diagnosis` / `ask_user` | 决定查什么、何时收敛、下结论 |
| `triage` | `triage_result` | 分级：影响面、紧急度、是否已知模式 |
| `log_analyst` | `log_finding` | 日志异常模式 |
| `metrics_analyst` | `metric_finding` | 指标拐点与相关性 |
| `change_analyst` | `change_finding` | 变更、发布、配置漂移 |

M0 没有 `ops_executor`——因为不执行。

`commander` 的工具白名单里只有这四个分析角色 + `diagnosis` + `ask_user`，**没有终端、没有写操作**。权限靠白名单，不靠提示词。

## 6. 任务分解

- [ ] 1. 替换角色集合：`MsgchainType` 枚举 → 5 个新值（含迁移与 GraphQL 同步）
- [x] 2. 写提示词（`prompts/aiops/`）：commander、planner、triage、log_analyst、metrics_analyst、change_analyst、incident_context 已完成；refiner、reporter 待补
- [ ] 3. 实现 4 个只读工具 + schema + 执行器
- [ ] 4. 诊断入口：从告警 JSON 创建 flow（M0 可以先用现有 REST/GraphQL）
- [ ] 5. 评测集：20 条历史告警 + 标注根因
- [ ] 6. 跑通基线，记录时间/成本/准确率

顺序不能反：**1 → 3 → 2 → 4 → 5 → 6**。先有角色和工具，提示词才有东西可写；先有评测集，调 prompt 才有意义。

## 7. 评测集：最容易被低估的一块

没有标注数据，你无法证明任何事——改一句 prompt 是变好还是变坏，全凭感觉。

三条获取途径，按可靠性排序：

1. **历史工单**（最好）：真实、有结论、有上下文。缺点是标注成本高。
2. **混沌工程注入**（可控）：主动制造已知根因，看系统能否找到。缺点是分布偏。
3. **公开故障复盘**（补充）：各家 postmortem，可当阅读理解题。

M0 至少 20 条，覆盖三类根因：镜像/启动问题、配置错误、依赖不可用。

## 8. Kill criteria

如果 M0 结束时出现以下任一情况，**停止扩张，退回去做检索增强**：

- 根因正确率 < 40%
- 单次成本 > $3
- 平均耗时 > 10 分钟（人已经自己查完了）
- 证据可复核率 < 80%（说明模型在编）

## 9. M0 不做

- 写操作、审批流、动作账本
- 多租户与凭据管理
- 告警接入与降噪（手写 JSON 即可）
- 前端改造（先用现有页面）
- 并发取证（单写者队列够用）
