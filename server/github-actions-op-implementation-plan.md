# GitHub Actions Op Contract Simplification Plan

This plan narrows the `gha` op family to a single workflow-selection model that behaves the same for both `local` and `github` backends.

## Target Contract

- `workflow` is a simple file name such as `ci.yml` or `release.yaml`.
- The value always resolves to `.github/workflows/<workflow>` in the current repository worktree.
- Only repository-root workflows are supported.
- No protocol-prefixed selectors are supported.
- No `cell://` selectors are supported.
- No `git+...` selectors are supported.
- The selected workflow must declare `workflow_dispatch`.
- For `backend: github`, the same workflow path must exist on the default branch so GitHub will accept `workflow_dispatch` on the temporary ref.
- Both backends execute the selected ref's workflow contents.
- Both backends discard all workflow filesystem and git mutations.

## Why This Contract

- It removes selector-specific branching from the op surface.
- It keeps local and GitHub execution aligned around the same workflow location and trigger model.
- It avoids closure and authentication complexity from external workflow sources.
- It avoids cell-relative behavior that GitHub cannot reproduce.
- It makes recipe authoring simpler because the selector is just the workflow file name.

## Implementation Steps

### 1. Selector and Validation Rewrite

- Replace the current selector parser with a resolver that accepts only a single workflow file name.
- Reject empty values, path separators, traversal patterns, and unsupported extensions.
- Resolve the file to `.github/workflows/<workflow>` under the current worktree.
- Return clear errors for missing files, directories, and invalid names.

Acceptance criteria:

- `workflow: "ci.yml"` resolves successfully when `.github/workflows/ci.yml` exists.
- Values like `repo://.github/workflows/ci.yml`, `cell://...`, `git+...`, `../ci.yml`, and `nested/ci.yml` fail before backend execution.

### 2. Workflow Trigger Compatibility Checks

- Add a shared validation step that inspects the selected workflow YAML and verifies it declares `workflow_dispatch`.
- Use the resolved workflow file contents already read during selector resolution rather than re-reading from multiple places.
- Keep validation shared so `local` and `github` enforce the same trigger requirement.

Acceptance criteria:

- A workflow without `workflow_dispatch` fails with a clear validation error.
- Both backends reject the same invalid workflow before starting execution.

### 3. GitHub Backend Default-Branch Gate

- Add a GitHub-side preflight that verifies the same workflow path exists on the repository default branch.
- Keep the actual execution model unchanged: push temp branch, dispatch by workflow file name with `ref` set to that temp branch, poll, collect artifacts/logs, delete temp branch.
- Do not require the default-branch workflow contents to match the temp-branch copy; only require same-path existence so `workflow_dispatch` is allowed.

Acceptance criteria:

- `backend: github` fails early if the workflow path is absent on the default branch.
- Successful runs continue to execute the temp-branch copy of the workflow.

### 4. Surface and Documentation Cleanup

- Update `gha` docs to show filename-only workflow selection.
- Remove all mention of `repo://`, `cell://`, and `git+...`.
- Clarify that subdirectories under `.github/workflows` are not supported.
- Clarify the GitHub constraint that the same workflow path must exist on the default branch.

Acceptance criteria:

- User-facing docs show only filename-based examples.
- Internal implementation notes stop promising external selector support.

### 5. Test Updates

- Replace selector unit tests with filename-only validation coverage.
- Update backend tests and live tests to use plain file names.
- Add coverage for rejected protocol selectors, rejected slashes, and missing `workflow_dispatch`.
- Add GitHub backend tests for the default-branch existence gate.

Acceptance criteria:

- `go test ./...` passes in `/src/server/gha`.
- Dependent module tests continue to pass in `api` and `c2j`.

## Out of Scope

- External workflow sources.
- Cell-relative workflow resolution.
- Workflow subdirectories.
- Mutation persistence.
- Additional event types beyond `workflow_dispatch`.
