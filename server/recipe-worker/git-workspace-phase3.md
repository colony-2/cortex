# Git Workspace Modernization – Phase 3 (Nested Execution & Rollout)

## Overview

Phase 3 finalizes the Git workspace initiative by extending the decorator model to inline flows, nested recipes, and rollout logistics. The goal is to ensure every execution path—parent recipes, sub-recipes, and inline mutators—shares a consistent Git context without relying on special keys.

## Goals

- Provide a shallow-cloned workspace for sub-recipes that mirrors the parent’s persisted state at handoff time.
- Allow inline ops that mutate the repository to opt into the same prepare/restore/persist guarantees.
- Define validation, testing, and migration work to retire legacy git ops and templates safely.

## Nested Recipes

- The recipe op (`server/ops/pkg/recipe/op.go`) runs inline. Wrap the invocation with `gitstate.WithInlineWorkspace`, which:
  1. Copies the parent’s `context.git` into the child invocation inputs.
  2. Creates a fresh child worktree (`<workspaceRoot>/<runID>/<childID>/work`).
  3. Performs a shallow clone from the parent’s `BaseRepo` and restores to the parent’s latest `PersistHash` using thin packs under `<blobStoreURI>/git/thin-packs`.
  4. Executes the child recipe with the same decorator machinery as Phase 2.
- When the child completes, it returns its updated git state via standard outputs:
  - `context.git` – serialized `GitContext` reflecting the child’s final state (including `persist_hash`).
  - For readability the wrapper sets `git_persist_hash = context.git.persist_hash`, but no other alias keys are emitted.
- The parent updates its own execution context by reading these outputs. There are no reserved or underscored keys involved.

## Inline Mutators

- Inline ops that only read git metadata continue to operate against the shared `context.git` without modification.
- Inline ops that need to mutate the workspace must wrap their logic in `gitstate.WithInlineWorkspace` to ensure clone/restore/persist semantics run inside the workflow task context (e.g., using local activities or deterministic-safe scaffolding as needed).
- Documentation and lint tooling should flag inline ops that attempt file mutations without opting into the helper.

## Error Handling & Retries

- Decorator failures (prepare/restore/persist) surface as activity errors, ensuring Temporal retries re-run the entire lifecycle.
- Invocation tracker metadata (including attempt counts) propagate through `context.git`, giving retry logic insight into past attempts.
- Inline wrappers propagate errors to the parent recipe so failure semantics remain consistent.

## Testing Strategy

- **Unit:** Extend `pkg/gitstate` tests to cover inline helpers and nested clone scenarios (parent->child).
- **Workflow:** Add integration tests under `pkg/compiler/sequence_integration_test.go` verifying:
  - Child recipes observe the parent’s persisted state on entry.
  - The parent context updates to the child’s final `git_persist_hash`.
  - Inline mutators update git state without introducing extra Temporal nodes.
- **End-to-end:** Execute full recipes with nested sub-recipes to ensure blob-store contents match expectations and commit metadata chains remain consistent.

## Migration & Rollout

- Recipes and templates must replace any references to removed git ops with the new built-in behavior.
- Documentation updates should highlight the new required inputs, the availability of `context.git`, and the location of thin packs (`git_blob_store`).
- Downstream consumers must read git metadata exclusively from `context.git` (and `git_persist_hash` when the scalar is more convenient).
- Validation tooling can warn when recipes attempt to register or invoke the removed git ops.

## Open Questions

1. Should inline mutators be linted/blocked unless they explicitly call `gitstate.WithInlineWorkspace`?
2. Do we need cleanup hooks (possibly Phase 4) to reclaim per-run worktrees or stale thin packs residing in the blob store?
3. Which metrics and logs are most useful to monitor clone/persist latencies across nested executions?

## Exit Criteria

- Sub-recipes seamlessly inherit and advance the Git context without special keys.
- Inline mutators use the same git lifecycle as activities or are explicitly marked read-only.
- Legacy git ops are fully retired, with migration guidance delivered to consumers.
