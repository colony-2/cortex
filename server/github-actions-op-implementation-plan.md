# GitHub Actions Op Implementation Plan

This plan turns the design in [github-actions-integration-patterns.md](github-actions-integration-patterns.md) into a staged implementation that fits the current server layout. It assumes we should ship value incrementally, starting with local `act` execution for `gha.run` and deferring the highest-risk pieces such as authenticated external workflow sources, lazy external artifact plumbing, and the remote GitHub backend behind explicit later phases.

## Goals

1. Add `gha.run` as a first-class op that can execute a workflow against the per-op worktree already managed by `recipe-worker`.
2. Normalize job and step output so recipes can branch on stable fields instead of parsing logs.
3. Preserve existing git persistence semantics for mutating workflows.
4. Leave room for `gha.run_job` and `gha.runs` without duplicating backend logic.

## Repo Constraints We Should Design Around

1. The worker already creates and restores an isolated git workspace per op in `recipe-worker/pkg/ops/op_executor.go`; `gha.run` should reuse that workspace rather than create a second nested `git worktree`.
2. Ops currently get `WorktreePath()` plus generic services, but `cell://` resolution and synthetic GitHub event payloads also need repo and cell metadata. We should expose a typed git execution context to ops instead of smuggling internal state through user-facing inputs.
3. `workflowctl.GetArtifactLazy()` is already the right seam for lazy artifact materialization. External artifact pointers from [EXTERNAL_ARTIFACTS.md](EXTERNAL_ARTIFACTS.md) should surface there so existing inbox binding code can continue calling `artifact.SaveToFile(...)`.
4. Remote GitHub execution depends on authenticated git push/fetch and a GitHub API client. The current `server/git/pkg/git` public interface defines `AuthConfig`, but authenticated fetch/push is not actually wired through the exported repository methods yet, so `backend: github` should not be the first slice.
5. `gha.runs` cannot safely run multiple mutating workflows against the same workspace in parallel. V1 should make it validation-only (`const: true`) or reject mutating configurations.

## Recommended Package Layout

- New package: `ops/pkg/gha/`
- `ops/pkg/gha/op.go`: registerable ops (`gha.run`, `gha.run_job`, later `gha.runs`)
- `ops/pkg/gha/types.go`: input and output structs plus validation helpers
- `ops/pkg/gha/selector.go`: workflow selector parsing and resolution
- `ops/pkg/gha/normalize.go`: backend-agnostic output builders
- `ops/pkg/gha/act_backend.go`: gitea/act integration
- `ops/pkg/gha/artifacts.go`: artifact and log registration helpers
- `ops/pkg/gha/github_backend.go`: remote backend, later phase
- `ops/pkg/export/exports.go`: export the GHA ops
- `ops/go.mod`: add `gitea/act` and later the GitHub API dependency
- `recipe-core/pkg/ops/op_dependencies.go` or a sibling file: add a typed git execution context accessor for `BaseRepo`, `BaseRef`, `ResolvedBaseHash`, `CellPath`, `CellName`, and `WorktreePath`
- `recipe-worker/pkg/ops/op_executor.go`: populate the new git execution context into op dependencies
- `workflow/...` and `recipe-core/pkg/workflowctl/...`: external artifact pointer plumbing, if not already landed by the prerequisite work

## Delivery Plan

### Phase 0: Prerequisites and Runtime Seams

1. Confirm or implement the external artifact pointer model from [EXTERNAL_ARTIFACTS.md](EXTERNAL_ARTIFACTS.md).
2. Extend op dependencies with a typed git execution context instead of relying on hidden defaulted inputs.
3. Decide the runtime dependency story for GitHub API access and git credentials. If that is not ready, explicitly scope V1 to `backend: act` only.
4. Add a feature flag or registration guard so the op family can land incrementally without implying full backend parity on day one.

Acceptance criteria:

1. An op can access repo root, cell path, base ref/hash, and worktree path without adding undocumented recipe fields.
2. A lazy artifact returned by workflow control can still be materialized by existing inbox binding code via `SaveToFile`.
3. The V1 scope boundary (`act` only vs `act` and `github`) is encoded in code and docs, not left implicit.

### Phase 1: `gha.run` Contract and Selector Resolution

1. Add `ops/pkg/gha` with the public input and output structs for `gha.run`.
2. Implement selector parsing for `repo://` and `cell://` first.
3. Resolve `repo://` and `cell://` directly against the current op worktree.
4. Return clear op-level errors for invalid selectors, missing files, directories, and path traversal.
5. Defer authenticated `git+https://` and `git+ssh://` selectors until shared external-repo auth exists. Optionally support `git+file://` first because it is easy to exercise in tests.

Acceptance criteria:

1. Schema generation exposes `gha.run` with the intended inputs.
2. `repo://` and `cell://` selectors work against the already-restored workspace commit.
3. Invalid selectors fail before any backend execution starts.

### Phase 2: Local `act` Backend MVP

1. Add gitea/act as a library dependency in `ops/go.mod`.
2. Implement a local backend that reads the resolved workflow YAML, plans either the whole workflow or a filtered job, executes against `deps.WorktreePath()`, and honors `with`, `env`, `secrets`, `event`, `timeout`, and `continue_on_error`.
3. Build the synthetic GitHub event/context payload from the current git execution context.
4. Implement structured capture with a `logrus.Hook` and normalize it into the `gha.run` output schema.
5. Handle `actions/checkout` explicitly. The preferred behavior is a shim/no-op against the already-prepared workspace, not a second checkout that rewrites the worktree.
6. Let the existing worker persist any repo mutations after the op finishes; `gha.run` should not bypass `gitstate.Controller`.

Acceptance criteria:

1. A local workflow can run end-to-end against the op worktree.
2. Non-const workflows that edit files are persisted by the existing post-op git decorator.
3. `continue_on_error: true` preserves structured failure output without failing the task.
4. `timeout` produces `timed_out` output and partial logs/results where available.

### Phase 3: Artifact and Log Registration

1. Register uploaded GHA artifacts and combined/per-job logs through the external artifact pointer mechanism.
2. For local `act`, expose artifacts as `file://` pointers rooted at the act artifact directory.
3. Decide whether logs are stored as generated files in the outbox or as lazy external artifacts. Prefer the same pointer model for both to keep recipe behavior consistent.
4. Ensure API and workflow artifact retrieval paths can materialize or stream these artifacts without special-casing GHA consumers.

Acceptance criteria:

1. Downstream artifact bindings can import a GHA artifact or log with the same `${{ ...artifacts[...] }}` syntax used elsewhere.
2. Unreferenced GHA artifacts are not eagerly copied into every downstream inbox.
3. Artifact names are stable and collision behavior is documented.

### Phase 4: Output Variants and Op Family Expansion

1. Add `gha.run_job` as a thin wrapper over the same execution core plus flattened output shaping.
2. Add `gha.runs` only after deciding concurrency semantics. The safest initial contract is `const: true` validation workflows only.
3. Share normalization, artifact registration, and selector resolution code across all three ops.

Acceptance criteria:

1. `gha.run_job` does not duplicate backend logic.
2. `gha.runs` rejects mutating configurations or runs them serially with explicit documented behavior.
3. All three ops produce stable schemas that recipe validation can understand.

### Phase 5: External Workflow Sources

1. Add `git+file://` selector support first, because it is easy to test locally.
2. After authenticated git access is available in the shared git package, add `git+https://` and `git+ssh://`.
3. Record `workflow_selector`, resolved commit, content hash, and resolution timestamp exactly once at selector resolution time.

Acceptance criteria:

1. External selectors are cached or memoized at the right granularity for immutable commit pins versus symbolic refs.
2. Audit fields are available for any non-hash external ref.
3. Private repo access is driven by shared auth plumbing, not one-off shell hacks inside the op.

### Phase 6: Remote GitHub Backend

1. Introduce a remote backend adapter that can push the current commit to a temporary ref, trigger the workflow, poll run status and jobs, register artifact/log pointers, and clean up the temporary ref.
2. Keep this behind a flag until git auth and GitHub API auth are fully wired.
3. For non-const workflows, define the exact fetch-back flow before implementation. If the remote workflow can produce more than one commit, the design must be tightened before coding.

Acceptance criteria:

1. `backend: github` produces the same normalized output shape as `backend: act`.
2. Temporary refs are namespaced and cleaned on success, with GC fallback for failures.
3. Mutating remote workflows either have a proven single-commit round-trip or remain unsupported.

### Phase 7: Recipe Adoption and Fallbacks

1. Pilot `gha.run` in `ticket-validate` only for repositories with `.github/workflows/ci.yaml`.
2. Keep the existing `recipe.run_and_get_result` fallback path until the GHA path has stable artifact/log behavior.
3. Add recipe fixtures demonstrating lint/test branching and artifact consumption.

Acceptance criteria:

1. Projects without a matching workflow still follow the current validation path.
2. Projects with a valid workflow can branch on `outputs.status`, `outputs.exit_code`, and per-job data without custom shell detection.

## Testing Plan

- Unit tests in `ops/pkg/gha` for selector parsing, output normalization, timeout handling, `continue_on_error`, and artifact pointer registration metadata.
- Integration tests in `ops/pkg/gha` that run a small fixture workflow with `act`, verify workspace mutations persist through the existing git decorator, and verify `repo://` versus `cell://` resolution.
- Recipe-worker integration tests with end-to-end fixtures under `recipe-worker/test-fixtures/recipes/` to verify outputs and artifacts flow through template interpolation and state-machine transitions.
- Remote-backend tests using mocked GitHub APIs for polling and normalization, plus a gated live test only if the repo already supports secret-backed integration coverage.

## Recommended MVP Cut

Ship this first:

1. `gha.run`
2. `backend: act`
3. `repo://` and `cell://`
4. Structured outputs
5. `continue_on_error`
6. `timeout`
7. Non-const mutation support through the existing worker git persistence

Defer this until after MVP:

1. `gha.runs`
2. Remote GitHub backend
3. Private external `git+ssh://` workflows
4. Full lazy artifact/log pointer support if the prerequisite work is not already merged

## Open Questions

1. Should resolved workflow metadata live in the op output, the execution record, or both?
2. Should `actions/checkout` be a hard shim, or should we allow a limited pass-through for workflows that depend on custom checkout options?
3. Is `gha.runs` meant to be true parallel execution or just ergonomic recipe sugar over multiple `gha.run` calls?
4. What is the canonical credentials source for `backend: github` and private `git+` selectors in this codebase?
