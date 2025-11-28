# SWF Migration - server/recipe-worker

## Objectives
- Rewrite `pkg/compiler/compiler.go` to execute recipes via SWF job/task model (no Temporal SDK, no inline-op abstraction).
- Keep recipe semantics (state/sequences/op invocation, git propagation) with inline child recipes only.
- Support user input pauses via SWF remote tasks triggered by the input op while keeping the op itself unregistered in job workers.

## Scope & Plan (Temporal usage to remove)
- `pkg/compiler` and tests rely on Temporal workflow/activity APIs and inline ops; rewrite to SWF job/task context, removing inline-op abstraction and markers.
- `pkg/worker/*` uses Temporal client/worker; replace with SWF engine bootstrap and capability registration.
- `pkg/worker/deps.go` workflow control exposes `StartChildWorkflow`/`Signal` and workflow/run IDs; remove these Temporal methods, collapse IDs to a single `job_id`, and align callers to SWF job APIs only.
- `pkg/ops/activity_registry.go` uses Temporal activity registration; convert to SWF capability registration.
- `pkg/executor/standalone.go` and tests rely on Temporal testsuite; replace with SWF harness.
- Docs/specs (`AGENTS.md`, `TEMPORAL_APIS.md`, execution specs) describe Temporal; update to SWF semantics and no inline ops.

## Compiler Refactor Plan (detailed)
- **Job/Task model mapping**
  - Single JobType: `"recipe"`. One JobWorker handles the full recipe execution tree (root + any child recipes executed inline).
  - Task types map to op types (taskType = op name). SWF `WorkSet` handles capability wiring automatically from the registered TaskWorkers; no manual capability naming beyond jobType/taskType.
  - Initial story creation: when starting a job, include artifacts listing the root recipe and all potential child recipe definitions available to this run so readers see the full recipe set up front.
  - For ops without a local TaskWorker (e.g., input), `DoTask` triggers a remote task wait; external actor completes via `FindTasksWaitingForCapability(jobType="recipe", taskType="input")` and `TaskHandle.Finish`.
- **Execution context**
  - Replace `workflow.Context` with a struct holding: `JobContext` (from SWF runner), logger (`TaskContext.Logger`), deterministic step ordinal, retry config (`swf.RetryPolicy`), and cancellation via `context.Context`.
  - Remove inline-op abstraction; all ops executed through a unified op executor that decides local vs remote.
- **Op execution (local vs remote)**
  - Local ops (most current “inline” ops): call `RegisterableOp.ExecuteInlineV2`-like handler but with plain `context.Context`, `time.Duration`, `*swf.RetryPolicy`; wrap in manual timeout/backoff.
  - Remote ops (current Temporal activities): invoke `JobContext.DoTask(taskType, TaskData)`; wait for return `TaskData`. If TaskWorker exists locally, SWF runner executes; if not (e.g., input), job pauses until external completion.
  - Git propagation: propagate git context patches after each op execution (local or remote) into the execution context and downstream inputs.
- **Input op remote task**
  - When hitting input op, emit `SimpleTaskData` with required metadata and call `DoTask("input", data)` (no TaskWorker registered).
  - External managers use `FindTasksWaitingForCapability(jobType, "input")` and `TaskHandle.Finish` to provide user responses; compiler resumes with returned data.
- **Sequence/state machine**
  - Reimplement `executeSequence`/state logic as pure Go orchestration that dispatches each op as its own SWF task (jobType `"recipe"`, taskType = op name). No batching multiple ops into a single task.
  - Apply per-op timeouts via `context.WithTimeout`; per-op retries via `swf.RetryPolicy` loop/backoff.
  - Resolution context/template evaluation stays, but uses plain Go context.
- **Child recipes**
  - Synchronous only: call `ExecuteRecipe` recursively within same JobWorker (same job_id). Carry forward git context and invocation tracking; remove async handles/parent-close behavior.
  - Resume logic uses `runmetadata` and SWF `RestartJob` (from recipe-core mapping) rather than Temporal reset.
- **Run metadata / story logging**
  - Record invocation/story envelopes directly to Strata chapters via recipe-core logging helpers (no markers/SideEffect). Ordinals align with SWF runner step counter.
  - Replace workflow/run IDs with single `job_id` everywhere.
- **Retry/timeout**
  - Use `swf.RetryPolicy` for all op and composite retries; implement backoff and non-retryable handling explicitly.
  - Timeouts enforced with `context.WithTimeout`; on timeout emit chapter indicating timeout and fail according to retry policy.
- **Error taxonomy**
  - Remove `temporal.ApplicationError`; use local error types indicating retryable vs non-retryable; map to chapter payloads for history.
- **Worker/registry wiring**
  - Replace Temporal worker manager with SWF `EngineBuilder` per recipes directory: for each recipe, register JobWorker + TaskWorkers (per op capability) except input.
  - Registry hot-reload: stop/start SWF WorkSets on recipe file changes; base task queue concept removed.
- **Testing**
  - Replace Temporal testsuite usage with SWF harness (ticket 11). Port compiler/sequence/state machine tests to run against SWF runner with fake TaskWorkers and remote task watchers for input.
  - Validate git workspace propagation, retry/backoff behavior, and child recipe recursion with job_id identifiers.

## SWF Gaps Affecting This Project
- No timers/heartbeats; compiler must enforce timeouts/backoff manually.
- No child jobs; async child mode cannot be supported.
- No signals/queries; run metadata/input handling relies on chapters and remote task polling.
