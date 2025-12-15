# Git Workspace Modernization – Phase 2 (Decorator & Persistence Pipeline)

## Overview

Phase 2 implements the Git lifecycle around every activity execution using a decorator pattern. The worker clones, restores, and persists Git state inline with each op, eliminating standalone git activities and keeping Temporal history aligned with recipe nodes.

## Goals

- Wrap activity execution with prepare/restore/persist logic using the shared `GitContext` introduced in Phase 1.
- Remove the exported `git_persist_commit` and `git_restore_commit` ops from the public registry.
- Record thin packs under `<blobStoreURI>/git/thin-packs` via storage adapters that support both local and remote backends.
- Emit structured, machine-readable commit metadata (YAML block) containing full invocation context.

## Activity Decorator

All activities registered through `ActivityRegistry.EnableActivitiesInWorker` are wrapped by a Git-aware decorator:

```go
func withGitWorkspace(reg ActivityRegistration, gc *gitstate.Controller) activityFn {
    return func(ctx context.Context, req ActivityInvocationRequest) (map[string]interface{}, error) {
        gitCtx := gitstate.ContextFromRequest(req)

        if err := gc.PrepareWorkspace(ctx, gitCtx); err != nil {
            return nil, err
        }
        if err := gc.Restore(ctx, gitCtx); err != nil {
            return nil, err
        }

        outputs, err := reg.Activity.ExecuteV2(req.Invocation, ctx, req.Input)
        if err != nil {
            return nil, err
        }

        newHash, updatedCtx, err := gc.Persist(ctx, gitCtx)
        if err != nil {
            return nil, err
        }

        gitstate.InjectPersistResult(outputs, newHash, updatedCtx)
        return outputs, nil
    }
}
```

### Key Points

- `PrepareWorkspace` lazily performs a shallow clone into the runID-scoped worktree and ensures the blob-store location exists (using adapter-specific calls such as `EnsureLocation`).
- `Restore` replays thin packs when the current persist hash differs from the target state. The first op after cloning can skip restore if the checkout already matches `PersistHash`.
- `Persist` captures changes with `gitcommit.PersistCommit`, writing thin packs to `<blobStoreURI>/git/thin-packs` using adapter primitives (`PutBlob`). The helper returns an updated `executionContext` containing the new commit hash and storage metadata.
- `InjectPersistResult` merges the new state into ordinary outputs by replacing `outputs["context"]["git"]` with the refreshed context (which includes the new `persist_hash`, thin-pack metadata, etc.). For readability we also expose `outputs["git_persist_hash"] = updatedCtx.PersistHash`, but no other alias keys remain.
- Inline ops remain read-only in this phase; mutating inline flows are handled in Phase 3.

## Gitstate Helper Package

Phase 2 introduces `pkg/gitstate`:

- `Controller` encapsulates clone/restore/persist orchestration and accepts configuration for workspace root, blob-store adapters, and commit templates.
- `GitContext` serializes execution metadata (base repo/hash, persist hash, worktree path, blob-store URI, ticket/cell IDs, recipe/node IDs, invocation hash, attempt counters, etc.).
- Storage adapters expose `EnsureLocation`, `PutBlob`, `GetBlob`, and `ListBlobs`, enabling implementations for `file://`, `s3://`, or future backends without altering workflow code.

## Commit Metadata

Each persist operation logs and records a structured commit message:

```text
Recipe <recipeID> node <nodeID>

---
git:
  base_hash: <baseHash>
  previous_hash: <priorPersistHash>
  persist_hash: <newPersistHash>
  blob_store_uri: <blobStoreURI>
  thin_pack_path: git/thin-packs/<newPersistHash>-<priorPersistHash>-<baseHash>.pack
invocation:
  hash: <invocationHash>
  id: <invocationID>
  attempt: <attempt>
  box_id: <boxID>
  activity_id: <activityID>
workflow:
  id: <workflowID>
  run_id: <runID>
ticket:
  id: <ticketID>
  cell: <cellName>
recipe:
  id: <recipeID>
  node_id: <nodeID>
---
```

- The YAML block is parsed from the invocation tracker data (`invocationTracker.nextInvocation`).
- Default author: `"<cellname> <cellname>@colony2"`. Recipes may override via `git_author` input (validated before persist).
- Logs & observability emit the same metadata, enabling thin-pack snapshots to be correlated with exact workflow attempts.

## Workspace & Storage Semantics

- Local filesystem isolation relies on the Temporal run ID in the worktree path, preventing concurrent workflow attempts from colliding.
- Thin packs leverage their hash-based filenames for deduplication; parent and child recipes share the same `<blobStoreURI>` namespace and can immediately access each other’s packs without run-specific prefixes.
- Removing external git persist/restore ops ensures the Temporal history mirrors recipe intent—one activity node per recipe op.

## Deliverables

- Decorator wiring inside `ActivityRegistry.EnableActivitiesInWorker` and `executeOp`.
- Removal of git persist/restore exports from `server/git/pkg/export` and any dependent registries.
- Implementation of `pkg/gitstate` (controller, context struct, storage adapters, helper functions).
- Tests covering decorator behavior, storage adapter wiring, and commit message formatting.

## Exit Criteria

- Activities automatically run inside a prepared Git workspace, and persistence happens after each successful execution.
- No standalone `git_*` ops remain in the public registry; recipes no longer schedule them explicitly.
- Commit metadata includes the full invocation context; thin packs land in the configured blob-store subdirectory.
