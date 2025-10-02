# Recipe Op Execution Modes Specification

## Overview

This specification introduces two optional properties on the existing `recipe` op: `git_state` and `run_mode`. Together they let callers choose whether child recipes share the parent’s git workspace and whether the parent waits for completion. The default behavior remains `shared` + `sync`, preserving Phase 3 semantics.

## Goals

- Allow recipes to opt into isolated git execution (`discrete`) without changing downstream logic.
- Support fire-and-forget child execution via `run_mode: async` while keeping deterministic workflow history and easy traceability.
- Maintain backwards compatibility for existing recipes that omit the new properties.

## Properties

| Field | Values | Default | Description |
| --- | --- | --- | --- |
| `git_state` | `shared`, `discrete` | `shared` | Determines whether the child inherits and advances the parent’s `GitContext` (`shared`) or runs using a fresh clone from the declared repo/base (`discrete`). |
| `run_mode` | `sync`, `async` | `sync` | Controls whether the parent blocks on child completion (`sync`) or continues immediately (`async`). |

Both properties are optional. Compilation injects defaults, so existing recipes require no changes.

## Git State Semantics

### `shared`

- Matches current behavior: parent passes its `context.git` to the child via `gitstate.WithInlineWorkspace`.
- Upon successful completion, the child’s updated `context.git` replaces the parent’s git context and `git_persist_hash` scalar.
- Failures leave the parent’s git context untouched until a successful child result is received.

### `discrete`

- Treated as a parentless invocation. The compiler marks the child with `GitIsolation=Discrete`, and the worker uses `gitstate.WithDetachedWorkspace` to create a standard recipe workspace.
- The helper performs a shallow clone from the repository/base hash declared on the recipe, mirroring top-level recipe behavior.
- No special metadata is injected; downstream nodes observe a normal `GitContext`.
- Outputs from the child remain available via templating, but the parent ignores any returned git context. Sharing results requires explicit pushes/merges (e.g., via `squashrebasemerge`).

Validation: `discrete` is only allowed when thin-pack infrastructure (Phase 2+) is present. Compilation fails if deprecated git ops are detected.

## Run Mode Semantics

### `sync`

- Parent awaits the child future (`childFuture.Get()`), merging outputs on success and propagating failures as today.

### `async`

- Worker invokes `workflow.ExecuteChildWorkflow()` but does **not** call `Get()` on the returned future.
- After `GetChildWorkflowExecution()` succeeds, the parent emits telemetry (`recipe.async_child_started`) containing the child recipe ID, workflow ID, run ID, and git state.
- Parent emits a synthetic output payload containing an `async_handle` for downstream consumers:

```json
{
  "recipe": "release::smoke",
  "workflow_id": "recipe-release::smoke-abc123",
  "run_id": "1f2e3d",
  "git_state": "discrete"
}
```

- Parent cancellation does **not** cascade to async children; we set `ParentClosePolicyAbandon` so children continue independently.
- Child failures surface in Temporal history/metrics but never throw on the parent path because the future is not awaited.

Validation: `git_state: shared` + `run_mode: async` is rejected since shared git state requires synchronized completion to propagate context safely.

## Combination Matrix

| `git_state` | `run_mode` | Behavior |
| --- | --- | --- |
| `shared` | `sync` | Existing behavior (default). |
| `shared` | `async` | Invalid (compile-time error). |
| `discrete` | `sync` | Parent waits; child workspace is isolated; parent git context unchanged. |
| `discrete` | `async` | Fire-and-forget child using isolated workspace; parent receives only the async handle. |

## Worker Changes

- `server/recipe-core/pkg/schema`: extend schema/validators with the new enum properties.
- `server/recipe-core/pkg/compiler`: default missing values, enforce invalid combinations, and record the settings in compiled metadata.
- `server/ops/pkg/recipe/op.go`:
  - Switch between `gitstate.WithInlineWorkspace` and `gitstate.WithDetachedWorkspace` based on `git_state`.
  - For async mode, call `ExecuteChildWorkflow`, capture workflow/run IDs, set `ParentClosePolicyAbandon`, emit telemetry, and return the async handle without awaiting the future.
- `pkg/gitstate`: provide `WithDetachedWorkspace` that mirrors top-level recipe semantics (clone from declared repo/base, reuse thin-pack pipeline, handle cleanup).
- `pkg/history` / `workflowctl`: surface async child metadata so CLIs/UI can list outstanding runs.
- Observability: add counters for discrete clones (`gitstate.discrete.clone_ms`) and async scheduling (`recipe.async_child.started`).

## Interaction with Git Ops

- `thinpackrebase` and `squashrebasemerge` (see separate specs) rely on these execution modes for end-to-end flows:
  - Discrete children can commit and merge via `squashrebasemerge` without mutating the parent workspace.
  - Parents can re-align with remote changes using `thinpackrebase` after a child finishes.

## Testing Strategy

- **Unit:**
  - Compiler validation for allowed/invalid combinations and default injection.
  - Recipe op handler tests covering shared vs discrete, sync vs async, and async handle shaping.
- **Workflow Integration:**
  - Mixed `git_state`/`run_mode` combinations to confirm parent context behavior and async telemetry.
  - Async discrete child that later fails, confirming the parent flow continues unabated while history records the failure.
- **Integration Scenario:**
  - Parent recipe (A) mutates a file and persists it via gitstate.
  - A invokes child recipe (B) with `git_state: discrete`, `run_mode: sync`. B edits a different file, commits locally, and runs `squashrebasemerge` to fast-forward main.
  - After B completes, A runs `thinpackrebase` to align with the updated main. Assert that A’s workspace now contains B’s edits and that parent cancellation never propagates to B.

## Rollout

- Feature flag the new properties so recipes can opt in gradually.
- Update documentation and examples to explain when to choose `shared` vs `discrete` and `sync` vs `async`.
- Monitor new metrics to ensure discrete clone costs and async usage remain healthy.
