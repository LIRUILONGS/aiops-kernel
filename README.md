<p align="center">
  <img src="assets/logo.svg" alt="OpsAGI" width="112">
</p>

<h1 align="center">OpsAGI</h1>

<p align="center"><b>一个会自己取证、自己下结论的 AIOps 智能体内核：面向真实主机的故障定位与性能优化，交付物是带证据的结论，不是聊天记录。</b></p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/LIRUILONGS/aiops-kernel" alt="License"></a>
  <a href="backend/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/LIRUILONGS/aiops-kernel?filename=backend/go.mod" alt="Go"></a>
  <img src="https://img.shields.io/badge/%E7%95%8C%E9%9D%A2-%E5%85%A8%E4%B8%AD%E6%96%87-f59e0b" alt="界面全中文">
  <a href="https://github.com/LIRUILONGS/aiops-kernel/commits/main"><img src="https://img.shields.io/github/last-commit/LIRUILONGS/aiops-kernel" alt="Last commit"></a>
</p>

<p align="center"><b>导航：</b> <a href="#它做什么">它做什么</a> · <a href="#它不做什么">它不做什么</a> · <a href="#截图">截图</a> · <a href="#核心机制">核心机制</a> · <a href="#界面">界面</a> · <a href="#快速开始">快速开始</a> · <a href="#文档">文档</a></p>

OpsAGI 是 [PentAGI](https://github.com/vxcontrol/pentagi)（MIT）的分支。上游是一个自主渗透测试平台；这棵树保留它的运行时——flow / task / subtask 调度、模型提供商层、工具调用执行器、GraphQL API 与 React 界面——把智能体、工具与提示词换成值班工程师的那一套：SSH 上的只读取证、可回溯的诊断结论、带实测判定的性能改动。

界面全中文，默认深色主题。

---

## 它做什么

输入一条告警（或一句"这台机器为什么慢"），它会：

1. 把问题拆成几条**可验证的假设**（计划即数据，每条假设带 `diagnosis` 或 `optimization` 类型）；
2. 派给对应的分析师，各自只读一个证据面（日志 / 指标 / 变更 / 分级）；
3. 在目标主机上执行**只读命令**，或起一个一次性容器在里面读宿主机；
4. 用 `diagnosis` 或 `optimization_result` 收尾，**没有证据会被打回重填**。

产出的结论长这样：根因一句话、至少两三条能复现的查询、置信度、影响面、建议动作与回滚方式、是否需要人工介入。

## 它不做什么

- **不改线上状态。** 唯一的例外是经操作员批准的取证动作（一次性采集容器、取证工具安装），且必须写进 `evidence` 与 `recommendation.rollback`。修复永远只写成建议。
- **不做单点猜测。** 单一信号只能提示原因，下根因结论至少要两个独立来源交叉验证。
- **不假装查过。** 没调用的工具、没看到的日志行、没发生的对话，都不允许出现在报告里。

---

## 截图

界面截图取自仓库自带的视觉基线（`frontend/e2e/specs/visual/`），数据是测试夹具里的演示会话，不是真实客户环境。图随 GitHub 主题切换。

<table>
  <tr>
    <td colspan="2" align="center">
      <a href="assets/screenshots/dashboard-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/dashboard-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/dashboard-light.png">
        <img src="assets/screenshots/dashboard-light.png" alt="仪表盘" width="100%">
      </picture></a>
      <br>
      <sub><b>仪表盘。</b> 最近的会话、成功率与未收尾的告警；一眼看出值班当天有没有悬着的东西。</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <a href="assets/screenshots/flows-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/flows-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/flows-light.png">
        <img src="assets/screenshots/flows-light.png" alt="会话列表" width="100%">
      </picture></a>
      <br>
      <sub><b>会话列表。</b> 每个 flow 的标题就是一条告警；状态、耗时、消耗都在列表上。</sub>
    </td>
    <td width="50%" align="center">
      <a href="assets/screenshots/flow-detail-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/flow-detail-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/flow-detail-light.png">
        <img src="assets/screenshots/flow-detail-light.png" alt="会话详情" width="100%">
      </picture></a>
      <br>
      <sub><b>会话详情。</b> 右侧是终端、任务、智能体、角色等观察面；诊断结论按证据组织。</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <a href="assets/screenshots/templates-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/templates-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/templates-light.png">
        <img src="assets/screenshots/templates-light.png" alt="模板" width="100%">
      </picture></a>
      <br>
      <sub><b>模板。</b> 常用问法存成模板，下次只换主机名。</sub>
    </td>
    <td width="50%" align="center">
      <a href="assets/screenshots/knowledges-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/knowledges-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/knowledges-light.png">
        <img src="assets/screenshots/knowledges-light.png" alt="知识库" width="100%">
      </picture></a>
      <br>
      <sub><b>知识库。</b> 值班手册与历史结论入库，检索时按条目召回。</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <a href="assets/screenshots/resources-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/resources-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/resources-light.png">
        <img src="assets/screenshots/resources-light.png" alt="资源" width="100%">
      </picture></a>
      <br>
      <sub><b>资源。</b> 取证脚本、日志包、截图挂在会话外，可跨 flow 复用。</sub>
    </td>
    <td width="50%" align="center">
      <a href="assets/screenshots/settings-providers-light.png"><picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/settings-providers-dark.png">
        <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/settings-providers-light.png">
        <img src="assets/screenshots/settings-providers-light.png" alt="模型提供商" width="100%">
      </picture></a>
      <br>
      <sub><b>模型提供商。</b> 任意 OpenAI 兼容端点，按角色分配模型。</sub>
    </td>
  </tr>
</table>

---

## 核心机制

### 四层

| 层 | 位置 | 职责 |
|---|---|---|
| L0 模型 | 任意 OpenAI 兼容端点 | 决定查什么、怎么查 |
| L1 Harness | `backend/pkg/tools` | 工具表与调用执行、容器生命周期 |
| L2 控制面 | `backend/pkg/controller` | flow / task / subtask 调度、状态机、日志与订阅 |
| L3 领域 | `backend/pkg/templates/prompts/*.tmpl`、`pkg/tools/aiops.go` | 角色提示词、出口 schema、校验规则 |

分层的目的不是整洁，是**让约束有地方落**：模型可以自由选工具，但它选不到的工具就是不存在；它想交的结论要过代码校验，过不了就重来。

### 工具

只读取证（`OPSAGI_TARGETS` 里配的主机，走 SSH）：

`ops_system`、`ops_containers`、`ops_logs`、`ops_baseline`、`ops_diff`、`ops_exec`（固定目录的只读命令，只选 ID 与参数）、`ops_watch`（一条命令在时间窗内重复采样）。

自由通道（边界在容器形状，不在命令名单）：

- `ops_sandbox` — 在目标主机上起一次性容器执行脚本。两种形状：`probe`（只读根文件系统、无网络、256MB）与 `workshop`（可写、可出网、512MB）。都 `--cap-drop=ALL`、都不进宿主机 PID 命名空间，宿主机的 `/proc` 与 `/sys` 只读挂在 `/host/proc`、`/host/sys`。**能读到宿主机的一切，却改不了它。**

内核层（`OPSAGI_SANDBOX_BPF` 打开时）：

`ops_trace` — 五个固定程序（`runqlat` / `offcputime` / `biolatency` / `execsnoop` / `tcpconnlat`），程序正文在代码里，模型只选 ID 与采样窗口。容器丢光 capability 后加回 `BPF`、`PERFMON`、`SYS_RESOURCE`，`/sys/kernel` 只读挂载，仍无网络、不进宿主机 PID 命名空间；老内核因为缺 `SYS_ADMIN` 拒绝 `perf_event_open` 时，自动补这一个能力重试一次——第一次永远是最窄的形状。

旁路（直接问监控系统，不经过主机 shell）：

- `prom_query`（`PROMETHEUS_URL`）、`k8s_get`（`K8S_KUBECONFIG`）。没配就不出现在工具表里，而不是出现了再报错。

出口：

`diagnosis`、`optimization_result`、`triage_result`、`log_finding`、`metric_finding`、`change_finding`。

### 策略即环境变量

权限、网络、镜像拉取都通过操作员的配置决定，不写在提示词里：

| 变量 | 默认 | 作用 |
|---|---|---|
| `OPSAGI_TARGETS` | 空 | 目标主机清单（JSON 数组：id / host / port / user / password 或 key） |
| `PROMETHEUS_URL` | 空 | 配了才有 `prom_query` |
| `K8S_KUBECONFIG` | 空 | 配了才有 `k8s_get` |
| `OPSAGI_SANDBOX_NETWORK` | `false` | 允许取证容器出网 |
| `OPSAGI_SANDBOX_PULL` | `false` | 允许拉取镜像 |
| `OPSAGI_SANDBOX_BPF` | `false` | 打开 `ops_trace`（内核层 eBPF 追踪），关着时工具不存在 |
| `OPSAGI_BPF_IMAGE` | `quay.io/iovisor/bpftrace:latest` | 追踪容器用的镜像，里面要有 `bpftrace` |
| `ASK_USER` | `false` | 打开 `ask` 工具（人工闸门） |
| `OPSAGI_SUBTASK_CONCURRENCY` | `1` | 同一任务内子任务并发度 |

---

## 界面

一个 flow 的右侧是可切换的观察面：终端、任务、智能体、**角色**、工具调用、向量库、文件、截图。

**角色**页回答"这一轮是谁在干活"。上方是按层摆放的十二个角色拓扑——规划（生成 / 精炼）在上，指挥官居中，四个取证分析师在下，消费与修补型角色在底；节点上带该角色在本 flow 的 chain 数、日志条数、token 用量与最后活动时间，一分钟内有动作的角色描一圈绿边，发生过的委派用带次数的箭头连起来。下方是按角色过滤的日志：点节点或点角色标签，只看这个角色写下或应答的条目。

图用 [`@xyflow/react`](https://reactflow.dev) 画。数据来自两条聚合：`msgchains` 按角色汇总 chain / token / 费用，`agentlogs` 按 `initiator → executor` 计数；查询只在 flow 运行中每 5 秒轮询一次，结束后不再刷新。

---

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

约半分钟，需要 Docker（流程自带的终端容器），任何一步不成立都会非零退出。

---

## 目录

```
backend/                 Go 服务：REST + GraphQL
  cmd/opsagi/            服务入口
  cmd/opsagi-seed/       管理员与密码种子
  pkg/controller/        flow / task / subtask 运行时
  pkg/tools/             工具表与执行器；ops.go / opsremote / aiops.go 是本项目新增的取证层
  pkg/opsremote/         只读命令目录与 SSH 执行
  pkg/templates/prompts/ 角色提示词（中文）
frontend/                React + TypeScript 界面
docs/aiops/              设计、交接、验收记录
docs/platform-reference.md  继承自 PentAGI 的平台文档（提供商与模型目录、部署细节）
scripts/                 命令行工具与批测脚本
```

---

## 文档

- [总览](docs/aiops/00-overview.md) — 这棵树要做什么，与上游的关系，分层为什么这样切
- [M0 计划](docs/aiops/01-m0-plan.md) — 只读诊断的端到端目标与验收口径
- [改造清单](docs/aiops/02-changes.md) — 相对上游的删 / 改 / 留 / 增，精确到路径
- [交接文档](docs/aiops/03-handover.md) — 当前状态、验证到什么程度、哪里还没验证（另有 [.docx](docs/aiops/03-handover.docx)）
- [平台参考](docs/platform-reference.md) — 从上游继承的部署与提供商配置细节

---

## 状态

已实测通过：真实主机上的故障定位（含 OOM 循环，三家证据交叉验证）、性能优化的判定链路、纯日志的离线诊断、容器终端面板的实时回显、日志数据集批量回归。

尚未验证：多目标主机并发、长时间运行的内存与连接泄漏、非 Debian 系的宿主机取证。

---

## 许可

代码遵循 MIT（见 [LICENSE](LICENSE)），并受 [EULA](EULA.md) 约束；上游版权归 PentAGI 开发团队。
