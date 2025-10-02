# Git Workspace Modernization – Phase 4 (Execution Modes & Thinpack Rebase)

## Overview

Phase 4 extends the recipe op so callers can choose how nested recipes share Git state and how the parent waits on their completion. The worker also gains a `thinpackrebase` op that rebases an existing thin-pack backed state onto a newer base commit without losing the recorded commit lineage. Together these changes unlock parallel orchestration patterns and simplify keeping long-lived feature branches synchronized with the primary branch.

## Goals

- Parameterize the recipe op with `git_state` and `run_mode` so callers can switch between shared and isolated Git workspaces and synchronous or asynchronous execution.
- Preserve backwards-compatible defaults (`shared` + `sync`) so existing recipes continue to function without modification.
- Provide a discrete Git workspace for child recipes that need fresh clones while isolating parent progress.
- Support fire-and-forget recipe execution where the parent continues immediately and ignores child outputs.
- Introduce the `thinpackrebase` op to rebase persisted work onto a new base hash and publish an updated thin pack + `GitContext`.
- Add a `squashrebasemerge` git op that squashes local history, rebases onto the main branch, and fast-forward merges back to the remote.

## Recipe Op Enhancements

### New Options

| Field | Values | Default | Description |
| --- | --- | --- | --- |
| `git_state` | `shared`, `discrete` | `shared` | Controls whether the child recipe receives and advances the parent Git state (`shared`) or runs against an isolated clone that does not flow back to the parent (`discrete`). |
| `run_mode` | `sync`, `async` | `sync` | Determines whether the parent waits for the child to finish (`sync`) or continues immediately (`async`). |

Configuration example:

```yaml
- id: child_release_smoketest
  op: recipe
  with:
    recipe: release::smoke
    git_state: discrete
    run_mode: async
    input:
      environment: staging
      ticket: TICKET-1234
```

Both options are optional; the worker injects defaults when they are omitted or misspelled values are rejected during compile-time validation.

### GitState Semantics

#### `shared`

- This is the current behavior: the parent’s `context.git` is copied into the child request and mutated in-place.
- The child runs inside `gitstate.WithInlineWorkspace`, cloning from the parent run directory and restoring to the parent’s latest `persist_hash`.
- When the child completes successfully, its returned `context.git` replaces the parent’s Git context and `git_persist_hash` scalar.
- Failures roll back exactly as they do today; the parent retains its prior `context.git` until a successful child result arrives.

#### `discrete`

- Treated as a parentless invocation: the compiler marks the child with `GitIsolation=Discrete`, instructing the worker to spin up a plain recipe workspace via `gitstate.WithDetachedWorkspace` without hydrating the parent’s mutable state.
- The helper performs a shallow clone directly from the repository + base hash declared on the recipe (the same behavior as a top-level recipe) so the child starts from the canonical remote view.
- No additional metadata is injected; downstream nodes observe a normal `GitContext` and remain agnostic about how the child was launched.
- The parent ignores any returned `context.git`. If the child needs to share results, it must do so via its own pushes/merges or explicit outputs.
- Workspace cleanup mirrors `WithInlineWorkspace`, deleting the detached worktree after persist or retry failure handling.

Validation rules:
- `discrete` requires Phase 2+ thin-pack infrastructure; compilation fails if the recipe references deprecated git ops.
- Discrete invocations honor retry/backoff policies while the parent’s Git state stays untouched between attempts.

### RunMode Semantics

#### `sync`

- Existing behavior: the parent workflow awaits `childFuture.Get()` and merges outputs when the future resolves.
- Errors propagate to the parent node, triggering retry or failure semantics identical to current shared execution.
- Metrics/timers continue to measure child duration.

#### `async`

- The worker starts the child workflow/activity via `workflow.ExecuteChildWorkflow()` but does **not** await the result.
- Immediately after `GetChildWorkflowExecution()` returns, the parent emits telemetry (`recipe.async_child_started`) containing child recipe ID, workflow ID, run ID, and `git_state`.
- Parent execution continues without merging any child outputs. The node emits a synthetic output payload containing only an `async_handle` (see below).
- Parent cancellation **does not** cascade to async children; the child continues unless it is explicitly cancelled by other means.
- Errors surfaced later by the child are recorded in recipe history and logs but do not retroactively fail the parent node.

Async handle shape (available to downstream nodes if they need to poll/query independently):

```json
{
  "recipe": "release::smoke",
  "workflow_id": "recipe-release::smoke-abc123",
  "run_id": "1f2e3d",
  "git_state": "discrete"
}
```

The compiler injects a validation error for `git_state: shared` combined with `run_mode: async`, because shared state requires synchronized completion to propagate Git context safely.

### Combination Matrix

| `git_state` | `run_mode` | Behavior |
| --- | --- | --- |
| `shared` | `sync` | Unchanged behavior (Phase 3 default). |
| `shared` | `async` | **Invalid** – rejected during compilation. |
| `discrete` | `sync` | Parent waits, child runs in isolated workspace, parent context untouched. |
| `discrete` | `async` | Fire-and-forget execution in isolated workspace with async handle output. |

## Worker Changes

- `server/recipe-core/pkg/schema`: add `git_state` and `run_mode` enumerations to the recipe op definition and update JSON/YAML validators.
- `server/recipe-core/pkg/compiler`: default missing values to `shared`/`sync`, surface validation errors for unsupported combinations, and include the new flags in `CompiledRecipeOp` metadata.
- `server/ops/pkg/recipe/op.go`: branch on the compiled metadata to select `WithInlineWorkspace` vs `WithDetachedWorkspace`, await vs fire-and-forget, and populate the async handle output when needed.
- `pkg/gitstate`:
  - Implement `WithDetachedWorkspace` plus supporting helpers to clone from `BaseRepo`/`BaseHash` using the same semantics as top-level recipes.
  - Ensure detached workspaces reuse the existing thin-pack pipeline without introducing additional serialization fields.
- `server/ops/pkg/git`: register the `thinpackrebase` and `squashrebasemerge` ops and expose them through the activity registry.
- `pkg/history` / `workflowctl`: capture async child metadata so the web UI and CLI can display outstanding runs and their eventual completion status.
- Observability: emit counters for discrete clones (`gitstate.discrete.clone_ms`) and async scheduling (`recipe.async_child.started`).

## Thinpack Rebase Op

### Purpose

Maintain feature branches that persist multiple commits by rebasing them onto a newer base (e.g., `origin/main`) without requiring users to manually clone and replay history outside the worker.

### Registration & Usage

- Register a new op name `thinpackrebase` under `server/ops/pkg/git` (and expose it through the recipe registry once implemented).
- Typical recipe usage:

```yaml
- id: refresh_main
  op: thinpackrebase
  with:
    target_base_hash: {{ context.inputs.main_head }}
    preserve_author: true
```

### Inputs

| Field | Required | Description |
| --- | --- | --- |
| `target_base_hash` | yes | Commit hash that will become the new base (usually the latest `main` HEAD). |
| `upstream_remote` | no | Remote name to fetch the target base from; defaults to the remote encoded in `context.git.base_repo`. |
| `preserve_author` | no | When true (default), reuse original commit authorship; set to false to use the workflow’s default author identity. |
| `update_refs` | no | Optional refspec (e.g., `refs/heads/feature`) to force-update after rebase; defaults to detached HEAD. |

The op runs inside whichever isolation mode the current node uses (shared or discrete) so callers can safely chain it with either workflow.

### Execution Flow

1. **Workspace Preparation** – reuses the active workspace (shared or detached). Ensures the remote configured in `context.git.base_repo` is fetched so the target hash is locally available.
2. **Baseline Validation** – verifies the existing `persist_hash` is a descendant of the new `target_base_hash` and fails fast if history diverged irreconcilably.
3. **Rebase** – runs `git rebase --reapply-cherry-picks` (or libgit2 equivalent) with the old base set to `context.git.base_hash` and the new base set to `target_base_hash`.
4. **Thin Pack Generation** – invokes `gitcommit.PersistCommit` to write a new thin pack representing the rebased commits. Thin-pack filenames incorporate the new base hash for traceability.
5. **Context Update** – updates `context.git.base_hash = target_base_hash`, `context.git.persist_hash = <new tip>`, and appends a `rebased_from` block to the commit metadata (old base + persist hashes).
6. **Output Injection** – `context.git` and `git_persist_hash` are refreshed, enabling downstream nodes to continue from the rebased history.

### Failure Semantics

- Conflicts surface as plain-text activity errors describing the failing command.
- The op respects retry policies; on retry it reuses the same workspace but detects if the target base has moved again to avoid replaying conflicting rebases.
- If thin-pack upload fails, the op retries the upload without re-running the rebase (the new commit is already materialized locally).

### Observability

- Emit `gitstate.rebase.duration_ms`, `gitstate.rebase.conflicts_total`, and `gitstate.rebase.thinpack_bytes` metrics.
- Log structured entries containing `old_base_hash`, `target_base_hash`, `old_persist_hash`, and `new_persist_hash` for audit trails.

## Schema & API Updates

- Update `server/recipe-worker/schema.json` and `server/recipe-core/schema/recipe-op.json` to document the new options and enumerate allowed values.
- Regenerate OpenAPI clients (`api/openapi`, `web/shared`) so the recipe execution endpoints expose the new configuration fields.
- Extend `server/recipe-history` query responses to include async child handles.

## Testing Strategy

- **Unit**
  - `pkg/gitstate`: cover `WithDetachedWorkspace` cloning behavior and workspace cleanup.
  - `ops/recipe`: verify option validation, async handle shaping, and shared/discrete branching.
  - `ops/thinpackrebase`: test successful rebase, conflict detection, and thin-pack regeneration.
  - `ops/squashrebasemerge`: exercise squash, rebase, push, and thin-pack persistence paths.
- **Workflow Integration**
  - Parallel recipe execution with mixed `git_state`/`run_mode` combinations to confirm isolation and parent context behavior.
  - Async discrete child that later fails, ensuring the parent flow continues and history logs the failure.
  - Scenario where `thinpackrebase` runs after a discrete child to confirm blob-store isolation.
  - Parent/child integration described below that exercises discrete mode, merge ops, and subsequent rebase handling end-to-end.
- **End-to-End**
  - Run a full recipe that spawns async smoke tests while the main flow continues to the next stage.
  - Verify the UI displays async child progress and allows linking to the spawned workflow run.
  - Ensure rebased commits remain accessible via the blob store and can be restored by downstream nodes.

## Migration & Rollout

- Phase gated rollout: enable compilation of `git_state`/`run_mode` fields behind a feature flag, defaulting to current behavior until workers are upgraded.
- Update documentation (`VIBETHIS.md`, user cookbook) with new examples and best practices for choosing shared vs discrete.
- Provide migration tooling to rewrite legacy recipes that relied on ad-hoc git ops into the new `thinpackrebase` op where applicable.
- Monitor async child usage metrics before advertising the feature broadly.

## Squash Rebase Merge Op

### Purpose

Provide an opinionated path for a recipe to squash its accumulated local commits, replay them on top of the remote main line, and then fast-forward merge those changes back to the remote repository.

### Registration & Usage

- Register a new git op named `squashrebasemerge` under `server/ops/pkg/git`.
- Typical usage:

```yaml
- id: deliver_feature
  op: squashrebasemerge
  with:
    target_branch: refs/heads/main
    upstream_remote: origin
```

### Inputs

| Field | Required | Description |
| --- | --- | --- |
| `target_branch` | yes | Fully qualified ref that represents the branch to merge into (defaults to `refs/heads/main` if omitted). |
| `upstream_remote` | no | Remote name to fetch `target_branch` from; defaults to the remote encoded in `context.git.base_repo`. |
| `preserve_author` | no | Mirrors the `thinpackrebase` option; defaults to true. |

### Execution Flow

1. **Fetch Target** – ensure the latest `target_branch` is available locally by fetching from `upstream_remote`.
2. **Squash** – collapse the local changes represented by `context.git.persist_hash` vs `context.git.base_hash` into a single commit (keeping commit metadata in commit message body for audit).
3. **Rebase** – reapply the squashed commit on top of the latest target tip (`git rebase --reapply-cherry-picks`). Conflicts error with plain text describing the conflict.
4. **Fast-Forward Merge** – update the remote branch via fast-forward (`git push upstream_remote HEAD:<target_branch>`). No merge commits are created.
5. **Thin Pack Persist** – run `gitcommit.PersistCommit` to record the new tip and thin pack, updating `context.git.persist_hash` and `context.git.base_hash` to the new target tip.

### Failure Semantics

- Fetch or push failures propagate unchanged for observability.
- Rebase conflicts stop execution with plain-text error output.
- Push rejection due to remote divergence emits a descriptive failure so recipes can decide whether to retry or alert an operator.

### Observability

- Emit `gitstate.squash_merge.duration_ms`, `gitstate.squash_merge.push_bytes`, and reuse existing thin-pack metrics.
- Log structured summaries containing `target_branch`, `old_base_hash`, and `new_persist_hash`.

## Integration Test: Discrete Child Merge

Add a workflow integration test that wires together the new execution modes and git ops:

1. **Parent Recipe (A)** – runs in shared/sync mode. It performs a simple file mutation that persists via the existing gitstate decorator.
2. **Child Recipe (B)** – invoked with `git_state: discrete` and `run_mode: sync`. B edits a different file, commits locally, and invokes `squashrebasemerge` to merge the change back into `main`.
3. **Parent Continuation** – once B completes, A invokes `thinpackrebase` against the updated main branch and verifies its workspace now includes B’s edit.

The test should assert:
- Parent and child mutations land in separate thin packs until the merge occurs.
- After `thinpackrebase`, A’s `context.git.persist_hash` reflects the tip produced by B’s merge, and the updated file is present locally.
- No parent cancellation is propagated to the child during the run.

## Open Questions

1. Should discrete children reuse the parent’s `blob_store_uri` or should we allow overriding it per invocation to keep artifacts fully isolated?
2. Do we need a follow-up to surface async child completion events back to the parent via signals for optional chaining?
3. What guardrails do we want if `thinpackrebase` encounters non-fast-forward rebases (e.g., interactive rebase requirements)?

## Exit Criteria

- Recipe op reflects the new options in schema, compiler, and worker with defaults preserving existing behavior.
- Discrete child recipes produce isolated Git workspaces and never mutate the parent’s `context.git`.
- Async invocations run successfully, emit telemetry, and return async handles while the parent proceeds.
- `thinpackrebase` is registered, fully tested, and updates `GitContext` + thin packs after rebasing onto a new base hash.
