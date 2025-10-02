# Squash Rebase Merge Op Specification

## Overview

The `squashrebasemerge` git op gives recipes a deterministic pathway to collapse local work into a single commit, rebase onto the latest remote `main`, fast-forward merge the result back to the remote repository, and refresh `GitContext` so the merged tip becomes the new base for subsequent operations. It replaces ad-hoc scripting with a worker-managed flow that keeps thin-pack metadata in sync with remote state.

## Goals

- Provide a turnkey mechanism for recipes to deliver feature work onto a target branch (default `main`) without manual intervention.
- Guarantee thin-pack persistence aligns with the merged remote commit so downstream recipes can restore the updated state.
- Allow callers to preserve original authorship by default while still enabling overrides when necessary.
- Immediately reflect the merged remote tip in `GitContext` so future thin packs and git ops build on the updated base.

## Registration & Usage

- Register `squashrebasemerge` under `server/ops/pkg/git` and expose it through the recipe registry.
- Typical usage:

```yaml
- id: deliver_feature
  op: squashrebasemerge
  with:
    target_branch: refs/heads/main
    upstream_remote: origin
```

The op inherits the surrounding gitstate lifecycle.

## Inputs

| Field | Required | Description |
| --- | --- | --- |
| `target_branch` | yes | Fully qualified ref that will receive the merged commit. Defaults to `refs/heads/main` if omitted. |
| `upstream_remote` | no | Remote name to fetch/push; defaults to the remote encoded in `context.git.base_repo`. |
| `preserve_author` | no | Defaults to `true`; toggle to `false` to rewrite the squash commit’s author info. |

## Execution Flow

1. **Fetch Target** – ensure the latest `target_branch` is available locally by fetching from `upstream_remote`.
2. **Squash** – collapse the diff between `context.git.base_hash` and `context.git.persist_hash` into a single commit, preserving metadata in the commit message body for auditability.
3. **Rebase** – replay the squashed commit onto the fetched tip (`git rebase --reapply-cherry-picks`). Conflicts abort with plain text errors.
4. **Fast-Forward Merge** – push the rebased commit to the remote (`git push upstream_remote HEAD:<target_branch>`), requiring a fast-forward.
5. **Thin Pack Persist & Context Refresh** – call `gitcommit.PersistCommit` to record the merged tip, then set `context.git.base_hash` and `context.git.persist_hash` to that remote head so subsequent ops (including additional thin-pack commits) treat it as the new base lineage.

## Failure Semantics

- Fetch/push failures bubble up directly for observability and retry logic.
- Rebase conflicts surface as plain text errors; no structured payload is required.
- Push rejection (e.g., remote advanced) aborts the op with a descriptive error so recipes can decide whether to retry or alert an operator.

## Implementation Notes

- Add to exports.go in `server/git/pkg/export`
- Validate that the operation only proceeds when the local history contains changes (no-op squashes should return early).

## Testing Strategy

- **Unit:** cover squash, rebase, and push happy paths plus failure scenarios (conflicts, push rejection).
- **Workflow Integration:**
  - Run within shared and discrete workspaces to confirm isolation semantics are preserved.
  - Chain with `thinpackrebase` to ensure the resulting remote state can be rebased onto by another recipe.
- **End-to-End:** exercise delivery of a feature branch into `main`, validating that thin packs and remote refs match the merged commit.

