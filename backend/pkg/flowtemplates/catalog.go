// Package flowtemplates carries the flow templates an installation ships with and
// the seed that copies them into a user's own list the first time it is asked for.
package flowtemplates

import "slices"

// Domains name the six operating situations the shipped templates cover. They are
// the grouping the UI shows; adding a template to a new domain means adding it here.
const (
	DomainAlert      = "告警诊断"
	DomainLogMetric  = "日志与指标"
	DomainChange     = "变更复盘"
	DomainCapacity   = "容量与性能"
	DomainKubernetes = "Kubernetes"
	DomainDatabase   = "数据库与中间件"
)

// Preset is one shipped template. The text is written for the operator to fill in
// and for the agent to read as the flow's task, so it names the probes that answer
// it and the evidence the conclusion must carry.
type Preset struct {
	Domain string
	Title  string
	Text   string
}

var shipped = []Preset{
	{
		Domain: DomainAlert,
		Title:  "CPU 饱和告警：定位吃满 CPU 的进程与诱因",
		Text: `诊断这条 CPU 饱和告警的根因。

已知：
- 目标主机：<主机 ID 或 IP>
- 告警原文：<粘贴告警，例如 "CPU usage > 90% for 10m">
- 首次触发时间：<时间>
- 受影响服务：<服务名与接口>

请按顺序取证：
1. ops_system 取负载、CPU 核数、内存与 Top 进程，先分清是单进程吃满还是整体负载高。
2. ops_containers 看同一时间窗的重启次数与退出码，排除"重启即高峰"。
3. ops_exec 的 top_cpu 与 load 各取一次，确认吃 CPU 的进程名；需要看排队情况时用 psi。
4. ops_logs 取时间窗内的错误日志原文，找出与 CPU 上升同时出现的报错或重试。
5. 需要趋势时用 prom_query 取窗口内的序列，不要拿两次孤立快照下结论。

交付一份诊断：根因、至少两条独立来源的证据（每条写清工具名、时间与原文片段）、影响面、建议动作与回滚方式。只读，不要改线上状态。
`,
	},
	{
		Domain: DomainAlert,
		Title:  "5xx 突增：定位第一个失败请求与共同特征",
		Text: `诊断 5xx 错误率突增，找出第一个失败请求与它的共同特征。

已知：
- 受影响服务：<服务名>
- 错误率：<从 X% 升到 Y%>
- 起始时间：<精确到分钟>
- 上游调用方：<网关 / 客户端>

取证顺序：
1. ops_logs 取起始时间前后各十分钟的错误日志，按状态码与接口分组，找出第一条例外原文。
2. ops_exec 的 listen_ports 与 established 各一次，确认端口仍在监听、连接是否堆积。
3. prom_query 取错误率与延迟序列，确认错误率拐点与延迟拐点是不是同一时刻。
4. 服务在 K8s 里且本实例配了集群时，用 k8s_get 看 pods 与 events：就绪探针失败、镜像拉取失败、OOMKilled 的写法各不相同。
5. 经过网关或反向代理时，用 ops_logs 读代理日志，区分"上游拒绝"与"代理自身报错"。

交付：第一个失败请求的时间与原文、共同特征（接口 / 参数 / 实例）、影响面、建议动作与回滚方式。只读。
`,
	},
	{
		Domain: DomainLogMetric,
		Title:  "错误日志首次出现点与指标拐点对齐",
		Text: `把错误日志的首次出现时间和监控指标的拐点对齐，判断哪一个先发生。

已知：
- 服务：<服务名>
- 时间窗：<开始> 至 <结束>
- 可疑现象：<例如 "P99 从 180ms 涨到 900ms">

做法：
1. ops_logs 逐实例读日志，记录第一条错误的时间戳与原文；多个实例逐一对齐，不要只看一个。
2. prom_query 取同一时间窗的延迟、错误率、QPS 序列，看斜率，不要用端点差代替趋势。
3. ops_exec 的 docker_stats 对齐容器级 CPU 与内存，排除资源竞争。
4. 把两侧的时刻排成一条时间线，每个点标注取证工具与时间戳。

交付：一条时间线（谁先动、谁后动、间隔多久）、最可能的因果方向、还需要哪一条证据才能确认。只读。
`,
	},
	{
		Domain: DomainLogMetric,
		Title:  "错误日志聚类：同一模式影响哪些实例",
		Text: `对给定时间窗内的错误日志做聚类，找出影响面最大的模式。

已知：
- 服务与实例列表：<服务名 / 实例 ID>
- 时间窗：<开始> 至 <结束>
- 已知的一条报错：<原文>

做法：
1. 用 ops_logs 逐实例取日志，保留原始行与时间戳。
2. 按错误文本归一化（去掉时间、请求 ID、数字）后分组，统计每个模式的首次出现、末次出现、实例分布与次数。
3. 对最大的两三个模式，用 ops_exec 的 package_history 与 image_history 确认近期是否部署或升级，把模式与变更对齐。
4. 怀疑与资源相关时，用 ops_exec 的 memory 或 docker_stats 补一条证据。

交付：模式清单（模式、次数、实例分布、首末时间）、每条模式的证据片段、最值得先修的一条及理由。只读。
`,
	},
	{
		Domain: DomainChange,
		Title:  "发布后异常归因：变更时间线与现象起点对齐",
		Text: `复盘一次发布窗口内的异常，判断它与本次变更的关系。

已知：
- 发布的服务与版本：<服务> <旧版本> → <新版本>
- 发布时间与方式：<时间，滚动 / 全量 / 蓝绿>
- 异常现象：<...>
- 观察窗口：<开始> 至 <结束>

做法：
1. ops_exec 的 unit_activated_at 或 docker_inspect 取实际启动时刻，用真实生效时间，不用流水线时间。
2. prom_query 取变更前后的关键指标序列，标出第一个偏离基线的点。
3. ops_logs 取变更后的第一批错误日志原文，看报错是否指向新版本代码或配置。
4. ops_exec 的 image_history 或 package_history 确认镜像与依赖的版本变化。
5. 若变更前采过基线，用 ops_diff 对照得出结论；没有基线就写明缺的是哪一次采样。

交付：变更与现象的时间关系（同时 / 之后 / 无关）、支持结论的至少两条证据、回滚还是继续观察的建议与回滚步骤。只读，不执行回滚。
`,
	},
	{
		Domain: DomainChange,
		Title:  "变更前基线采集与验收标准",
		Text: `为即将执行的变更做一次基线采集，并写下验收与回滚标准。

已知：
- 计划变更：<服务 / 配置 / 版本>
- 变更窗口：<开始> 至 <结束>
- 观察指标：<延迟、错误率、重启次数>

做法：
1. 用 ops_baseline 在变更前采一次完整基线，记下采集时刻。
2. ops_exec 的 listen_ports、unit_status、disk_usage、memory 各取一次，留下可回滚的现状快照。
3. 与操作员确认验收标准：指标回到什么区间算成功，出现哪些信号就必须回滚。
4. 变更后用 ops_diff 对照基线，输出代码算出的变化结论，不要自己再采一次再心算。

交付：基线值清单、验收与回滚阈值、变更后需要复核的检查项。只读。
`,
	},
	{
		Domain: DomainCapacity,
		Title:  "P99 劣化：定位瓶颈并给出可测量的改进方案",
		Text: `定位接口 P99 延迟劣化的瓶颈，并给出可测量的改进方案。

已知：
- 接口：<路径>
- 当前 P99 与目标：<现值> → <目标值>
- 开始劣化的时间：<...>
- 流量水平：<QPS>

做法：
1. prom_query 取 P99、P50、QPS 与错误率序列，先判断是整体变慢还是长尾变长。
2. ops_system 与 ops_exec 的 top_cpu、top_mem、psi 各一次，分清 CPU、内存还是 IO 压力。
3. 本实例开了内核追踪时用 ops_trace 的 runqlat 或 offcputime，看时间花在排队还是阻塞；没开就写明分辨率受限。
4. ops_exec 的 established 与 pg_stat_activity（数据库在目标主机时）确认连接池是否打满。
5. 改进前后各采一次基线，用 ops_diff 给出差值结论，不要凭感觉说"变快了"。

交付：瓶颈所在的层与证据、改进建议（含风险与回滚）、一组可复现的前后测量方法。只读，不执行变更。
`,
	},
	{
		Domain: DomainCapacity,
		Title:  "容量水位：磁盘与内存还能撑多久",
		Text: `评估目标主机的容量余量，回答"还能撑多久、什么时候必须扩容"。

已知：
- 目标主机：<主机 ID 或 IP>
- 关注的资源：<磁盘 / 内存 / inode>
- 增量来源：<日志、数据文件、缓存>

做法：
1. ops_exec 的 disk_usage、inode_usage、memory 各取一次，记录使用率与绝对量。
2. 相隔至少十分钟采两次（ops_watch 可连续采样）估算增长速度，据此推算耗尽时间，并写明假设。
3. ops_logs 与 ops_exec 的 kernel_log 查写满、只读挂载、OOM 杀进程的痕迹。
4. prom_query 取文件系统与内存序列，验证估算方向与实测一致。

交付：当前水位、增长速率、预计耗尽时间（附假设）、先行处置与扩容建议。只读。
`,
	},
	{
		Domain: DomainKubernetes,
		Title:  "Pod 反复重启（CrashLoopBackOff）根因",
		Text: `诊断 Pod 反复重启的根因，区分调度、镜像、探针与应用自身四类原因。

已知：
- 集群与命名空间：<...>
- Pod / 工作负载：<名字>
- 重启次数与时间窗：<...>

做法：
1. k8s_get 取 pods，看 waiting 原因、上一次终止的退出码与重启次数。
2. k8s_get 取 events 并按时间排序，找出调度失败、拉镜像失败、探针失败的第一条原因。
3. 退出码 137 通常是被杀或 OOM，退出码 1 通常是启动配置错误；用 ops_exec 的 docker_inspect 交叉确认资源限制与 OOMKilled。
4. Pod 能起但马上退出时，读容器日志首尾各一百行，定位第一条致命错误。
5. 怀疑与节点相关时，k8s_get 看 nodes 的资源与状态，确认是否集中在一个节点。

交付：根因类别与证据、涉及的工作负载、修复建议与验证方式（重启后看哪些指标）。只读，不改任何资源。
`,
	},
	{
		Domain: DomainKubernetes,
		Title:  "Pod 一直 Pending：调度不上去的原因",
		Text: `定位 Pod 一直 Pending 的原因，给出能落地的调度修复建议。

已知：
- 集群与命名空间：<...>
- Pending 的 Pod：<名字或标签选择器>
- 持续时间：<...>

做法：
1. k8s_get 的 events 找 FailedScheduling 原文，区分资源不足、亲和性冲突、污点与存储未就绪。
2. k8s_get 的 nodes 对比可分配与已分配资源，量化缺口（差多少 CPU / 内存）。
3. 存储类与 PVC 状态 k8s_get 不直接给，用 events 里的卷名与失败信息定位，并写明取证工具。
4. 需要节点侧证据时用 ops_system 与 ops_exec 的 disk_usage，排除磁盘或 inode 触发的污点。

交付：阻塞原因与证据原文、缺口量化、建议（扩容 / 调整 requests / 改亲和性）与回滚方式。只读。
`,
	},
	{
		Domain: DomainDatabase,
		Title:  "数据库变慢：慢查询、锁等待还是连接池耗尽",
		Text: `诊断数据库响应变慢，区分慢查询、锁等待与连接池耗尽。

已知：
- 数据库类型与地址：<PostgreSQL / MySQL，主机:端口>
- 使用它的服务：<服务名>
- 现象：<超时 / 连接被拒绝 / P99 上升>

做法：
1. ops_exec 的 pg_stat_activity 或 mysql_processlist 看当前活动会话与执行时间最长的那条。
2. pg_locks 与 pg_lock_wait_summary 看是否存在锁等待链，写明谁在等谁。
3. pg_stat_statements 按总耗时与调用次数排序，分清"一次很慢"与"很多次都慢"。
4. mysql_connections 或 pg_stat_activity 的计数对比连接上限，判断是否连接池耗尽；同时看服务侧 established 是否堆积。
5. 对最慢的一条语句用 pg_explain 取执行计划，标注是否走索引、是否全表扫描。

交付：瓶颈类别（慢查询 / 锁 / 连接数）、证据与原文、影响的服务、建议动作与回滚方式。只读，不执行 kill 或 DDL。
`,
	},
	{
		Domain: DomainDatabase,
		Title:  "中间件延迟毛刺：Redis 与消息队列",
		Text: `定位中间件（Redis 或消息队列）延迟毛刺的根因。

已知：
- 中间件类型与地址：<Redis / Kafka / RabbitMQ，主机:端口>
- 使用它的服务：<服务名>
- 毛刺表现：<P99 周期性尖刺 / 消费积压>

做法：
1. ops_exec 的 established 与 listen_ports 确认连接数与端口状态，排除连接风暴。
2. ops_system 与 docker_stats 看宿主与容器的 CPU、内存、网络是否在同一时刻抖动。
3. ops_logs 取中间件日志，找超时、拒绝连接、慢操作、主从切换的原文。
4. ops_trace 的 tcpconnlat 或 biolatency 确认是网络往返还是落盘导致的延迟。
5. 消费类中间件再核对消费组积压的增长曲线，判断是生产变快还是消费变慢。

交付：毛刺的时间点与证据、根因类别、影响面、处置建议与回滚方式。只读。
`,
	},
}

// Shipped returns the templates this build ships. The slice is a copy: seeding and
// the GraphQL query must not be able to rewrite the catalogue for the next caller.
func Shipped() []Preset {
	return slices.Clone(shipped)
}
