# Prompt Engineering in OpsAGI

This guide is for whoever edits `backend/pkg/templates/prompts/*.tmpl`. It covers what the prompt layer guarantees, which template serves which chain, which variables a template may use, and the rules the shipped prompts follow — including the ones that exist because a prompt once said something the code did not do.

## 1. What the layer guarantees

Each `PromptType` in `backend/pkg/templates/templates.go` names one template file, one set of declared variables, and one render call site. The render is plain `text/template`: a template that uses a variable it did not declare renders as `<no value>` rather than failing, which is why the validator exists.

`RenderPrompt` is the only entry point. It parses the template, executes it against the caller's data and returns the string the provider puts in front of the model. A template that cannot parse fails the chain that asked for it, not the boot.

The templates are embedded with `//go:embed prompts/*.tmpl`. The embed does not recurse, so every prompt lives directly in `prompts/`, and a subdirectory makes the file invisible to the binary at run time while still being visible in git.

## 2. Which template serves which chain

| Template | Prompt type | Rendered by |
|---|---|---|
| `commander.tmpl` | `PromptTypeCommander` | `pkg/providers` before a subtask chain |
| `triage.tmpl`, `log_analyst.tmpl`, `metrics_analyst.tmpl`, `change_analyst.tmpl` | one per analyst | the delegation handler |
| `reporter.tmpl` | `PromptTypeReporter` | the task result reporter |
| `generator.tmpl`, `refiner.tmpl` | planning | the generator and refiner calls |
| `subtasks_generator.tmpl`, `subtasks_refiner.tmpl`, `task_reporter.tmpl` | planning data | the user-message half of a two-part prompt |
| `reflector.tmpl`, `question_reflector.tmpl` | correction | the reflector |
| `summarizer.tmpl`, `full_execution_context.tmpl`, `short_execution_context.tmpl`, `execution_logs.tmpl`, `flow_descriptor.tmpl`, `task_descriptor.tmpl` | context | `pkg/csum` and the context builders |
| `toolcall_fixer.tmpl`, `input_toolcall_fixer.tmpl`, `tool_call_id_detector.tmpl`, `tool_call_id_collector.tmpl` | tool-call repair | the fixer and the id extractor |
| `assistant.tmpl` | `PromptTypeAssistant` | the assistant provider |
| `language_chooser.tmpl` | `PromptTypeLanguageChooser` | flow creation, before anything is planned |
| `image_chooser.tmpl` | `PromptTypeImageChooser` | flow creation, when the image is not fixed |

The generator and refiner prompts come in two parts: the system prompt carries the role and the rules, and the second template carries the data (`Task`, `Tasks`, `Subtasks`, …) as the user message. Keeping the data out of the system prompt keeps the system half stable.

## 3. Variables

A template may only use the variables `PromptVariables` declares for its type. Grouped by what they carry:

**Language and time.** `Lang`, `CurrentTime`. `Lang` holds an English language name (`Chinese`, `English`, `Russian`); the prompts never compare it, they only print it into a sentence that says what language to answer in.

**Tools.** Every tool the prompt names is a variable, never a literal: `DiagnosisToolName`, `OptimizationToolName`, `TriageToolName`, `LogAnalystToolName`, `MetricsAnalystToolName`, `ChangeAnalystToolName`, `SubtaskListToolName`, `SubtaskPatchToolName`, `ReportResultToolName`, `AskToolName`, `WatchToolName`, `SandboxToolName`, `TraceToolName`, `TerminalToolName`, `FileToolName`, `WebSearchToolName`, the `Search*`/`Store*` memory tools, and the flow-manager tools. Renaming a tool in `registry.go` therefore changes the prompt text without touching a template.

**Gates.** `AskUserEnabled`, `TraceEnabled`, `FlowManagerEnabled`, and `OptimizationMode` decide which conditional blocks render. A block whose gate is false must not be the only place a mandatory instruction lives.

**Environment and context.** `DockerImage`, `Cwd`, `ContainerPorts`, `DefaultImage`, `UserFiles`, `ExecutionContext`, `ExecutionLogs`, `ExecutionState`, `CompletedSubtasks`, `PlannedSubtasks`, `Subtasks`, `Tasks`, `Task`, `Subtask`, `Input`, `Request`, `Message`, `N`.

**Summarization markers.** `SummarizationToolName` and `SummarizedContentPrefix` let a prompt recognise a compacted transcript instead of mistaking it for a user message.

**Tool-call repair.** `ToolCallName`, `ToolCallArgs`, `ToolCallSchema`, `ToolCallError`, `FunctionName`, `Samples`, `PreviousAttempts`, `RandomContext`.

Adding a variable to a template without declaring it in `PromptVariables` fails the validator, and it fails the save of a user-edited prompt as well.

## 4. The rules the shipped prompts follow

**State the fact, not the tool's interface.** The tool schemas travel beside the prompt as function definitions, so the model already sees the argument names, the enums and the required fields. A prompt that repeats them costs tokens and drifts — the two get out of step, and the schema is the one that wins. What a prompt must say is the part the schema cannot: when to use this tool instead of that one, what the result is not, and what the agent must do before calling it. `{{.ToolPlaceholder}}` is the one-line statement that the answer must be a tool call, not an interface description.

**Never hardcode a number the code owns.** Timeouts, iteration limits, the number of subtasks and the retry budget are configuration. A prompt that says "you have 1200 seconds" is wrong the moment someone sets `TERMINAL_TOOL_TIMEOUT`; the tool schema carries the configured value, and the prompt says what to do when the budget runs out.

**One line per paragraph.** Inside a template, a paragraph is one physical line. Wrapped prose renders with the newlines intact and the model reads a wall of broken sentences. The same rule holds for `.md` files in this repository.

**The technical channel stays in English.** Commands, tool names, file paths, log excerpts and JSON keys are English in every language. `{{.Lang}}` governs the prose that explains them, not the evidence itself. A translated command is a broken command.

**No first person, no persona furniture.** The prompts describe the job and the constraints. They do not thank the model, praise its reasoning, or narrate what it is about to do.

**No archaeology.** A prompt says what is true now. "It used to be", "no longer", "before this change" belong in git history; in a prompt they cost tokens and mislead.

**Every claim about behaviour must match the executor.** If the prompt says a tool exists, the executor for that chain must offer it; if the prompt says a mode has one exit, the executor must advertise exactly one. A prompt that promises a tool the agent does not have produces a call to `unknownToolResponse`, and the round is wasted. The tests in `pkg/providers` and `pkg/tools` pin several of these pairs; when one fails, the prompt or the executor is wrong — decide which before editing either.

**Facts about the environment come from the environment.** The commander is told the time, the container image and the working directory from the executor's own values. A prompt that writes a fixed date or a fixed path is wrong for every deployment but the one it was written on.

## 5. Language

`language_chooser.tmpl` runs once per flow, reads the operator's input and returns an English language name. That name becomes `{{.Lang}}` for every later prompt in the flow.

The AIOps renderers in `pkg/tools/labels.go` use the same value for the parts of a result the code writes rather than the model: finding headings, separators, the plan-mode label, the diagnosis and optimization section titles. A new label goes there and nowhere else — a template that hardcodes its own heading produces a report with two languages in it.

The reporter writes the whole report in the flow's language. Everything inside it that is a command, a log line or a quoted value stays verbatim.

## 6. Editing a prompt

1. Change the `.tmpl` file under `backend/pkg/templates/prompts/`.
2. If the change adds a variable, declare it in `PromptVariables` for that prompt type and pass it at the call site; both are compile-time-visible, so a forgotten one shows up as a test failure rather than a `<no value>` in production.
3. Run `go test ./pkg/templates/... ./pkg/providers/...`. The validator parses every shipped template, checks each variable against `PromptVariables`, and renders it with the sample data in `pkg/templates/validator/testdata.go`.
4. If the prompt promises a tool, check the executor that serves that prompt still offers it. The tool-layer tests pin the executor's list; a prompt edit that assumes a new tool needs the executor change in the same commit.
5. Rebuild the binary. Prompts are embedded; restarting an old binary keeps the old prompts.

The validator's samples are also the place to add a realistic excerpt when a prompt gains a new section — a rendering test that passes only on empty data proves the template parses, not that it reads.

## 7. Anti-patterns

- **Restating the tool schema.** Costs tokens, drifts, and the schema wins in the end.
- **Two exits in one prompt.** If the prompt offers a diagnosis and an optimization conclusion to the same chain, the model will eventually file the wrong one. The commander's exit is chosen by the plan mode, and the executor advertises one.
- **Instructions that only work on one deployment.** Host names, image names and target lists come from configuration; the prompt says what to do with a target, not which target.
- **Asking the model to format what the code already formats.** The finding renderers produce their sections from the labels table; a prompt that also prescribes headings produces duplicates.
- **Editing one half of a two-part prompt.** The system half and the data half are read together; a rule that lives in one and is contradicted in the other is read as two rules.
