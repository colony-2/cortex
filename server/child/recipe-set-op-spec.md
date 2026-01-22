# RecipeSet Op Specification

## Overview

`recipe_set` is a workflow op that launches multiple recipe invocations in parallel, forces each child to run with `git_state=discrete` and `run_mode=async`, and blocks until all children complete. Successful execution returns no payload; callers simply proceed once every recipe has finished. If any child fails, the op raises a structured error containing per-child status records aligned with the input order.

## Goals

- Provide a lightweight way to fan out several recipes without hand-written Temporal code.
- Guarantee isolated workspaces for every child while leaving the parent git context untouched.
- Allow duplicate recipe names with distinct inputs; each child is independent.
- Keep implementation entirely within `server/ops`, reusing the existing recipe op execution path and avoiding `server/recipe-core` changes.

## Schema

- Identifier: `op: recipe_set`.
- Usage follows the standard op input pattern:
  ```yaml
  - id: parallel-smoke
    op: recipe_set
    inputs:
      recipes:
        - recipe: release::smoke
          input:
            env: staging
        - recipe: release::smoke
          input:
            env: production
        - recipe: lint::full
          input:
            target: ui-app
  ```
  - `inputs.recipes` (required, min 1): array of recipe calls processed in parallel. Each entry mirrors the existing `recipe` op payload (`recipe`, `input`, `options`, templated values, etc.).
  - `git_state` and `run_mode` are not permitted in this context; the op ignores or rejects them to enforce discrete async execution.

## Shared Implementation with `recipe` Op

- The current `recipe` op logic lives in `server/ops/pkg/recipe`, where `childExecutor` handles workspace setup, child workflow options, telemetry, and async handles.
- To avoid duplication, extract the launch-and-wait logic into reusable helpers:
  - Introduce an exported constructor `recipe.NewChildInvocation(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, call recipe.RecipeInput, baseInputs map[string]any)` that returns a struct with methods for scheduling a child (`Schedule(runMode executionRunMode, gitMode gitStateMode)`) and waiting for completion (`Await(ctx workflow.Context)`).
  - Factor telemetry helpers (`mapKeys`, logging) and workspace planning (`planDetachedWorkspace`, `withInlineWorkspace`) into shared functions that both ops can call.
  - Keep git/run mode parsing private to the recipe package so `recipe_set` can request the enforced combination (`discrete` + `async`) through the helper without reimplementing validation.
- The `recipe_set` executor will import `server/ops/pkg/recipe` and invoke these helpers for each child, guaranteeing identical behavior (timeouts, retry policy, context propagation) as the original op.

## Runtime Semantics (server/ops)

- Add a `RecipeSet` executor that reuses the factored helpers to schedule child workflows by recipe name.
- For each item supplied via `inputs.recipes`:
  - Resolve templated fields and build the same invocation struct used by the `recipe` op.
  - Use the shared helper to schedule the child with `git_state=discrete` and `run_mode=async`, capturing the returned future plus index and recipe identifier. Wait for `GetChildWorkflowExecution()` to confirm startup before launching the next child to avoid Temporal start throttling.
- After all children are launched, wait for every future to finish (e.g., via `workflow.NewSelector`). Parent cancellation triggers `RequestCancelChildWorkflow` for any inflight children and relies on `ParentClosePolicyWaitForChildren` to block until completion or cancellation.
- On universal success, return no payload (`nil` result) and allow downstream ops to continue normally.

## Failure Reporting

- If any child completes with an error, assemble an array of status objects ordered identically to `inputs.recipes` and surface it as the op error payload. Example:
  ```json
  [
    {"index": 0, "recipe": "release::smoke", "status": "succeeded"},
    {"index": 1, "recipe": "release::smoke", "status": "failed", "error": {"message": "lint violation", "details": {...}}},
    {"index": 2, "recipe": "lint::full", "status": "succeeded"}
  ]
  ```
- The failure payload lets callers or tooling correlate outcomes with inputs, including scenarios where the same recipe name appears multiple times.

## Telemetry

- Increment `recipe_set.children_started` and `recipe_set.children_completed` counters tagged by recipe slug and array index.
- Record per-child latency histograms mirroring existing recipe op metrics to observe fan-out performance.

## Testing Strategy

- Unit tests for the `RecipeSet` executor covering:
  - Validation errors for missing or empty `inputs.recipes` and attempts to set `git_state`/`run_mode`.
  - Parallel success path that returns `nil`.
  - Mixed success/failure runs producing the ordered status array payload.
  - Parent cancellation ensuring all in-flight children receive cancel requests.
- Workflow integration test verifying concurrent execution (comparing child start timestamps) and confirming that a failing child surfaces the aggregated status array while successful children still finish.

## Rollout

- Document the new op in authoring guides with examples for running parallel smoke tests or environment promotions.
- Audit existing multi-child recipe patterns and replace them with `recipe_set` where appropriate.
- Monitor the new telemetry counters to gauge adoption and concurrency characteristics.
