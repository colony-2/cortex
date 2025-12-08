# Task Chain + Capability Claim Notes

## What We Added
- **Multi-step ops:** `RegisterableOp` now exposes a `TaskChain` of `TaskStep`s. Each step is registered as a separate task type named `opType:stepName`.
- **Step-driven continuation:** The activity registry emits `NextTaskType` with each step’s output (defaulting to the next chain step) so the compiler can follow step-to-step execution without global lookups.
- **Per-step disallow:** `TaskStep.DisallowAsTask` prevents a step from being run as a normal task; it still participates in chains via `NextTaskType`.

## Intended Flow for “Claimed” Steps
- For a two-step op:
  - Step 1: normal task `op:op` (or `op:step1`) runs on the worker.
  - Step 2: task type `op:step2` is **disallowed** for direct execution; it should surface via `FindTasksWaitingForCapability(jobType, taskType)`, be claimed, and finished via the returned `TaskHandle`.
  - Once step 2 is finished, the recipe completes.

## Integration Test Goal
- Define a two-step op where step 1 is allowed and step 2 is disallowed (`DisallowAsTask`).
- Run a recipe invoking that op.
- Use `SWFEngine.FindTasksWaitingForCapability(RecipeJobType, "op:step2")` to get the pending handle for step 2.
- Complete step 2 via the handle and assert the recipe/job completes with the expected outputs.

## Current Issue
- In the toy SWF engine, the second step never shows up as “waiting for capability”; the job is marked `COMPLETED` immediately after step 1.
- Likely causes:
  - The compiler currently dispatches every step in the chain directly (using `TaskChain`), so a disallowed step is still executed instead of being queued for external claim.
  - The registry marks `DisallowAsTask`, but we don’t currently translate that into an “await external capability” path in the worker runtime.
  - The toy engine’s `DoTask` fallback (`awaitExternalCompletion`) is only used when there is no registered task worker; with per-step task workers registered, the disallowed step is still runnable.

## Next Steps to Fix
1) **Respect DisallowAsTask at dispatch time:** In the compiler/worker path, when a step has `DisallowAsTask`, do not call `DoTask` directly; instead enqueue/await via the engine’s “pending capability” mechanism so `FindTasksWaitingForCapability` can see it.
2) **Registry wiring:** Ensure disallowed steps are not registered as runnable task workers (already in place) and that the runtime routes them to the waiting queue rather than invoking the handler locally.
3) **Re-run the integration test:** Once disallowed steps surface as pending capabilities, the test should claim/finish step 2 and see the job complete.

## Status
- Code changes for task chains and next-step continuation are in place.
- The capability integration test remains failing because the disallowed step never becomes a pending capability. A runtime change is needed to enqueue disallowed steps instead of executing them. Once that is in place, the test can be enabled to validate the full claim/complete flow.
