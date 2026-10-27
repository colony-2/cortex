# JobDB: expose unfinished task invocations during inspection

## Update: 2026-10-27

Cortex now pins JobDB `v0.0.59-0.20261027194840-510d817719ac`. Its
`TestExecutionUnfinished*` tests pass, covering independent inspection of local,
external, and retrying tasks across toy, SQLite, and remote backends. The new
start events and replay misses expose unfinished invocation identity, state,
and input. The original missing-observation issue below is addressed at JobDB's
interface; c2j still needs to project the input onto the visible story leaf.

One contract difference remains: the implementation may reconstruct input by
running its builder during replay when no input snapshot was persisted. Tests
explicitly exercise this behavior. Thus the original request below to avoid
rerunning builders is not fully implemented; callers must respect JobDB's
read-only replay contract for input construction.

## Requested capability

Expose an unfinished task's identity, attempt, lifecycle state, and resolved input
through the supported workflow replay/inspection interface before its result is
available. c2j should be able to build a running task leaf without parsing private
execution records or executing the task or its input builder during inspection.

Cortex users currently report that the leaf appears only after completion. They
need to inspect the running operation and its input while waiting for output.

## Source findings

Inspected JobDB `v0.0.57`, used with c2j
`v0.0.105-0.20261027024848-fbcd5cd24a8a`. These are source findings; the reported
production job has not been reproduced locally.

- `pkg/workflow/execution_task.go`: `DoTask` calls `emitTask` after obtaining a
  result. An unfinished replay returns before that call.
- `pkg/workflow/execution_history.go`: `emitTask` emits `OnTaskStart` and
  `OnTaskEnd` together. Task and attempt declarations can also be deferred until
  a subsequent durable boundary.
- `pkg/workflow/execution_task.go` and `execution_external.go`: unfinished-task
  replay misses omit `TaskType` and attempt identity. The external-task path has
  already read a wait record containing input, but does not expose a task-start
  event before returning the miss.

c2j's fallback needs the missing task type to construct a leaf. Supplying the
type alone would still leave the resolved-input requirement unaddressed.

## Contract needed

- Provide an authoritative invocation/attempt identity, task type, state, and
  resolved input while the invocation is unfinished. Include start time when
  known; distinguish queued/waiting work from work actually executing.
- Make this usable by a separate inspection process for both local and externally
  dispatched tasks. Define when input becomes inspectable; once execution starts,
  inspection should not have to wait for completion to retrieve it.
- Do not fabricate an end event, output, or output artifacts for unfinished work.
  Preserve the invocation/attempt identity when the result subsequently arrives.
- Keep inspection read-only and side-effect-free. Do not rerun input builders or
  task bodies to recover input. Use existing data/artifact access rules.

Extending replay observer events or another supported inspection result is fine;
the API and persistence design belong to JobDB. This request does not prescribe
a new table or endpoint.

## Verification

Run integration tests in JobDB with a task held behind an explicit barrier.
Inspect from a separate runtime while it remains blocked, assert identity,
accurate state and input with no completion/output, then release it and assert
the same attempt completes. Cover local execution, external dispatch, and a
running retry with a completed prior attempt. Assert inspection never invokes
the input builder or task again and does not mutate execution history.

The consuming c2j work is described in
[C2J-In-Progress-Story-Feature-Request.md](C2J-In-Progress-Story-Feature-Request.md).
