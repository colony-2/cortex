# Git Fast-Forward Merge Enhancement Spec

## Overview
The existing `squashrebasemerge` recipe op must support a fast-forward-only mode so workflows (notably DeprecationRecipe) can guarantee that changes integrate without rebasing. This spec defines the behaviour, inputs, validation, and telemetry required to add a `skip_rebase` flag to the op.

## Goals
- Allow callers to request fast-forward-only merges (`skip_rebase: true`).
- Preserve current behaviour when the flag is omitted or `false`.
- Provide clear errors when a fast-forward is impossible (e.g., upstream branch advanced).
- Continue to run configured tests and collect merge metadata regardless of mode.

## API Changes
Add an optional boolean input to `squashrebasemerge`:
```yaml
skip_rebase: false  # default; when true, the op enforces fast-forward-only semantics
```

Constraints when `skip_rebase` is `true`:
- Local branch must already be a descendant of the target branch (`git merge-base` equality check).
- Rebase step is skipped entirely; the op squashes local commits as-is and attempts `git merge --ff-only` (or equivalent fast-forward push).
- If fast-forward is impossible, return a structured error `NotFastForward` containing:
  - `target_branch`
  - `local_head`
  - `upstream_head`
  - Guidance to rerun detection/remediation before retrying.

## Execution Flow
1. Validate repo state as today (ensure clean working tree).
2. If `skip_rebase` is `true`, assert branch is fast-forwardable (merge-base check). Otherwise, perform existing rebase behaviour.
3. Run configured test commands (unchanged).
4. Squash commits and push:
   - `skip_rebase=false`: existing rebase+squash+force push flow.
   - `skip_rebase=true`: run `git merge --ff-only` (or equivalent `git push` fast-forward) without rebase; abort on failure with `NotFastForward` error.
5. Emit outputs: merged hash, summary, `git_context_patch` as today plus a new boolean `fast_forward`.

## Telemetry
- Log when `skip_rebase` mode is enabled, including merge targets and result.
- Metric `squashrebasemerge_fast_forward_attempts` / `_failures` to monitor usage and conflicts.

## Backward Compatibility
- Flag defaults to `false`; existing recipes require no changes.
- Errors introduced by fast-forward mode are opt-in.
- Documentation updated in Op reference to describe new input and outputs.

## Rollout
1. Implement flag & error handling under feature flag.
2. Add unit/integration tests covering successful FF merges, conflict failure, and fallback to legacy mode.
3. Update documentation (`RECIPE_OPS_REFERENCE.md`) once feature flag enabled globally.
