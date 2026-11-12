# Flow Execution in OpsAGI

This document describes how a flow runs: what the units of work are, which agent plays which part, what each agent can do, and where the limits are. It describes the code as it stands in `pkg/controller`, `pkg/providers` and `pkg/tools`; when the two disagree, the code wins.

## 1. Units of work

- **Flow** — the top-level object a user creates. It carries the flow type (`diagnosis` or `optimization`), the model provider, an optional image override, and the language the operator writes in.
- **Task** — the user's request inside a flow. A flow starts with one task built from the input; the flow-input tool can add another task to the same flow later.
- **Subtask** — one investigation step planned by the generator. Subtasks run one at a time, in order, and each one is executed by a commander chain.
- **Action** — one tool call inside an agent chain: a command, a query, a delegated analyst run.

Every subtask carries a **plan mode** in `subtasks.plan_mode`: `diagnosis` for an incident, `optimization` for a performance task. It comes from the planner's subtask entry and defaults to `diagnosis` when the planner leaves it empty; the commander reads it to decide which conclusion tool it may call. Rows written before the column existed fall back to parsing a `【目标类型：…】` prefix out of the description.

## 2. Workers

- **FlowWorker** — owns the flow lifecycle: creates the task from the user input, runs it, stops it, restores it after a restart.
- **TaskWorker** — drives one task: generate the subtask list, run the subtasks in order, refine the remaining plan after each one, then produce the report.
- **SubtaskWorker** — runs one subtask's commander chain and writes back its status, result and message chain.
- **AssistantWorker** — runs the interactive assistant attached to a flow. It shares the flow's containers and provider settings, has its own log tables, and can be asked to control the flow itself through the flow-manager tools.

Workers are created per flow and are not queued: a flow that fails to build is marked `failed` and says why on its message log.

## 3. Providers

- **FlowProvider** — the interface the workers drive: `PerformAgentChain` for a subtask, `PerformTaskResultReporter` for a task's report, plus the model and tool plumbing.
- **AssistantProvider** — the assistant's version of the same interface, with the flow-manager handlers injected after the chain is prepared.
- **ProviderController** — builds a provider from the flow's or assistant's stored configuration and hands out the embeddings client used for the memory store.

## 4. Roles

Twelve chain types exist in `MsgchainType`, and each one asks its provider for a different model configuration bucket through `agentByChain`:

| Role | Bucket | What it does |
|---|---|---|
| `commander` | `commander` | Executes one subtask: probes the environment read-only, delegates evidence to the analysts, then files the conclusion through `diagnosis` or `optimization_result`. |
| `triage` | `simple` | First look at an alert: classifies what kind of problem this is and which evidence faces are worth pulling. Returns through `triage_result`. |
| `log_analyst` | `analyst` | Reads logs: container logs, systemd units, kernel messages. Returns through `log_finding`. |
| `metrics_analyst` | `analyst` | Reads metrics and resource history. Returns through `metric_finding`. |
| `change_analyst` | `analyst` | Reads what changed and when: deploys, restarts, config, DNS and certificate timing. Returns through `change_finding`. |
| `reporter` | `simple` | Writes the task's final report from the subtask results, in the flow's language. Returns through `report_result`. |
| `generator` | `generator` | Decomposes the task into three to five ordered subtasks. Returns through `subtask_list`. |
| `refiner` | `refiner` | Re-plans the remaining subtasks after each completion. Returns through `subtask_patch`. |
| `reflector` | `reflector` | Corrects an agent that answered in prose when a tool call was required. |
| `summarizer` | `simple` | Compacts a chain that is approaching its context limit. |
| `tool_call_fixer` | `simple_json` | Repairs a tool call whose arguments do not parse. |
| `assistant` | `assistant` | Answers the operator in the assistant panel, optionally controlling the flow. |

The four analysts are not separate workers: the commander reaches each of them through a delegation tool of the same name, the analyst runs its own chain with its own message chain, and its finding comes back to the commander as the tool result.

## 5. Planning

The generator receives the task, the current time and the operator's language, and must answer through `subtask_list`. `TasksNumberLimit` is 5, so the prompt asks for three to five subtasks: enough to separate evidence faces, few enough that the verdict does not wait on a long plan.

Each planned subtask carries a title, a description, and the plan mode its entry names. The description is rendered by `PlanDescription(lang)`, which writes the plan label in the flow's language or leaves the prefix off entirely — the column, not the text, decides how the subtask runs.

After a subtask completes, the refiner receives the completed and remaining subtasks and may add, remove, modify or reorder them through `subtask_patch`. It cannot touch a completed subtask.

Subtask states are `created`, `running`, `waiting`, `completed` and `failed`. A subtask that ends `waiting` is paused at an `ask` checkpoint; `submit_flow_input` answers it. A chain that breaks for any other reason is marked `failed` rather than `waiting`, so a broken run does not look like a question waiting for an answer.

## 6. Subtask execution

`PerformAgentChain` always runs a `commander` chain, whatever the subtask is. The commander's tool list is built by `GetCommanderExecutor`:

- The conclusion tools: `diagnosis` for a diagnosis subtask, `optimization_result` for an optimization subtask. Exactly one of them is offered — an executor that advertised both would let a performance task file an incident diagnosis.
- The read-only ops probes: `ops_containers`, `ops_logs`, `ops_system`, `ops_baseline`, `ops_diff`, `ops_exec`, `ops_watch`, `ops_sandbox`, and `ops_trace` when the BPF sandbox image is available.
- `prom_query` and `k8s_get` when Prometheus and the Kubernetes API are configured.
- The four delegation tools: `triage`, `log_analyst`, `metrics_analyst`, `change_analyst`.
- `ask` when `ASK_USER` is on.

There is no terminal, no file tool and no web search in the commander's list: what an agent may do is its tool list, and the commander reads rather than changes.

The chain ends when the model calls its conclusion tool, calls `ask`, or reaches the iteration limit. The limit is `MAX_GENERAL_AGENT_TOOL_CALLS` (100) for the commander and the assistant, and `MAX_LIMITED_AGENT_TOOL_CALLS` (20) for every other role. The last three iterations are a shutdown grace period: the model is told to finish now, and a chain with no exit left is ended rather than looped.

## 7. The analysts

Each analyst gets a narrowed evidence toolset and exactly one exit:

| Delegation tool | Chain | Evidence tools | Exit |
|---|---|---|---|
| `triage` | `triage` | `ops_containers`, `ops_system`, `ops_logs`, `ops_exec`, `k8s_get` | `triage_result` |
| `log_analyst` | `log_analyst` | `ops_logs`, `ops_containers`, `k8s_get` | `log_finding` |
| `metrics_analyst` | `metrics_analyst` | `ops_system`, `ops_containers`, `prom_query`, `k8s_get` | `metric_finding` |
| `change_analyst` | `change_analyst` | `ops_containers`, `ops_system`, `ops_exec`, `k8s_get` | `change_finding` |

An evidence backend that a deployment simply does not have narrows the analyst instead of failing the chain: the tool is dropped from its list, and the analyst reports on the faces it can see. The exit tool is not optional — an analyst without it has no way to return a result.

The finding a delegated analyst returns is rendered by the delegation handler, not written by the commander, so its headings and separators follow the flow's language. The commander decides which face the next probe belongs to and delegates; it does not re-derive the analyst's evidence.

## 8. Reporting

The reporter runs once per task, after the last subtask, and returns a `TaskResult` through `report_result`. It receives the task, the completed subtasks with their results, the actions that actually ran, and the flow's language, and writes six sections: the one-sentence verdict, the root cause, the evidence with its sources and timestamps, the blast radius, what to do next (immediate and needs-confirmation), and what is unfinished.

The report is the artefact the operator reads. It is written in the flow's language; the technical channel inside the agent chains (commands, log lines, quoted output) stays verbatim.

## 9. Tools by category

Fifty tool names exist in `pkg/tools/registry.go`:

**Environment.** `terminal` (blocking command in the flow's container, `TERMINAL_TOOL_TIMEOUT`, default 1200s, hard limit 3h), `file` (read and write, absolute paths only). Uploaded files appear under `/work/uploads` and `/work/resources` and reach prompts through `{{.UserFiles}}`.

**Search.** `browser` (page fetch plus screenshot) and `web_search`, the single search entry point: the agent supplies a `query` and a `mode` (`links`, `answer`, `research`, `exploit`), and the orchestrator picks the engine, retries transient failures and falls back. The engines behind it are Google, DuckDuckGo, Tavily, Traversaal, Perplexity, Firecrawl, Searxng, Sploitus and an optional internal analytics engine; their ids are also registered as individual tools for `ftester`, and no agent is offered them directly.

**Vector store.** `search_in_memory` for execution memory, and the typed knowledge pairs `search_guide`/`store_guide`, `search_answer`/`store_answer`, `search_code`/`store_code`. `graphiti_search` reads the knowledge graph when one is configured.

**Ops probes.** `ops_system`, `ops_containers`, `ops_logs`, `ops_baseline`, `ops_diff`, `ops_exec`, `ops_watch`, `ops_sandbox`, `ops_trace`, plus `prom_query` and `k8s_get`. They run against the flow's container or against a configured remote target, and they are read-only apart from `ops_exec`'s explicit file staging.

**Conclusions and findings.** `diagnosis` and `optimization_result` are the commander's exits; `triage_result`, `log_finding`, `metric_finding` and `change_finding` are the analysts'.

**Delegation.** `triage`, `log_analyst`, `metrics_analyst` and `change_analyst` hand a subtask to an analyst chain.

**Planning.** `subtask_list` (generator), `subtask_patch` (refiner), `report_result` (reporter).

**Barriers.** `done` and `ask`, and the conclusion tools above, end a chain.

**Flow management.** `get_flow_status`, `stop_flow`, `submit_flow_input`, `patch_flow_subtasks` and `wait_flow_completion`, offered to the assistant when it has a flow worker behind it.

A tool that is not in an executor's list is not offered to the model, and a call to one lands on `unknownToolResponse`, which lists the names the model may use instead of failing the chain.

## 10. Context management

Every chain is compacted before it overflows. The summarizer receives either the full execution context or the short one, depending on what the chain already contains, and replaces the middle of the conversation with a summary that keeps the last messages and the tool-call structure intact. `csum.SummarizationToolName` is the marker left in the transcript where a segment was compacted; it is not a tool, and the executor answers a call to it with an explanation rather than an error.

Three other roles keep the loop honest:

- the **reflector** re-asks an agent that answered in prose when the chain required a tool call;
- the **tool_call_fixer** repairs arguments that do not parse, using the tool's schema;
- the **repeating detector** notices the same call repeated unchanged and nudges the agent off it.

## 11. Language

`language_chooser` reads the operator's language from the flow input and returns an English language name — `Chinese`, `English` or `Russian` in practice. `{{.Lang}}` then reaches the prompts, and the AIOps renderers in `pkg/tools/labels.go` use it to pick headings, separators and section titles for findings, diagnoses and optimizations. The reporter writes the whole report in that language; commands, log excerpts and quoted output stay verbatim.

## 12. Memory and the knowledge graph

Results that later subtasks may need are written to the pgvector store with their scope (user, flow, task, subtask) and the tool that produced them. The anonymizer runs before the write: secrets from `GetSecretPatterns()` never reach the store, the logs or the transcripts. `graphiti_search` and the per-call write in `storeToolExecutionToGraphiti` are a second, optional memory face when a Graphiti service is configured; without it the tools simply are not offered.

## 13. Human in the loop

With `ASK_USER=true` the commander and the analysts are offered `ask`. A call pauses the subtask in `waiting`; the flow stops there until input arrives through `submit_flow_input`, the API or the UI. With the flag off the tool is not offered and a chain that would have asked has to decide from the evidence it has.

The assistant can control the flow through `stop_flow`, `submit_flow_input`, `patch_flow_subtasks` and `wait_flow_completion`, which answer in the flow's own terms: `stop_flow` reports the state the flow actually reached, and `patch_flow_subtasks` returns the new subtask ids.

## 14. Logging and observability

Every chain writes its message chain to `msgchains`, each message to `msgs`, each tool call to `toolcalls`, terminal sessions to `termlogs`, vector-store traffic to `vecstorelogs`, search traffic to `searchlogs` and per-agent summaries to `agentlogs`. The logs are what the UI subscribes to and what analytics reads.

Tracing goes to Langfuse through OpenTelemetry: a chain opens an agent observation, each tool call opens a tool observation under the calling agent, and a barrier opens a span. The observation carries the flow, task and subtask ids and the tool category. A tool call that reaches the wrong handler or the wrong trace is a bug visible in the trace tree, not only in the logs.

## 15. Containers

Each flow gets a primary Docker container, chosen at flow creation: the flow's image override, then the model's choice through `image_chooser` (`DOCKER_IMAGE_SELECTION_MODE=llm`), then the configured default. The container is where `terminal`, `file` and the ops probes without a remote target run. `ops_exec` and the sandbox tools can also reach a configured remote target (`OPSAGI_TARGETS`), which is how the product inspects a host it does not sandbox.

Uploads and resources are copied into the container at the paths the prompts name; `DOCKER_ALLOWED_IMAGES` and the per-flow image override bound what can be started.

## 16. Limits and environment variables

| Variable | Default | What it bounds |
|---|---|---|
| `MAX_GENERAL_AGENT_TOOL_CALLS` | 100 | Tool calls in a commander or assistant chain |
| `MAX_LIMITED_AGENT_TOOL_CALLS` | 20 | Tool calls in any other role's chain |
| `TERMINAL_TOOL_TIMEOUT` | 1200 | Seconds a `terminal` command may run (hard limit 3h) |
| `ASK_USER` | false | Whether `ask` is offered at all |
| `AGENT_PLANNING_STEP_ENABLED` | false | Whether the planner prompts are used |
| `DOCKER_IMAGE_SELECTION_MODE` | llm | Whether the model picks the flow image |
| `DOCKER_DEFAULT_IMAGE` | `debian:latest` | Image when nothing else decides |
| `OPSAGI_TARGETS` | empty | Remote hosts the ops probes may reach |

The number of subtasks is not an environment variable: `TasksNumberLimit` in `pkg/providers/provider.go` is 5, and the generator prompt asks for three to five.
