# Recipe Execution Modes – Remaining Work

This document tracks the remaining tasks required to finish the `git_state` / `run_mode` implementation for the `recipe` op and to land the end-to-end tests described in `server/ops/recipe-op-execution-modes-spec.md`.

## Runtime gaps

- **Populate child `context` for discrete executions.**
  - `executeDiscreteSync` and `executeDiscreteAsync` currently fail with `"missing context"` when the caller does not pre-seed `Raw["context"]`. We need a helper that clones the parent’s `context` block (git/worktree/blobstore/ticket/cell) before handing inputs to the child.
  - Mirror the behaviour of the inline workspace helper so that both sync and async discrete paths always provide a well-formed `context` (including `git_persist_hash`).

- **Backfill git defaults when `Raw` is empty.**
  - The original inline path uses `GitWorkspacePhase` preparation to build `context.git`. Discrete paths should call a shared helper (e.g. `ensureGitInputs`) so bare invocations still get base repo/hash/ticket/cell metadata.
  - Reuse the recipe-worker `withRequiredGitInputs` logic or move it into a shared test helper so unit and integration tests don’t need to duplicate git bootstrap code.

- **Async handle propagation.**
  - After scheduling an async discrete child we should persist the child `context` into the returned handle (at minimum `context.git.persist_hash` and `context.git.base_hash`) so downstream ops can correlate remote commits.

## Schema / API work

- **Extend recipe schema.** Add `git_state` and `run_mode` enums to the recipe invocation schema so fixture validation (`fixtures_schema_validation_test`) recognises recipes that use the new properties.
- **Docs & examples.** Update `recipe_op_execution_modes_spec.md`, CLI docs, and fixture recipes once discrete/async flow is stable.

## Testing plan

1. **Workflow-level unit test (new).**
   - Leverage `testsuite.WorkflowTestSuite` inside `server/ops/pkg/recipe` to simulate:
     1. parent shared sync (current behaviour – regression guard),
     2. discrete sync, verifying the parent worktree is untouched and child commits land in remote via `squashrebasemerge`,
     3. discrete async, verifying an `async_handle` is emitted and the handle carries git metadata.
   - Use the test helper to seed a temporary bare repo with parent/child worktrees.

2. **Recipe-worker integration fixture (later).**
   - Once runtime gaps above are closed, add a real fixture (parent + discrete child + `thinpackrebase`) under `server/recipe-worker/test-fixtures/recipes/` and update the harness so the new ops are auto-registered. This should cover template resolution (`context.git.*`) and end-to-end thin-pack regeneration.

3. **Context exposure regression tests.**
   - Keep `context_template_access_test.go` to ensure `{{ context.git.base_hash }}` resolves at compile time.

## Open questions

- Do we need to expose additional context shortcuts (e.g. `context.worktree`) alongside `context.git.worktree_path`, or should templates always drill into `context.git`?
- How should async handles communicate failure states back to parents? (e.g. add `status` field when the child finishes later via polling?)
- Should recipe schema enforce that `run_mode: async` only appears when `git_state: discrete` to prevent invalid combinations at compile time?

---
Owners: ops & recipe-worker team
