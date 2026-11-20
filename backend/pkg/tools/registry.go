package tools

import (
	"maps"
	"opsagi/pkg/database"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/vxcontrol/langchaingo/llms"
)

const (
	FinalyToolName             = "done"
	AskUserToolName            = "ask"
	BrowserToolName            = "browser"
	GoogleToolName             = "google"
	DuckDuckGoToolName         = "duckduckgo"
	TavilyToolName             = "tavily"
	FirecrawlToolName          = "firecrawl"
	TraversaalToolName         = "traversaal"
	PerplexityToolName         = "perplexity"
	SearxngToolName            = "searxng"
	SploitusToolName           = "sploitus"
	WebSearchToolName          = "web_search"
	SearchInMemoryToolName     = "search_in_memory"
	SearchGuideToolName        = "search_guide"
	StoreGuideToolName         = "store_guide"
	SearchAnswerToolName       = "search_answer"
	StoreAnswerToolName        = "store_answer"
	SearchCodeToolName         = "search_code"
	StoreCodeToolName          = "store_code"
	GraphitiSearchToolName     = "graphiti_search"
	ReportResultToolName       = "report_result"
	SubtaskListToolName        = "subtask_list"
	SubtaskPatchToolName       = "subtask_patch"
	TerminalToolName           = "terminal"
	FileToolName               = "file"
	GetFlowStatusToolName      = "get_flow_status"
	StopFlowToolName           = "stop_flow"
	SubmitFlowInputToolName    = "submit_flow_input"
	PatchFlowSubtasksToolName  = "patch_flow_subtasks"
	WaitFlowCompletionToolName = "wait_flow_completion"
	OpsSystemToolName          = "ops_system"
	OpsContainersToolName      = "ops_containers"
	OpsLogsToolName            = "ops_logs"
	OpsBaselineToolName        = "ops_baseline"
	OpsDiffToolName            = "ops_diff"
	OpsExecToolName            = "ops_exec"
	OpsWatchToolName           = "ops_watch"
	OpsSandboxToolName         = "ops_sandbox"
	OpsTraceToolName           = "ops_trace"
	PromQueryToolName          = "prom_query"
	K8sGetToolName             = "k8s_get"
	RecordFindingToolName      = "record_finding"
	DiagnosisToolName          = "diagnosis"
	TriageResultToolName       = "triage_result"
	LogFindingToolName         = "log_finding"
	MetricFindingToolName      = "metric_finding"
	ChangeFindingToolName      = "change_finding"
	OptimizationResultToolName = "optimization_result"

	// The four analysts are reached through delegation tools rather than through
	// the executor: the commander decides which evidence face the next probe
	// belongs to, and each tool runs that analyst's own chain and returns its
	// finding as the tool result.
	TriageToolName         = "triage"
	LogAnalystToolName     = "log_analyst"
	MetricsAnalystToolName = "metrics_analyst"
	ChangeAnalystToolName  = "change_analyst"
)

type ToolType int

const (
	NoneToolType ToolType = iota
	EnvironmentToolType
	SearchNetworkToolType
	SearchVectorDbToolType
	StoreAgentResultToolType
	StoreVectorDbToolType
	BarrierToolType
)

func (t ToolType) String() string {
	switch t {
	case EnvironmentToolType:
		return "environment"
	case SearchNetworkToolType:
		return "search_network"
	case SearchVectorDbToolType:
		return "search_vector_db"
	case StoreAgentResultToolType:
		return "store_agent_result"
	case StoreVectorDbToolType:
		return "store_vector_db"
	case BarrierToolType:
		return "barrier"
	default:
		return "none"
	}
}

// GetToolType returns the tool type for a given tool name
func GetToolType(name string) ToolType {
	if toolType, ok := toolsTypeMapping[name]; ok {
		return toolType
	}
	return NoneToolType
}

var toolsTypeMapping = map[string]ToolType{
	FinalyToolName:             BarrierToolType,
	AskUserToolName:            BarrierToolType,
	BrowserToolName:            SearchNetworkToolType,
	GoogleToolName:             SearchNetworkToolType,
	DuckDuckGoToolName:         SearchNetworkToolType,
	TavilyToolName:             SearchNetworkToolType,
	FirecrawlToolName:          SearchNetworkToolType,
	TraversaalToolName:         SearchNetworkToolType,
	PerplexityToolName:         SearchNetworkToolType,
	SearxngToolName:            SearchNetworkToolType,
	SploitusToolName:           SearchNetworkToolType,
	WebSearchToolName:          SearchNetworkToolType,
	SearchInMemoryToolName:     SearchVectorDbToolType,
	SearchGuideToolName:        SearchVectorDbToolType,
	StoreGuideToolName:         StoreVectorDbToolType,
	SearchAnswerToolName:       SearchVectorDbToolType,
	StoreAnswerToolName:        StoreVectorDbToolType,
	SearchCodeToolName:         SearchVectorDbToolType,
	StoreCodeToolName:          StoreVectorDbToolType,
	GraphitiSearchToolName:     SearchVectorDbToolType,
	ReportResultToolName:       StoreAgentResultToolType,
	SubtaskListToolName:        StoreAgentResultToolType,
	SubtaskPatchToolName:       StoreAgentResultToolType,
	TerminalToolName:           EnvironmentToolType,
	FileToolName:               EnvironmentToolType,
	GetFlowStatusToolName:      EnvironmentToolType,
	StopFlowToolName:           EnvironmentToolType,
	SubmitFlowInputToolName:    EnvironmentToolType,
	PatchFlowSubtasksToolName:  EnvironmentToolType,
	WaitFlowCompletionToolName: EnvironmentToolType,
	OpsSystemToolName:          EnvironmentToolType,
	OpsContainersToolName:      EnvironmentToolType,
	OpsLogsToolName:            EnvironmentToolType,
	OpsBaselineToolName:        EnvironmentToolType,
	OpsDiffToolName:            EnvironmentToolType,
	OpsExecToolName:            EnvironmentToolType,
	OpsWatchToolName:           EnvironmentToolType,
	OpsSandboxToolName:         EnvironmentToolType,
	OpsTraceToolName:           EnvironmentToolType,
	PromQueryToolName:          EnvironmentToolType,
	K8sGetToolName:             EnvironmentToolType,
	RecordFindingToolName:      StoreAgentResultToolType,
	DiagnosisToolName:          BarrierToolType,
	TriageResultToolName:       StoreAgentResultToolType,
	LogFindingToolName:         StoreAgentResultToolType,
	TriageToolName:             StoreAgentResultToolType,
	LogAnalystToolName:         StoreAgentResultToolType,
	MetricsAnalystToolName:     StoreAgentResultToolType,
	ChangeAnalystToolName:      StoreAgentResultToolType,
	MetricFindingToolName:      StoreAgentResultToolType,
	ChangeFindingToolName:      StoreAgentResultToolType,
	OptimizationResultToolName: BarrierToolType,
}

var reflector = &jsonschema.Reflector{
	DoNotReference: true,
	ExpandedStruct: true,
}

// A rendered browser page routinely exceeds the result size limit; a tool off this list hands it over whole.
var allowedSummarizingToolsResult = []string{
	TerminalToolName,
	BrowserToolName,
}

// Results later subtasks recall from memory; losing the search agent's findings degrades them silently.
var allowedStoringInMemoryTools = []string{
	TerminalToolName,
	FileToolName,
	GoogleToolName,
	DuckDuckGoToolName,
	TavilyToolName,
	FirecrawlToolName,
	TraversaalToolName,
	PerplexityToolName,
	SearxngToolName,
	SploitusToolName,
	WebSearchToolName,
}

var registryDefinitions = map[string]llms.FunctionDefinition{
	TerminalToolName: {
		Name: TerminalToolName,
		Description: "Calls a terminal command in blocking mode. " +
			"Use timeout=0 or a negative value to apply the configured server default timeout. " +
			"Explicit positive values are accepted up to 10800 seconds (3 hours); values outside this range are replaced by the server default. " +
			"Only one command can be executed at a time",
		Parameters: reflector.Reflect(&TerminalAction{}),
	},
	FileToolName: {
		Name: FileToolName,
		Description: "Reads, writes, or edits local files (see 'action'). " +
			"Prefer edit_file for targeted changes to an existing file; use write_file only for a new file or a full rewrite.",
		Parameters: reflector.Reflect(&FileAction{}),
	},
	ReportResultToolName: {
		Name:        ReportResultToolName,
		Description: "Send the report result to the user with execution status and description",
		Parameters:  reflector.Reflect(&TaskResult{}),
	},
	SubtaskListToolName: {
		Name:        SubtaskListToolName,
		Description: "Send new generated subtask list to the user",
		Parameters:  reflector.Reflect(&SubtaskList{}),
	},
	SubtaskPatchToolName: {
		Name: SubtaskPatchToolName,
		Description: "Submit delta operations to modify the current subtask list instead of regenerating all subtasks. " +
			"Supports add (create new subtask at position), remove (delete by ID), modify (update title/description), " +
			"and reorder (move to different position) operations. Use empty operations array if no changes needed.",
		Parameters: reflector.Reflect(&SubtaskPatch{}),
	},
	BrowserToolName: {
		Name:        BrowserToolName,
		Description: "Opens a browser to look for additional information from the web site",
		Parameters:  reflector.Reflect(&Browser{}),
	},
	GoogleToolName: {
		Name: GoogleToolName,
		Description: "Search in the google search engine, it's a fast query and the shortest content " +
			"to check some information or collect public links by short query",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	DuckDuckGoToolName: {
		Name: DuckDuckGoToolName,
		Description: "Search in the duckduckgo search engine, it's a anonymous query and returns a small content " +
			"to check some information from different sources or collect public links by short query",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	TavilyToolName: {
		Name: TavilyToolName,
		Description: "Search in the tavily search engine, it's a more complex query and more detailed content " +
			"with answer by query and detailed information from the web sites",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	FirecrawlToolName: {
		Name: FirecrawlToolName,
		Description: "Search in the firecrawl search engine, it combines web search with page scraping to return " +
			"the main-content markdown for each result, ideal for deep research on complex technical topics " +
			"and reading documentation directly from the discovered web sites",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	TraversaalToolName: {
		Name: TraversaalToolName,
		Description: "Search in the traversaal search engine, presents you answer and web-links " +
			"by your query according to relevant information from the web sites",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	PerplexityToolName: {
		Name: PerplexityToolName,
		Description: "Search in the perplexity search engine, it's a fully complex query and detailed research report " +
			"with answer by query and detailed information from the web sites and other sources augmented by the LLM",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	SearxngToolName: {
		Name: SearxngToolName,
		Description: "Search in the searxng meta search engine, it's a privacy-focused search engine " +
			"that aggregates results from multiple search engines with customizable categories, " +
			"language settings, and safety filters",
		Parameters: reflector.Reflect(&SearchAction{}),
	},
	SploitusToolName: {
		Name: SploitusToolName,
		Description: "Search the Sploitus exploit aggregator (https://sploitus.com) for public exploits, " +
			"proof-of-concept code, and offensive security tools. Sploitus indexes ExploitDB, Packet Storm, " +
			"GitHub Security Advisories, and many other sources. Use this tool to find exploit code and PoCs " +
			"for specific software, services, CVEs, or vulnerability classes (e.g. 'ssh', 'apache log4j', " +
			"'CVE-2021-44228'). Returns exploit URLs, CVSS scores, CVE references, and publication dates.",
		Parameters: reflector.Reflect(&SploitusAction{}),
	},
	WebSearchToolName: {
		Name: WebSearchToolName,
		Description: "Search the web through a unified engine. Provide a `query` and a `mode`: " +
			"`links` for a quick list of source links with snippets, `answer` for a synthesized answer over " +
			"live sources (default), `research` for deep multi-source analysis with reasoning, or `exploit` for " +
			"exploit code, PoCs, and offensive tooling. The tool selects the best available search provider for " +
			"that mode, retries transient failures, and automatically falls back to alternative providers, so you " +
			"do NOT choose or name a specific engine. Queries must be short, keyword-focused, and in English.",
		Parameters: reflector.Reflect(&WebSearchAction{}),
	},
	SearchInMemoryToolName: {
		Name: SearchInMemoryToolName,
		Description: "Search in the vector database (long-term memory) for relevant information by providing one or more semantically rich, " +
			"context-aware natural language queries (1 to 5 queries). Formulate each query with sufficient context, intent, and detailed descriptions " +
			"to enhance semantic matching and retrieval accuracy. Multiple queries allow exploring different semantic angles and improve recall. " +
			"Results from all queries are merged, deduplicated, and ranked by relevance score. This function is ideal when you need to retrieve specific information " +
			"to assist in generating accurate and informative responses. If Task ID or Subtask ID are known, " +
			"they can be used as strict filters to further refine the search results and improve relevancy.",
		Parameters: reflector.Reflect(&SearchInMemoryAction{}),
	},
	SearchGuideToolName: {
		Name: SearchGuideToolName,
		Description: "Search in the vector database for relevant guides by providing one or more semantically rich, context-aware natural language queries (1 to 5 queries). " +
			"Formulate each query with sufficient context, intent, and detailed descriptions of the guides you need to enhance semantic matching and " +
			"retrieval accuracy. Multiple queries allow exploring different aspects of the guide topic and improve search coverage. " +
			"Specify the type of guide required to further refine the search. Results from all queries are merged, deduplicated, and ranked by relevance score. " +
			"This function is ideal when you need to retrieve specific guides to assist in accomplishing tasks or solving issues.",
		Parameters: reflector.Reflect(&SearchGuideAction{}),
	},
	StoreGuideToolName: {
		Name: StoreGuideToolName,
		Description: "Store the guide to the vector database for future use. " +
			"Anonymize all sensitive data (IPs, domains, credentials, paths) using descriptive placeholders",
		Parameters: reflector.Reflect(&StoreGuideAction{}),
	},
	SearchAnswerToolName: {
		Name: SearchAnswerToolName,
		Description: "Search in the vector database for relevant answers by providing one or more semantically rich, context-aware natural language queries (1 to 5 queries). " +
			"Formulate each query with sufficient context, intent, and detailed descriptions of what you want to find and why you need it " +
			"to enhance semantic matching and retrieval accuracy. Multiple queries allow exploring different formulations and improve search coverage. " +
			"Specify the type of answer required to further refine the search. Results from all queries are merged, deduplicated, and ranked by relevance score. " +
			"This function is ideal when you need to retrieve specific answers to assist in tasks, solve issues, or answer questions.",
		Parameters: reflector.Reflect(&SearchAnswerAction{}),
	},
	StoreAnswerToolName: {
		Name: StoreAnswerToolName,
		Description: "Store the question answer to the vector database for future use. " +
			"Anonymize all sensitive data (IPs, domains, credentials) using descriptive placeholders",
		Parameters: reflector.Reflect(&StoreAnswerAction{}),
	},
	SearchCodeToolName: {
		Name: SearchCodeToolName,
		Description: "Search in the vector database for relevant code samples by providing one or more semantically rich, context-aware natural language queries (1 to 5 queries). " +
			"Formulate each query with sufficient context, intent, and detailed descriptions of what you want to achieve with the code and what should be included, " +
			"to enhance semantic matching and retrieval accuracy. Multiple queries allow exploring different code patterns and use cases. " +
			"Specify the programming language to further refine the search. Results from all queries are merged, deduplicated, and ranked by relevance score. " +
			"This function is ideal when you need to retrieve specific code examples to assist in development tasks or solve programming issues.",
		Parameters: reflector.Reflect(&SearchCodeAction{}),
	},
	StoreCodeToolName: {
		Name: StoreCodeToolName,
		Description: "Store the code sample to the vector database for future use. It's should be a sample like a one source code file for some question. " +
			"Anonymize all sensitive data (IPs, domains, credentials, API keys) using descriptive placeholders",
		Parameters: reflector.Reflect(&StoreCodeAction{}),
	},
	GraphitiSearchToolName: {
		Name: GraphitiSearchToolName,
		Description: "Search the Graphiti temporal knowledge graph for historical penetration testing context, " +
			"including previous agent responses, tool execution records, discovered entities, and their relationships. " +
			"Supports 7 search types: temporal_window (time-bounded search), entity_relationships (graph traversal from an entity), " +
			"diverse_results (anti-redundancy search), episode_context (full agent reasoning and tool outputs), " +
			"successful_tools (proven techniques), recent_context (latest findings), and entity_by_label (type-specific entity search). " +
			"Use this to avoid repeating failed approaches, reuse successful exploitation techniques, understand entity relationships, " +
			"and build on previous findings within the same penetration testing engagement.",
		Parameters: reflector.Reflect(&GraphitiSearchAction{}),
	},
	AskUserToolName: {
		Name:        AskUserToolName,
		Description: "If you need to ask user for input, use this tool",
		Parameters:  reflector.Reflect(&AskUser{}),
	},
	FinalyToolName: {
		Name:        FinalyToolName,
		Description: "If you need to finish the task with success or failure, use this tool",
		Parameters:  reflector.Reflect(&Done{}),
	},
	GetFlowStatusToolName: {
		Name: GetFlowStatusToolName,
		Description: "Return structured information about the automation flow. " +
			"Choose the detail level based on what you need to know:\n" +
			"  'summary'  — flow health snapshot: status, task counts, subtask counts, active task/subtask IDs. Best first call.\n" +
			"  'tasks'    — all tasks with ID, status, title. Add verbose=true for inputs and results.\n" +
			"  'subtasks' — all subtasks, optionally filtered by task_id. Add verbose=true for descriptions and results.\n" +
			"  'running'  — full Task→Subtask execution chain: task input, subtask description, partial result, and recent agent messages (10 default, 50 with verbose=true). Use when you need to understand what is happening right now.\n" +
			"  'planned'  — subtasks with status 'created' (not yet started), optionally filtered by task_id. Add verbose=true for full descriptions.",
		Parameters: reflector.Reflect(&GetFlowStatusAction{}),
	},
	StopFlowToolName: {
		Name: StopFlowToolName,
		Description: "Cancel the currently executing automation task. " +
			"If a task is actively running its execution context is cancelled; the flow transitions to 'waiting' state and can accept new input or subtask patches. " +
			"If no task is running the tool returns an informational message and takes no further action. " +
			"After calling this tool, verify the flow reached 'waiting' state before making further changes.",
		Parameters: reflector.Reflect(&StopFlowAction{}),
	},
	SubmitFlowInputToolName: {
		Name: SubmitFlowInputToolName,
		Description: "Deliver text to the automation flow. The behaviour depends on the current flow state:\n" +
			"  Subtask at 'ask' checkpoint — the text is delivered as the user's answer and the subtask resumes immediately.\n" +
			"  Flow waiting with no active subtask — the text becomes the goal for a new task; the generator and refiner agents decompose it into a subtask list. Write a complete, detailed description with all context the automation needs.\n" +
			"Returns an error if a task is currently running — cancel the running task first.",
		Parameters: reflector.Reflect(&SubmitFlowInputAction{}),
	},
	PatchFlowSubtasksToolName: {
		Name: PatchFlowSubtasksToolName,
		Description: "Replace the planned (not-yet-started) subtask list for a specific task using delta operations. " +
			"Supported operations: add (insert a new subtask at a position), remove (delete a planned subtask by ID), modify (update title or description), reorder (move to a different position). " +
			"The currently executing or waiting subtask can also be targeted by including its ID in an operation; doing so resets it to 'created' status. " +
			"After patching, submit new input to trigger execution of the updated plan. " +
			"Returns an error if a task is currently running — cancel it first.",
		Parameters: reflector.Reflect(&PatchFlowSubtasksAction{}),
	},
	WaitFlowCompletionToolName: {
		Name: WaitFlowCompletionToolName,
		Description: "Block until the currently running automation task completes (or the timeout expires). " +
			"Use this when you need to wait for the automation to finish before inspecting its results or taking further action. " +
			"Returns immediately with an informational message if no tasks exist or if no task is currently running — " +
			"use " + GetFlowStatusToolName + " first to confirm the flow state before calling this tool. " +
			"On return, always call " + GetFlowStatusToolName + " with detail='summary' to inspect the final status.",
		Parameters: reflector.Reflect(&WaitFlowCompletionAction{}),
	},
	OpsSystemToolName: {
		Name: OpsSystemToolName,
		Description: "以只读方式采集目标主机的系统快照：主机名、运行时长、负载、CPU 使用率、内存与 Swap、文件系统使用率、CPU 占用最高的进程。" +
			"用于判断是否存在资源饱和、内存泄漏、磁盘写满或热点进程。" +
			"该工具只读取数据，不会修改目标主机上的任何状态。",
		Parameters: reflector.Reflect(&OpsTargetArgs{}),
	},
	OpsContainersToolName: {
		Name: OpsContainersToolName,
		Description: "列出目标主机上的全部容器及其状态、重启次数、退出码与镜像，并附带处于 failed 状态的 systemd 单元。" +
			"重启次数持续增长说明容器在崩溃重启；退出码 137 通常表示被 OOM 杀死，退出码 1 通常表示启动配置错误。" +
			"该工具只读取数据，不会重启或停止任何容器。",
		Parameters: reflector.Reflect(&OpsTargetArgs{}),
	},
	OpsLogsToolName: {
		Name: OpsLogsToolName,
		Description: "读取目标主机的日志尾部。source 填容器名时读取该容器的 docker logs；source 留空时读取最近 30 分钟的系统错误日志（journalctl -p err）。" +
			"日志通常是定位根因最直接的证据，优先用它确认报错文本与时间点。",
		Parameters: reflector.Reflect(&OpsLogsArgs{}),
	},
	OpsBaselineToolName: {
		Name: OpsBaselineToolName,
		Description: "对目标主机采集一次带时间戳的基线指纹，返回一段 JSON。用于性能优化类目标：变更前采一次、变更后再采一次，然后用 ops_diff 对比。" +
			"指纹包含负载、CPU、内存、Swap、各分区使用率、容器状态与重启次数、CPU 与内存占用最高的进程。" +
			"该工具只读取数据，不修改目标主机。",
		Parameters: reflector.Reflect(&OpsBaselineArgs{}),
	},
	OpsDiffToolName: {
		Name: OpsDiffToolName,
		Description: "对比两次 ops_baseline 采集的指纹，逐项给出变化量、百分比与「改善/恶化/持平」判定，并标注变化是否显著。" +
			"差值与百分比由代码计算，不要自己心算：直接把两次指纹原样传入即可。" +
			"没有显著变化时结论应当是「本次变更未产生可测量效果」，而不是勉强找一条改善。",
		Parameters: reflector.Reflect(&OpsDiffArgs{}),
	},
	OpsExecToolName: {
		Name: OpsExecToolName,
		Description: "按名称执行一条预置的只读诊断命令，用于其他取证工具没覆盖到的角度。command 只能取下列 ID 之一，命令文本本身来自代码里的目录，不接受自由命令：" +
			"listen_ports（监听端口与进程）、established（已建立连接）、disk_usage（容量）、inode_usage（inode）、memory（内存与 Swap）、load（平均负载与核数）、" +
			"psi（PSI 压力停顿，CPU / IO / 内存各自被拖住多少）、" +
			"top_cpu / top_mem（按 CPU 或内存排序的进程，可传 lines）、unit_status / unit_logs（systemd 单元状态与日志，需 unit）、kernel_log（内核日志，可传 lines）、" +
			"docker_inspect（容器状态、退出码、OOMKilled、重启次数，需 container）、docker_stats（一次性资源采样）、file_stat（绝对路径的元数据，需 path）、" +
			"package_history（最近的软件包安装与升级，可传 lines，用来对齐变更时间线）、unit_activated_at（systemd 单元的激活与启动时刻，需 unit）、" +
			"container_image（容器所用镜像与创建时刻，需 container）、image_history（镜像各层的构建命令与时刻，需 image，可传 lines）、" +
			"pg_stat_activity（Postgres 容器里还在跑的查询，需 container 与 database）、pg_stat_statements（Postgres 里累计耗时最高的语句，需 container 与 database，可传 lines）、" +
			"pg_locks（此刻的锁等待：谁被谁挡住、等了多少秒、卡在哪把未授予的锁上，需 container 与 database，可传 lines）、" +
			"pg_explain（累计耗时第 N 条语句的执行计划：只取计划、不执行，需 container、database 与 rank，rank 从 1 起）。" +
			"参数只允许字母、数字与 . _ - /，含空格或 shell 元字符会被拒绝。" +
			"返回结果会带上真实执行的命令原文，写证据时直接引用它。",
		Parameters: reflector.Reflect(&OpsExecArgs{}),
	},
	OpsWatchToolName: {
		Name: OpsWatchToolName,
		Description: "把一条只读命令在短时间内重复采样，返回带时间戳的序列——用来抓“正在发生”的东西：连接数、进程数、OOM 计数、负载、队列深度。" +
			"command 与 ops_exec 用同一套目录 ID；samples 取 2–30，interval_seconds 取 1–30，窗口最长 300 秒。" +
			"它比两次离散快照有用，是因为拐点、锯齿和“某个值在窗口里持续上升”只有在序列里才看得出来；写 evidence 时引用序列里真正变化的那几行。" +
			"要采样宿主上看不到的东西（某个容器内部的计数、短命进程），用 ops_sandbox 在里面写循环，而不是这个工具。",
		Parameters: reflector.Reflect(&OpsWatchArgs{}),
	},
	OpsSandboxToolName: {
		Name: OpsSandboxToolName,
		Description: "在目标主机上起一个一次性采集容器，在里面执行你写的只读脚本——这是你在目标机上唯一的自由命令通道。" +
			"容器的形状由代码固定：只读根文件系统、无网络、丢弃全部 capability、不加入宿主机 PID 命名空间、256MB / 1 CPU / 256 进程上限；" +
			"宿主机的 /proc 与 /sys 以只读方式挂在 /host/proc 与 /host/sys。" +
			"因此你可以读任何东西，改不了任何东西：写文件会失败，kill 宿主进程会失败（那些 PID 不在容器里），联网会失败。" +
			"看宿主机进程要用 /host/proc/<pid>，看宿主机监听端口要读 /host/proc/1/net/tcp（容器自己的网络命名空间是空的，ss 只能看到空的）。" +
			"image 由你决定：要装工具就选一个有包管理器的（debian/ubuntu 用 apt、alpine 用 apk），留空用 debian:latest；拉镜像会写目标机，操作员没放开（OPSAGI_SANDBOX_PULL）时只能用目标机上已有的镜像，报错会列出它们。" +
			"这个形状里跑不了 eBPF 或 perf：它们要的能力被丢掉了，那是 ops_trace 的事。需要联网取证时，用 ask 请操作员批准。",
		Parameters: reflector.Reflect(&OpsSandboxArgs{}),
	},
	OpsTraceToolName: {
		Name: OpsTraceToolName,
		Description: "在目标主机上跑一段 eBPF 程序，看 /proc 快照看不到的那一层：调度延迟、off-CPU 阻塞、块设备 IO 延迟、进程创建、TCP 建连延迟。" +
			"script 是固定目录里的 ID（" + strings.Join(traceIDs(), "、") + "），程序正文在代码里，你选的是问题不是代码。" +
			"seconds 是采样窗口，取 3 到 30 秒，默认 10；窗口越长直方图越满，但也越贵。" +
			"容器形状由代码固定：先丢光能力再加回 BPF / PERFMON / SYS_RESOURCE，宿主机的 /sys/kernel 只读挂载，不进宿主机 PID 命名空间，无网络。" +
			"如果内核因为缺 SYS_ADMIN 拒绝 perf_event_open，会自动补上这一个能力重试一次（第一次永远是最窄的那个形状）——这一步失败发生在采样窗口打开之前，代价只是一次启动。" +
			"镜像里必须有 bpftrace，默认用 quay.io/iovisor/bpftrace。" +
			"它是采样不是监控，一秒钟的结论不要写成五分钟的结论；写 evidence 时把窗口和直方图的峰值一起写进去。" +
			"这个工具只在操作员打开内核追踪（OPSAGI_SANDBOX_BPF）时出现：BPF 要的能力比采集容器多，那条边界由他们划。",
		Parameters: reflector.Reflect(&OpsTraceArgs{}),
	},
	K8sGetToolName: {
		Name: K8sGetToolName,
		Description: "从集群里读一类只读对象：pods（含重启次数与卡住的原因）、events（控制面说过的话）、deployments、nodes。" +
			"k8s 里的故障证据几乎都在 events 和 pod 的 waiting 原因里，先读这两样再下结论。" +
			"读的是配置好的那个集群，不经过目标主机的 shell；没有配集群时这个工具不会出现。",
		Parameters: reflector.Reflect(&K8sGetArgs{}),
	},
	PromQueryToolName: {
		Name: PromQueryToolName,
		Description: "对指标后端执行一条 PromQL 范围查询，返回每条序列的首值、末值、极值与「最大跳变」的时刻。" +
			"这是唯一能看到指标随时间怎么变的工具：ops_baseline 只给一瞬间的快照，判断「拐点在哪一刻」必须用这里的跳变行。" +
			"窗口默认往回 15m，用 range 调大（1m~24h）；step 决定采样密度。查询走 HTTP，不经过目标主机的 shell。" +
			"返回「窗口内没有数据」说明选择器或采集有问题，换更宽的窗口或去掉标签再试，不要当作结论。",
		Parameters: reflector.Reflect(&PromQueryArgs{}),
	},
	RecordFindingToolName: {
		Name: RecordFindingToolName,
		Description: "把一条结论写进这条事故的记录里：一眼能读的标题、它意味着什么、以及支撑它的原始输出。" +
			"聊天记录会滚走，这条不会——用户之后回看事故、事故指挥接手时读的都是它。" +
			"只在探针真的查过之后写；evidence 直接粘贴工具返回里的命令行与那几行输出，不要改写、不要心算。" +
			"同一条结论只写一次：重复写不会让它更可信，只会让时间线更难读。",
		Parameters: reflector.Reflect(&RecordFindingArgs{}),
	},
	DiagnosisToolName: {
		Name: DiagnosisToolName,
		Description: "提交本次事故的最终诊断，并结束当前排查。这是唯一的正常出口。" +
			"调用之前结论必须已经被工具真实取证过：evidence 至少两条，每条都要能靠 query 复现。" +
			"没查过的东西不要写进结论；结论不完整或需要人动手时把 needs_human 置为 true。",
		Parameters: reflector.Reflect(&Diagnosis{}),
	},
	TriageResultToolName: {
		Name: TriageResultToolName,
		Description: "交回分诊结论：分级、影响面、是否命中已知故障模式。" +
			"result 是完整技术报告，事故指挥只看这一段，写短了这次委派就白做了。",
		Parameters: reflector.Reflect(&TriageResult{}),
	},
	LogFindingToolName: {
		Name: LogFindingToolName,
		Description: "交回日志分析结论：错误模式、首次出现时刻、与告警的先后关系。" +
			"pattern 要写成能直接匹配日志原文的特征，不要写「报错很多」。",
		Parameters: reflector.Reflect(&LogFinding{}),
	},
	MetricFindingToolName: {
		Name: MetricFindingToolName,
		Description: "交回指标分析结论：哪些指标、拐点时刻、拐点前后的数值变化。" +
			"onset 是数值开始异常的那一刻，不是发现异常的时刻。",
		Parameters: reflector.Reflect(&MetricFinding{}),
	},
	TriageToolName: {
		Name: TriageToolName,
		Description: "把定级与影响面的判断交给分诊员。" +
			"它只回答三个问题：多严重、影响谁、像不像见过的某个故障。" +
			"question 要写清告警原文与已知现象，不要只写「看看严重吗」。",
		Parameters: reflector.Reflect(&AnalystQuestion{}),
	},
	LogAnalystToolName: {
		Name: LogAnalystToolName,
		Description: "把日志面的取证交给日志分析员。" +
			"它会在事故时间窗内找第一个错误模板、首次出现时刻、实例分布，以及之前十秒发生了什么。" +
			"question 里带上受影响容器名与时间窗。",
		Parameters: reflector.Reflect(&AnalystQuestion{}),
	},
	MetricsAnalystToolName: {
		Name: MetricsAnalystToolName,
		Description: "把指标面的取证交给指标分析员。" +
			"它找拐点而不是均值：哪条序列先动、动多少、哪些序列没动。" +
			"question 里带上受影响的指标面（主机负载、内存、磁盘、容器资源）与时间窗。",
		Parameters: reflector.Reflect(&AnalystQuestion{}),
	},
	ChangeAnalystToolName: {
		Name: ChangeAnalystToolName,
		Description: "把变更面的取证交给变更分析员。" +
			"它构建事故起始点前后的变更时间线，并算出每条变更离起始点多远。" +
			"question 里带上起始点时间与受影响组件。",
		Parameters: reflector.Reflect(&AnalystQuestion{}),
	},
	OptimizationResultToolName: {
		Name: OptimizationResultToolName,
		Description: "提交性能优化的最终结论，并结束当前任务。这是性能类目标唯一的正常出口。" +
			"baseline 必须是变更前真实采集到的数值，observed_gain 必须是变更后复测到的数值。" +
			"测过但没有变化，就把 verdict 写 no_measurable_effect —— 这是一个合法结论，不要为了交差编造改善。",
		Parameters: reflector.Reflect(&OptimizationResult{}),
	},
	ChangeFindingToolName: {
		Name: ChangeFindingToolName,
		Description: "交回变更分析结论：谁在什么时候改了什么、变更什么时候生效、与告警的间隔。" +
			"只报时间先后时明说只有相关性，不要断言因果。",
		Parameters: reflector.Reflect(&ChangeFinding{}),
	},
}

func getMessageType(name string) database.MsglogType {
	switch name {
	case TerminalToolName:
		return database.MsglogTypeTerminal
	case FileToolName:
		return database.MsglogTypeFile
	case BrowserToolName:
		return database.MsglogTypeBrowser
	case GoogleToolName, DuckDuckGoToolName, TavilyToolName, FirecrawlToolName,
		TraversaalToolName, PerplexityToolName, SearxngToolName, SploitusToolName, WebSearchToolName,
		SearchGuideToolName, SearchAnswerToolName, SearchCodeToolName, SearchInMemoryToolName, GraphitiSearchToolName:
		return database.MsglogTypeSearch
	case AskUserToolName:
		return database.MsglogTypeAsk
	case FinalyToolName:
		return database.MsglogTypeDone
	case DiagnosisToolName, OptimizationResultToolName:
		return database.MsglogTypeDone
	case RecordFindingToolName:
		return database.MsglogTypeAdvice
	default:
		return database.MsglogTypeThoughts
	}
}

func getMessageResultFormat(name string) database.MsglogResultFormat {
	switch name {
	case TerminalToolName:
		return database.MsglogResultFormatTerminal
	case FileToolName, BrowserToolName:
		return database.MsglogResultFormatPlain
	default:
		return database.MsglogResultFormatMarkdown
	}
}

// GetRegistryDefinitions returns tool definitions from the tools package
func GetRegistryDefinitions() map[string]llms.FunctionDefinition {
	registry := make(map[string]llms.FunctionDefinition, len(registryDefinitions))
	maps.Copy(registry, registryDefinitions)
	return registry
}

// GetToolTypeMapping returns a mapping from tool names to tool types
func GetToolTypeMapping() map[string]ToolType {
	mapping := make(map[string]ToolType, len(toolsTypeMapping))
	maps.Copy(mapping, toolsTypeMapping)
	return mapping
}

// GetToolsByType returns a mapping from tool types to a list of tool names
func GetToolsByType() map[ToolType][]string {
	result := make(map[ToolType][]string)

	for toolName, toolType := range toolsTypeMapping {
		result[toolType] = append(result[toolType], toolName)
	}

	return result
}
