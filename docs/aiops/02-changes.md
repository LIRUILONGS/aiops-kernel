# 改造清单

按"删 / 改 / 留 / 增"四类，精确到路径。执行顺序见文末。

## 1. 删（渗透专用）

| 路径 | 内容 | 处理 |
|---|---|---|
| `backend/pkg/templates/prompts/*.tmpl` | 39 份渗透提示词 | 全部删除，换成 `prompts/aiops/` |
| `backend/pkg/tools/` 中的 `hack_result`、`sploitus` 搜索、`search_guide`/`store_guide`、`advice` | 渗透专用工具 | 删除工具与注册项 |
| `frontend` 中的攻击相关文案与图标 | 术语 | 先改文案，不动结构 |
| `backend/pkg/graph/schema.graphqls` 的渗透域字段 | 如攻击结果类型 | M0 暂留，M2 清理 |

删除时注意：工具是从 `registry.go` 注册、在 `executor.go` 分发的，删工具要同时删注册、schema、以及各角色执行器里的引用。

## 2. 改（保留结构，换语义）

| 路径 | 改什么 |
|---|---|
| `backend/pkg/database/models.go` | `MsgchainType` 枚举 → 新的角色集合 |
| `backend/migrations/sql/` | 新增迁移：重建 `MSGSCHAIN_TYPE` 枚举（Postgres 不能就地扩展） |
| `backend/pkg/graph/schema.graphqls` | `AgentType` 枚举同步 |
| `frontend/src/graphql/types.ts` | 由 `pnpm run graphql:generate` 重新生成 |
| `backend/pkg/providers/provider.go` | 提示词加载路径、模型槽位名 |
| `backend/pkg/controller/` | flow/task/subtask 的术语：flow→incident，task→investigation |
| `backend/pkg/tools/tools.go` | 各角色执行器的工具白名单 |
| 所有面向模型的报错文案 | 英文 → 中文（模型会读到） |

## 3. 留（零改动或近零）

| 路径 | 说明 |
|---|---|
| `backend/pkg/cast/` | ChainAST，纯数据结构 |
| `backend/pkg/csum/` | 压缩，只认 chain |
| `backend/pkg/providers/` 下的各家适配 | 与领域无关 |
| `backend/pkg/docker/` | M0 不用，M2 经 ExecutionBackend 使用 |
| `backend/pkg/observability/` | 与领域无关 |
| `backend/pkg/schema/`、`pkg/password/`、`pkg/version/` | 基础设施 |

## 4. 增（AIOps 特有）

| 路径 | 内容 |
|---|---|
| `backend/pkg/templates/prompts/aiops/` | 新提示词（中文） |
| `backend/pkg/tools/aiops/` | 只读取证工具：`k8s_get`、`loki_query`、`prom_query`、`change_query` |
| `backend/pkg/aiops/incident/` | 告警模型与入口解析 |
| `docs/aiops/` | 设计文档 |

## 5. 角色映射

| 旧（PentAGI） | 新（AIOps） | 出口 |
|---|---|---|
| `primary_agent` | `commander` | `diagnosis` / `ask_user` |
| `generator` | `planner` | `subtask_list` |
| `refiner` | `refiner`（保留） | `subtask_patch` |
| — | `triage` | `triage_result` |
| `searcher` | `log_analyst` | `log_finding` |
| `memorist` | `metrics_analyst` | `metric_finding` |
| `installer` | `change_analyst` | `change_finding` |
| `reporter` | `reporter`（保留） | —— |
| `summarizer` | `summarizer`（保留） | —— |
| `tool_call_fixer` | `tool_call_fixer`（保留） | —— |
| `pentester`、`coder`、`adviser`、`reflector`、`enricher`、`assistant` | M0 删除，M2 按需重建 | —— |

新增角色时，出口工具（barrier）必须同时增加，否则该角色的循环没有正常终止路径。

## 6. 执行顺序

1. **角色枚举**：先改 `models.go` 与迁移，再改 GraphQL，再重新生成前端类型。
2. **工具层**：删渗透工具，加四个只读工具。
3. **提示词**：路径指向 `prompts/aiops/`。
4. **入口**：从告警 JSON 创建 flow。
5. **评测集**：20 条历史告警。

顺序不能反：提示词要引用工具名，工具要先存在；角色要先定义，提示词才有归属。

## 7. 每一步的验证方式

| 步骤 | 验证 |
|---|---|
| 枚举 | `go build ./...` 通过；迁移在空库上可执行 |
| 工具 | 单测：参数校验、只读断言、错误分类 |
| 提示词 | 模板渲染测试（所有 `.tmpl` 能被 `text/template` 解析） |
| 入口 | 手写一条告警 JSON，端到端跑通并产生 `diagnosis` |
| 评测集 | 20 条跑分，记录准确率、耗时、成本 |
