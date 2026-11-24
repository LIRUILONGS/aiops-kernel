# Capability lease policy

`OPSAGI_POLICY` points at a YAML file that decides what a subtask's lease contains. It is read once at start-up: the file is not watched, and a change needs a restart.

Without the variable every lease comes from `capability.DefaultPolicy`, which is the scope the runtime had before per-flow leases existed — every configured host, every tool the role already holds, the whole read-only catalogue, the probe container shape, no call budgets. Setting the file is how a policy starts to differ from "what it always did".

## Failing the boot

The loader refuses a document it cannot apply, because the alternative is a run that silently used the built-in default while the operator believed their file was in force:

| What is wrong | Why it is not tolerated |
|---|---|
| the file cannot be read | the path is wrong or the volume is not mounted; the runtime would otherwise run on defaults for a whole incident |
| the YAML does not parse | a half-applied policy is worse than none: some subtasks would get the rule and some the fallback |
| a mode name nobody issues | `diagnosys` is a typo; the runtime would silently fall back to `diagnosis` and the operator would think the rule applied |
| no `diagnosis` rule | that rule is the fallback for a plan mode the file does not name, so a file without it is ambiguous by construction |
| `ttl` is not a positive duration | a bad `ttl` would silently become the 8h default |
| a budget ≤ 0 | `0` refuses every call of that tool, which is never what a budget is written for |

## Fields

A rule answers "may this subtask do this, here, now". Every list is exact: `"*"` is the only way to say everything, an empty list permits nothing, and a value that is not listed is refused.

| Field | Meaning |
|---|---|
| `targets` | hosts from `OPSAGI_TARGETS`, by `id` |
| `tools` | tool names; the role's own tool table still applies on top, a lease never adds a tool a role does not hold |
| `commands` | command keys from the read-only catalogue in `pkg/opsremote` |
| `shapes` | `probe` or `workshop` for `ops_sandbox` |
| `trace_programs` | program keys the tracer accepts |
| `budgets` | how many times one tool may run in a single execution of a subtask; `"*"` sets the limit for tools the map does not name |

`ttl` at the top level is how long a lease stays valid. Keep it longer than a run: a lease that expires mid-diagnosis refuses the rest of a subtask that is working correctly, and the refusal looks like a policy decision rather than a clock running out.

## What a refusal does

A call outside the lease is refused with a sentence that names the value and what the lease allows. In `shadow` the same sentence is recorded as `would_deny` and the call proceeds, which is how a policy is measured before it bites.

A refusal in a dimension an operator can judge — target, command, shape, trace program — also files an approval request, and the refusal quotes its number. Approving it in the flow's `能力租约` panel writes a grant good for four hours, and the model's next retry passes. A spent budget is not approvable: the answer there is to conclude on the evidence already gathered, or to raise the budget and restart.

## Example

`docs/aiops/policy.example.yml` is a working document, not a sketch: two modes, probe-only, and budgets sized so that a subtask exceeding them is looping rather than investigating.
