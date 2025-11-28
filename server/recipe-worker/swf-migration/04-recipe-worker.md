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
- **Execution model**: introduce an engine adapter mapping recipes to SWF job workers + task workers. Op types become capabilities; execute ops locally or as remote tasks; eliminate inline-op special casing.
- **Compiler rewrite**: replace Temporal `workflow.*` usage with SWF-aware execution context (`JobContext`, logger, step ordinals). Reimplement `executeOp/executeSequence/executeStateMachine` with explicit timeout/backoff logic; record story via recipe-core envelopes.
- **Remote ops**: create remote task requests and block until `TaskHandle.Finish` returns data; integrate with invocation tracker and git propagation.
- **Child recipes**: run synchronously by calling `ExecuteRecipe` recursively in the same runner; reject async modes; remove async handle plumbing.
- **Input op integration**: ensure compiler pauses execution when hitting input op, expressing capability need so external manager can complete via `FindTasksWaitingForCapability` + `TaskHandle.Finish`.
- **Worker lifecycle**: replace Temporal worker manager with SWF engine bootstrap registering job/task workers per recipe; keep registry hot reload semantics.
- **Run metadata/resume**: map run metadata to Strata chapters; design resume flow using `RestartJob` and chapter clones (no child workflow IDs).
- **Testing**: port compiler/unit/integration tests to SWF harness; update git workspace tests; ensure async paths removed.

## SWF Gaps Affecting This Project
- No timers/heartbeats; compiler must enforce timeouts/backoff manually.
- No child jobs; async child mode cannot be supported.
- No signals/queries; run metadata/input handling relies on chapters and remote task polling.
