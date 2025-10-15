# Git Fast-Forward Merge Enhancement Spec

## Overview
The `squashrebasemerge` git op currently rebases local work onto the target branch tip, squashes the rebased commits, and force-pushes the synthetic merge commit. Deprecation workflows need the ability to publish change sets without rewriting history. This document defines how to introduce a fast-forward-only mode that honours upstream history, surfaces actionable failures, and preserves existing behaviour for callers that do not opt in.

## Goals
- Provide a `skip_rebase` toggle that enforces fast-forward semantics while keeping the default path unchanged.
- Guarantee op-level safety checks so fast-forward publishing fails fast when the local branch diverged.
- Produce telemetry (logs + metrics) that highlight fast-forward adoption and failure reasons.
- Preserve downstream contracts (outputs, tests, audit fields) for both legacy and new modes.

## Non-Goals
- Changing the default behaviour of `squashrebasemerge`.
- Supporting automatic fallback from fast-forward to rebase; callers must make an explicit choice.
- Redesigning workspace preparation, test execution, or thin-pack persistence.

## Background
`squashrebasemerge` lives in `server/git/pkg/squashrebasemerge` and is surfaced through the recipe registry. During normal execution it:
1. Rebases onto the remote target branch (usually `origin/main`).
2. Runs configured test activities.
3. Squashes the rebased commits into a single merge payload.
4. Force pushes the synthetic commit to the target branch.

Fast-forward workflows must avoid step 1 entirely and must never force push. Instead, the op should verify that the local head is already ahead of the remote tip and then fast-forward the remote ref.

## Registration & Usage
No registration changes are required; the op remains addressable as `squashrebasemerge`. Recipes enable fast-forward mode by opting into the new flag:

```yaml
- id: publish
  op: squashrebasemerge
  with:
    target_branch: main
    skip_rebase: true
    tests:
      - id: smoke
        run: npm test
```

## Inputs
| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `skip_rebase` | boolean | `false` | When `true`, the op enforces fast-forward-only publishing. |
| `target_branch` | string | _(required)_ | Unchanged. Remote branch to update. In fast-forward mode this must already contain the merge base. |
| `target_remote` | string | `origin` | Remote used for `fetch`/`push`. Behaviour unchanged but must be fetched before validation. |
| `tests` | array | `[]` | Existing contract for pre-merge checks. Runs regardless of mode. |
| _all existing fields_ | _various_ | _(current defaults)_ | No semantic changes when `skip_rebase` is omitted or `false`.

## Outputs
Outputs remain backwards compatible while adding a new field:
- `merge_commit_hash` – unchanged; in fast-forward mode this is the same as `local_head` and remote tip.
- `git_context_patch` – unchanged contents.
- `fast_forward` *(new boolean)* – `true` when `skip_rebase` mode succeeds, `false` otherwise.

## Pre-Execution Validation
1. **Workspace Cleanliness** – existing check (`git status --porcelain`).
2. **Remote Sync** – ensure `target_remote/target_branch` is fetched prior to validation. In fast-forward mode stale remotes should emit a `RemoteBehind` error.
3. **Fast-Forward Feasibility** – when `skip_rebase=true`, compute
   ```bash
   git merge-base --is-ancestor <remote>/<branch> HEAD
   ```
   If the merge base differs, return `NotFastForward`.
4. **Rebase Guard** – when `skip_rebase=true`, explicitly skip rebase invocation to avoid accidental rebases via shared helpers.

## Execution Flow
1. **Preparation** – reuse existing workspace and remote setup.
2. **Validation** – perform checks above and exit early on failure.
3. **Tests** – always execute configured tests. Fast-forward failures should be detected before test execution when possible, but tests run after validation to preserve current sequencing.
4. **Publish Path**:
   - **Legacy (`skip_rebase=false`)** – run existing rebase/squash/force-push pipeline untouched.
   - **Fast-Forward (`skip_rebase=true`)**:
     1. Skip rebase entirely.
     2. Optionally run `git merge --ff-only <remote>/<branch>` into the local branch to double-verify ancestry (no-op if already descendant).
     3. Perform `git push <remote> HEAD:<branch>` and rely on server-side fast-forward checks; do not pass `--force`.
5. **Output & Telemetry** – emit outputs, record logs, and send metrics.

## Error Handling
Introduce a dedicated error type under `pkg/common` (or op-local package) to signal fast-forward violations:
```go
type NotFastForwardError struct {
    TargetBranch  string
    LocalHead     string
    UpstreamHead  string
}
```
- Returned when merge-base validation or remote push reports a non fast-forward update.
- Should implement `Is(target error)` so callers can unwrap using `errors.Is(err, ErrNotFastForward)`.

Additional failure cases:
- **RemoteBehindError** – raised when the local fetch is stale (optional but improves guidance).
- Push or network failures should reuse existing error flow; include mode info in wrapped errors for diagnosis.

## Telemetry & Metrics
- Log fields: `mode=fast_forward`, `target_branch`, `local_head`, `upstream_head`, `result`.
- Counters:
  - `squashrebasemerge.fastforward.attempts`
  - `squashrebasemerge.fastforward.successes`
  - `squashrebasemerge.fastforward.failures`
- Emit structured error metadata (e.g., via `activity.RecordFailure`) so Temporal history captures fast-forward diagnostics.

## Testing Strategy
- **Unit Tests (`pkg/squashrebasemerge`)**:
  - Happy-path fast-forward: upstream equals base, push succeeds, `fast_forward=true`.
  - Failure cases: merge-base mismatch, remote tip advanced between validation and push.
  - Regression: ensure legacy mode still performs rebase / force push.
- **Integration Tests (`recipe-worker`)**:
  - Recipe fixture that publishes via `skip_rebase=true` and verifies remote state.
  - Temporal replay to ensure deterministic behaviour when the flag toggles.
- **Smoke**:
  - End-to-end run against a real repo to verify no force pushes occur in fast-forward mode.

## Rollout Plan
1. Implement new flag behind feature gate `git.skip_rebase_ff`. Default disabled.
2. Ship unit tests; ensure CI covers both modes.
3. Enable gate for staging recipes and monitor telemetry.
4. Update operator documentation (`VIBETHIS.md`, `RECIPE_OPS_REFERENCE.md`).
5. Remove feature flag once adoption stabilises.

## Documentation Updates
- Extend `server/git/squashrebasemerge-op-spec.md` with flag details.
- Update recipe cookbook examples and DeprecationRecipe docs to highlight the new path.
- Note behavioural differences (no force push, stricter failure mode) in changelog.

## Open Questions
- Should we allow the op to opt into automatic rebase fallback when fast-forward fails (future enhancement)?
- Do we need a configurable retry window to re-fetch/upstream before erroring?
- Confirm naming for telemetry counters matches organisation conventions.
