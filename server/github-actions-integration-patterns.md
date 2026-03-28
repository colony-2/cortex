# GitHub Actions Integration: `gha.run` Op Design

## Overview

A new Colony2 op that executes GitHub Actions workflows as atomic steps within recipes. Colony2 dispatches the workflow to nektos/act (or a remote runner), waits for completion, and captures results. From Colony2's perspective, a GHA workflow is just another activity — like `codex.exec` is to agent work, `gha.run` is to CI/CD work.

Colony2 handles orchestration, state, durability, and data flow. GitHub Actions handles execution within its runner model. The boundary is clean: inputs go in, status and artifacts come out.

**Prerequisites:** This design depends on two general-purpose c2 enhancements that should be implemented first:
- **`const` nodes** ([CONST_NODES.md](CONST_NODES.md)) — most `gha.run` usage requires `const` for correct validation semantics
- **External artifact pointers** ([EXTERNAL_ARTIFACTS.md](EXTERNAL_ARTIFACTS.md)) — GHA artifacts and logs are exposed as lazy external pointers rather than eagerly downloaded

---

## Workflow Source

The `workflow` field identifies the GitHub Actions workflow file to execute. Three protocols are supported: `repo://` and `cell://` for the current cell's repository, and `git+<scheme>://` for external git repositories.

### `repo://` — Repository Root (Primary for GHA Workflows)

Reference a file relative to the **repository root**. This is the standard way to reference GHA workflows, since `.github/workflows/` lives at the repo root regardless of which cell you're in.

```
repo://<repo-relative-path>
```

**Examples:**
```
repo://.github/workflows/ci.yaml
repo://.github/workflows/lint.yaml
repo://.github/workflows/build-and-test.yaml
```

### `cell://` — Cell Working Path

Reference a file relative to the **cell's working path** within the repo. Resolves to `<cell_path>/<path>` in the repository. Useful for cell-local files, but rarely needed for GHA workflows since those conventionally live at the repo root.

```
cell://<cell-relative-path>
```

**Examples:**
```
cell://workflows/validate.yaml       → <cell_path>/workflows/validate.yaml
cell://.github/workflows/ci.yaml     → <cell_path>/.github/workflows/ci.yaml
```

### Shared Semantics for `repo://` and `cell://`

- The workflow file is always read from the auto-persisted commit of the prior op. Since c2 commits after every op, there is no ambiguity about uncommitted state — both protocols resolve against committed state.
- No git ref is specified or needed. The ref is implicitly the current op's input commit.
- A cell is a subdirectory within a repo. `repo://` paths start from the repo root; `cell://` paths start from `context.workflow.cell_path`. For cells at the repo root (`cell_path: "."`), the two are equivalent.

**Parsing rules (both protocols):**
1. Require a non-empty path after the `://`
2. Reject empty path segments
3. Reject absolute paths (must not start with `/`)
4. Reject path traversal (`..`) outside the repository root
5. Require the target to be a file, not a directory
6. Normalize the path before resolution

### `git+<scheme>://` — External Git Repositories

For workflows in other repositories — shared CI templates, org-wide security scans, third-party workflow collections. Uses the same canonical git selector format as recipe and skill references.

```
git+<scheme>://<repo-location>//<repo-relative-workflow-path>@<git-ref>
```

Supported schemes: `file`, `ssh`, `http`, `https`.

**Examples:**

```
git+ssh://git@github.com/acme/shared-workflows.git//.github/workflows/standard-ci.yaml@main
git+https://github.com/acme/platform-workflows.git//.github/workflows/go-ci.yaml@v2.1.0
git+https://github.com/acme/shared-workflows.git//.github/workflows/node-ci.yaml@a1b2c3d4e5f6
git+file:///src/test-fixtures//.github/workflows/mock-ci.yaml@HEAD
```

**When to use each scheme:**

| Scheme | Use case |
|--------|----------|
| `git+ssh://` | Private org repos accessed via SSH keys |
| `git+https://` | Public repos or private repos with token auth |
| `git+http://` | Internal infrastructure without TLS |
| `git+file://` | Local filesystem repos — primarily useful for testing, development, or referencing sibling repos on the same machine. This is *not* the way to reference the cell's own repo (use `repo://` or `cell://` for that). |

**Parsing rules:**
1. Split on the last `@` → repo+path portion and git ref
2. Require a non-empty git ref
3. Require a `git+` scheme prefix with one of: `file`, `ssh`, `http`, `https`
4. Split the repo+path on `//` → repo URL and repo-relative workflow path
5. Require a non-empty repo-relative workflow path
6. Reject empty path segments
7. Reject absolute workflow paths (must not start with `/`)
8. Reject `.` and path traversal (`..`) outside the repository root
9. Require the target to be a file, not a directory
10. Normalize the workflow path before resolution

**Resolution:**

- **`git+ssh://` / `git+https://`**: Fetch the file from the remote repository at the specified ref. Selectors pinned to a commit hash are cached indefinitely (immutable content). Symbolic refs (`@main`, `@v2.1.0`) are re-resolved each execution.
- **`git+http://`**: Same as HTTPS but without TLS.
- **`git+file://`**: Read the file from a local filesystem git repo at the given ref. The repo path is everything before `//`. Useful for testing against fixture repos or referencing workflows in adjacent local repositories.

### Workflow Content Recording

For `repo://` and `cell://` selectors, the workflow content is pinned to the auto-persisted commit — the commit hash itself is the record of what was run.

For `git+` selectors with non-hash refs (e.g., `@main`, `@v2.1.0`), Colony2 records the following in the op's execution record at resolution time:

| Field | Description |
|-------|-------------|
| `workflow_selector` | The original selector as written in the recipe |
| `resolved_commit` | The concrete commit hash the ref resolved to |
| `workflow_content_hash` | SHA-256 of the workflow file content |
| `resolved_at` | Timestamp of resolution |

This ensures auditability — given any past op execution, you can determine exactly which workflow file was run, even if the branch or tag has since moved. The resolved commit is also used for caching: if the same selector resolves to the same commit on a subsequent execution, the cached file is reused.

### Using Template Expressions

Recipe authors can construct external selectors dynamically:

```yaml
workflow: 'git+https://github.com/acme/shared-workflows.git//.github/workflows/ci.yaml@{{ inputs.ci_workflow_version }}'
```

`repo://` and `cell://` selectors are typically static since the path is fixed and the ref is implicit.

---

## Op Family

Following the `recipe.*` pattern (single vs. batch, sync vs. async), the GHA integration is a family of ops:

| Op | Behavior |
|----|----------|
| `gha.run` | Run one workflow, wait for completion, return results |
| `gha.run_job` | Run a single job from a workflow (skip others) |
| `gha.runs` | Run multiple workflows in parallel, wait for all |

`gha.run` is the primary op. The others are sugar for common patterns.

---

## `gha.run` — Input Structure

```yaml
- id: ci
  op: gha.run
  const: true
  inputs:
    # --- Workflow source (required) ---
    workflow: 'repo://.github/workflows/ci.yaml'

    # --- What to run ---
    job: test                               # optional: run only this job (default: all jobs)
    event: push                             # optional: simulate this trigger event (default: push)

    # --- GHA inputs (workflow_dispatch) ---
    with:
      node-version: "20"
      environment: staging

    # --- Environment ---
    env:                                    # extra env vars injected into all jobs
      CI: "true"
      DATABASE_URL: "postgres://..."
    secrets:                                # mapped to GHA secrets context
      NPM_TOKEN: "{{ context.secrets.npm_token }}"

    # --- Execution ---
    backend: act                            # act (local, default) | github (remote)
    runner_image: ubuntu-latest             # act runner image override
    timeout: 30m                            # max wall-clock time
    continue_on_error: false                # if true, op succeeds even if workflow fails

    # --- Remote backend config (only when backend: github) ---
    remote:
      push_to: origin                       # git remote to push state to
      ref_prefix: refs/c2/gha              # namespace for temporary refs
```

### The `job` Field

By default, `gha.run` executes all jobs in the workflow, respecting `needs:` dependencies. The `job` field filters to a single job (and its transitive `needs:` dependencies).

This enables a pattern where you reference the project's full CI workflow but run specific jobs at different recipe stages:

```yaml
# Run lint early, before implementation review
- id: lint
  op: gha.run
  const: true
  inputs:
    workflow: 'repo://.github/workflows/ci.yaml'
    job: lint

# Run full test suite after implementation
- id: test
  op: gha.run
  const: true
  inputs:
    workflow: 'repo://.github/workflows/ci.yaml'
    job: test
```

### The `event` Field

GHA workflows use `on:` to declare trigger events, and many steps/jobs use `if: github.event_name == 'push'` conditionals. The `event` field controls what act simulates:

- `push` (default) — most broadly compatible
- `pull_request` — for workflows that behave differently on PRs
- `workflow_dispatch` — when the workflow expects explicit inputs via `with:`

Colony2 constructs a synthetic event payload with the cell's git context (ref, sha, repository) so that `github.*` expressions resolve correctly inside the workflow.

---

## Structured Output

The `gha.run` op produces a unified output structure regardless of backend. Colony2 is responsible for normalizing act's local output and GitHub's API responses into this common schema.

### Output Schema

```json
{
  "status": "success",
  "exit_code": 0,
  "duration_seconds": 142,
  "error_message": "",
  "workflow": {
    "resolved_selector": "repo://.github/workflows/ci.yaml",
    "resolved_commit": "a1b2c3d4",
    "content_hash": "sha256:..."
  },
  "jobs": {
    "test": {
      "status": "success",
      "conclusion": "success",
      "duration_seconds": 95,
      "steps": [
        {
          "name": "Checkout",
          "status": "success",
          "duration_seconds": 2
        },
        {
          "name": "Setup Node",
          "status": "success",
          "duration_seconds": 8
        },
        {
          "name": "Run tests",
          "status": "success",
          "duration_seconds": 85
        }
      ]
    },
    "lint": {
      "status": "success",
      "conclusion": "success",
      "duration_seconds": 30,
      "steps": [
        {
          "name": "Checkout",
          "status": "success",
          "duration_seconds": 2
        },
        {
          "name": "Run lint",
          "status": "success",
          "duration_seconds": 28
        }
      ]
    }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `success`, `failure`, `cancelled`, `timed_out` |
| `exit_code` | int | 0 if all jobs passed, 1 otherwise |
| `duration_seconds` | int | Wall-clock execution time |
| `error_message` | string | Non-empty if the op itself failed (not the workflow) |
| `workflow` | object | Resolved workflow metadata (selector, commit, content hash) |
| `jobs` | map | Per-job status, conclusion, duration, and ordered step detail |
| `jobs.<id>.matrix` | map | Matrix values for this job instance (empty if no matrix) |
| `jobs.<id>.steps` | list | Ordered step results with name, status, and duration |

### How Structured Output Is Captured

**Local (gitea/act):** Colony2 uses gitea/act as a Go library and injects a `logrus.Hook` via `common.WithLoggerHook()`. The hook captures per-job results (`jobResult` field), per-step results (`stepResult` field with `Conclusion`/`Outcome`/`Outputs`), and log content (`raw_output` lines). Colony2 accumulates these events during execution and builds the output schema. See [Act/Gitea Integration Details](#actgitea-integration-details) for the full API pattern.

**Remote (GitHub):** Colony2 uses the GitHub Actions API:
- [List workflow run jobs](https://docs.github.com/en/rest/actions/workflow-jobs#list-jobs-for-a-workflow-run) returns per-job status, conclusion, and step details
- [Download workflow run logs](https://docs.github.com/en/rest/actions/workflow-runs#download-workflow-run-logs) returns the full log archive

Both backends produce the same output schema. Recipe authors write transition conditions once and they work regardless of backend.

### `gha.run_job` — Flattened Output

When using `gha.run_job`, outputs are flattened since there's only one job:

```json
{
  "status": "success",
  "conclusion": "success",
  "exit_code": 0,
  "duration_seconds": 42,
  "steps": [
    { "name": "Checkout", "status": "success", "duration_seconds": 2 },
    { "name": "Run tests", "status": "success", "duration_seconds": 40 }
  ]
}
```

### `gha.runs` — Aggregate Output

```json
{
  "status": "failure",
  "all_passed": false,
  "results": {
    "test": { "status": "success", "exit_code": 0, "duration_seconds": 120 },
    "lint": { "status": "success", "exit_code": 0, "duration_seconds": 30 },
    "build": { "status": "failure", "exit_code": 1, "duration_seconds": 95 }
  }
}
```

---

## Artifacts and Logs

GHA workflows produce two kinds of output data: **artifacts** (from `actions/upload-artifact`) and **logs** (stdout/stderr from jobs and steps). Colony2 exposes both as **external artifact pointers** — the general-purpose lazy artifact mechanism described in [EXTERNAL_ARTIFACTS.md](EXTERNAL_ARTIFACTS.md).

### GHA Artifacts

Every artifact uploaded by `actions/upload-artifact` in the workflow is automatically registered as an external artifact pointer. The GHA artifact name becomes the c2 artifact key — no configuration needed.

| | act backend | github backend |
|-|-------------|----------------|
| **URL** | `file://` path to act's local artifact directory | `https://` GitHub Artifacts API download URL |

A workflow that uploads `test-results` and `coverage-report` produces two c2 artifacts:

```yaml
# These just work — c2 registers pointers automatically
artifacts:
  results: '${{ sequence.ci.artifacts["test-results"] }}'
  coverage: '${{ sequence.ci.artifacts["coverage-report"] }}'
```

Nothing is downloaded until a downstream op imports the artifact.

### Logs as External Artifacts

Workflow logs are not GHA artifacts — they come from act's structured output (local) or the GitHub logs API (remote). Colony2 registers them as external artifact pointers so they can be consumed using the same mechanism:

| Artifact key | Content | act URL | github URL |
|-------------|---------|---------|------------|
| `gha-logs` | Combined workflow log (all jobs) | `file://` path to parsed act output | `https://` GitHub logs download endpoint |
| `gha-logs/<job-id>` | Per-job log | `file://` path to filtered act output | `https://` per-job log endpoint |

```yaml
# Import the full log into a review step's inbox
- id: review
  op: input
  artifacts:
    logs: '${{ sequence.ci.artifacts["gha-logs"] }}'
  inputs:
    form:
      title: "CI Results"
      question: "Review CI output. Logs available in inbox."
```

The per-job log breakdown allows ops to selectively import only the logs they need.

---

## Execution Model: Git State

Colony2 auto-persists a local git commit after every op. The commit hash from the prior op is the canonical input state for `gha.run`. This commit — not a working directory — is the source of truth.

### Local Execution (act)

1. **Worktree creation.** Colony2 runs `git worktree add` at the prior op's commit hash, creating an isolated worktree for act. This is a cheap, fast operation — git worktrees share the object store with the main repo.

2. **Workflow resolution.** If the `workflow` selector uses `repo://` or `cell://`, the workflow YAML is read from this worktree (which is checked out at the prior op's commit). If it uses a `git+` selector, the workflow file is fetched from the specified repo/ref separately.

3. **Act invocation.** Colony2 uses gitea/act as a Go library: parses the workflow via `model.ReadWorkflow`, plans execution via `CombineWorkflowPlanner`, configures the runner with `Workdir` pointing at the new worktree, and executes with a `logrus.Hook` for structured result capture. act runs the workflow in Docker containers, with the worktree mounted as the workspace.

4. **`actions/checkout` handling.** Colony2 sets `GITHUB_WORKSPACE` to the worktree and configures act so that `actions/checkout` acts as a no-op — the workspace already contains the committed state at the correct ref.

5. **Post-execution.** If the op is `const` (see [CONST_NODES.md](CONST_NODES.md)), the worktree is discarded. If not `const`, any changes in the worktree are committed and the git state pointer advances. The temporary worktree is then cleaned up. External artifact pointers are registered for any GHA artifacts produced.

### Remote Execution (github)

When `backend: github`, the workflow runs on real GitHub Actions runners. Colony2 must get the cell's git state to GitHub and get results back.

1. **Push state.** Colony2 pushes the prior op's commit to a remote under a temporary ref (e.g., `refs/c2/gha/<job-id>/<op-hash>`). The `remote.push_to` input controls which git remote is used. The `remote.ref_prefix` controls the ref namespace.

2. **Trigger workflow.** Colony2 triggers the workflow via the GitHub API (`workflow_dispatch` or `repository_dispatch`), targeting the pushed ref.

3. **Poll for completion.** Colony2 polls the GitHub API for the workflow run status. Structured output (per-job status, step details) is captured via the workflow jobs API.

4. **Register artifact pointers.** For each GHA artifact produced by the workflow, Colony2 registers an external artifact pointer with the GitHub API download URL. For logs, it registers pointers to the GitHub logs download endpoint. Nothing is downloaded at this point.

5. **Reconstruct thin pack (if not `const`).** If the workflow mutated the repo and the op is not `const`, Colony2 fetches the resulting state from the remote ref and produces a thin pack representing the delta. This thin pack is applied to local history exactly as if the op had run locally — maintaining a continuous commit chain.

6. **Cleanup.** The temporary remote ref is deleted after results are captured (or garbage-collected if the op fails — see Decision 5).

**Key property:** From the recipe's perspective, local and remote execution produce identical structured output and artifact pointers. A recipe that works with `backend: act` also works with `backend: github` — only the execution environment and artifact locators differ. The git history is continuous regardless of where the op ran.

### Workflow Source vs. Execution Workspace

The workflow source and the execution workspace are independent concerns:

| | `repo://` / `cell://` workflow | `git+` workflow (external repo) |
|-|--------------------------|--------------------------|
| **Local (act)** | Read from worktree at prior commit | Fetch from external repo, run against cell's repo worktree |
| **Remote (github)** | Read from pushed ref | Referenced via GitHub's reusable workflows or fetched to pushed repo |

The execution workspace is always the cell's git state. The workflow file can come from anywhere.

---

## Concrete Examples

### Example 1: Replace `ticket-validate`

Current approach — `ticket-validate` recipe detects test commands, runs them via `command_execution`:

```yaml
# current: in new-ticket.yaml
validate:
  op: recipe.run_and_get_result
  inputs:
    name: ticket-validate
    inputs:
      commands: '...'
```

New approach — run the project's CI workflow directly, marked `const` since validation shouldn't mutate:

```yaml
validate:
  op: gha.run
  const: true
  inputs:
    workflow: 'repo://.github/workflows/ci.yaml'
    timeout: 30m
    continue_on_error: true
```

The recipe author no longer needs to detect test frameworks or construct shell commands. The project's CI definition *is* the validation. GHA artifacts (test results, coverage reports) are registered as external artifact pointers and available to downstream ops on demand.

### Example 2: Selective Job Execution in State Machine

Run different CI jobs at different stages of the ticket lifecycle. The entire validation phase is `const`:

```yaml
state:
  initial: implement
  states:
    implement:
      op: codex.exec
      transitions:
        - to: validation_phase
          when: "true"
      inputs: { ... }

    validation_phase:
      const: true
      sequence:
        - id: lint
          op: gha.run_job
          inputs:
            workflow: 'repo://.github/workflows/ci.yaml'
            job: lint
            continue_on_error: true
        - id: test
          op: gha.run
          inputs:
            workflow: 'repo://.github/workflows/ci.yaml'
            continue_on_error: true
            timeout: 30m
      outputs:
        lint_passed: '${{ sequence.lint.outputs.exit_code == 0 }}'
        test_passed: '${{ sequence.test.outputs.status == "success" }}'
        all_passed: '${{ sequence.lint.outputs.exit_code == 0 && sequence.test.outputs.status == "success" }}'
      transitions:
        - to: implement
          when: "!outputs.all_passed"
        - to: ready_to_merge_review
          when: "true"

    ready_to_merge_review:
      op: input
      artifacts:
        test_logs: '${{ states.validation_phase.artifacts["gha-logs/test"] }}'
      inputs:
        form:
          title: "Ready to merge"
          fields:
            - id: decision
              type: multiple_choice
              question: |
                Lint: ${{ states.validation_phase.outputs.lint_passed ? "passed" : "FAILED" }}
                Tests: ${{ states.validation_phase.outputs.test_passed ? "passed" : "FAILED" }}
              options:
                - value: merge
                  label: Merge
                - value: revise
                  label: Revise implementation
```

### Example 3: Shared Org Workflow

Reference a centrally-maintained CI workflow from a shared repo, pinned to a release tag:

```yaml
validate:
  op: gha.run
  const: true
  inputs:
    workflow: 'git+https://github.com/acme/shared-workflows.git//.github/workflows/node-ci.yaml@v3.2.0'
    timeout: 30m
    continue_on_error: true
    with:
      node-version: "20"
    secrets:
      NPM_TOKEN: "{{ context.secrets.npm_token }}"
```

The workflow lives in a separate repository and is version-locked. The resolved commit hash and content hash are recorded in the execution record for auditability.

### Example 4: Private Infra Workflow via SSH

```yaml
validate:
  op: gha.run
  const: true
  inputs:
    workflow: 'git+ssh://git@gitl.colony2.com/infra/ci-templates.git//.github/workflows/go-ci.yaml@main'
    timeout: 20m
    continue_on_error: true
```

### Example 5: Parallel Workflows from Mixed Sources

```yaml
- id: ci
  op: gha.runs
  const: true
  inputs:
    workflows:
      - id: project_ci
        workflow: 'repo://.github/workflows/ci.yaml'
      - id: security_scan
        workflow: 'git+https://github.com/acme/shared-workflows.git//.github/workflows/security-scan.yaml@v1.0.0'
        secrets:
          SNYK_TOKEN: "{{ context.secrets.snyk_token }}"
    timeout: 45m
```

### Example 6: Remote Execution

```yaml
- id: full_ci
  op: gha.run
  const: true
  inputs:
    workflow: 'repo://.github/workflows/ci.yaml'
    backend: github
    remote:
      push_to: origin
      ref_prefix: refs/c2/gha
    timeout: 30m
    continue_on_error: true
```

The git state flows: local commit → push to `refs/c2/gha/<job-id>/<op-hash>` on `origin` → GitHub runs the workflow → structured output captured via API → artifact pointers registered (not downloaded) → temporary ref deleted. Since this is `const`, no thin pack is fetched back.

### Example 7: Code Generation (Mutating Workflow)

Not all workflows are read-only. A code generation workflow intentionally mutates the codebase:

```yaml
- id: generate
  op: gha.run
  # NOT const — this workflow produces code changes that should be committed
  inputs:
    workflow: 'repo://.github/workflows/codegen.yaml'
    timeout: 10m

- id: validate_generated
  op: gha.run
  const: true
  inputs:
    workflow: 'repo://.github/workflows/ci.yaml'
    timeout: 30m
    continue_on_error: true
```

### Example 8: Consuming Artifacts Lazily

```yaml
sequence:
  - id: ci
    op: gha.run
    const: true
    inputs:
      workflow: 'repo://.github/workflows/ci.yaml'
      continue_on_error: true

  # This op imports coverage — c2 materializes the external artifact into inbox now
  - id: coverage_check
    op: command_execution
    const: true
    artifacts:
      coverage: '${{ sequence.ci.artifacts["coverage-report"] }}'
    inputs:
      run: |
        # coverage-report is materialized in inbox/coverage/
        cat $INBOX/coverage/lcov.info | npx lcov-summary
```

---

## UX Design Decisions

### Decision 1: `actions/checkout` Behavior

Most GHA workflows start with `actions/checkout@v4`. When running via act against a worktree created from a specific commit, this step needs special handling.

**Option A: Let it run.** act's checkout action will clone/fetch into the workspace. This is slow, redundant, and could overwrite the worktree state that Colony2 prepared.

**Option B: Shim it.** Colony2 sets `GITHUB_WORKSPACE` to the worktree and configures act so that `actions/checkout` acts as a no-op. The workspace already contains the committed state at the correct ref.

**Option C: Skip it.** Colony2 pre-processes the workflow YAML and removes `actions/checkout` steps before passing to act.

**Recommendation: Option B.** Shimming is the least surprising — the step appears to run but doesn't destroy existing state. Option C is fragile (many workflows use checkout with custom options like `fetch-depth`).

### Decision 2: Failure Semantics

**Approach:**
- `continue_on_error: false` (default): any job failure → op failure → state machine handles it
- `continue_on_error: true`: op always succeeds, recipe author checks `outputs.status` and `outputs.jobs.*` in transition conditions
- Per-job detail is always available regardless of `continue_on_error`

Matches `command_execution`'s model exactly.

### Decision 3: Secrets

The `secrets` input field maps c2 secret names to GHA secret names. Colony2 passes them to act via `--secret`. Colony2's existing `secret_filter` applies — secrets are redacted from logs and outputs. If `backend: github`, secrets are already configured in the repository settings and don't need to be passed.

### Decision 4: Timeout

The `timeout` input sets the maximum wall-clock time. If exceeded, act is killed, status is `timed_out`, and partial output is captured. Default: 30m. GHA's own `timeout-minutes` on jobs/steps is respected within that outer bound.

### Decision 5: Remote Ref Cleanup

When `backend: github`, Colony2 pushes to a temporary ref. Cleanup strategy:

- Clean up after successful op completion
- If the op fails or Colony2 crashes, a separate `c2 gc refs` command (or periodic job) cleans up stale refs older than a configurable TTL under the `ref_prefix` namespace

---

## Impact on `ticket-validate`

The `ticket-validate` recipe currently detects test frameworks, suggests commands, and runs them via `command_execution`. With `gha.run`, projects with `.github/workflows/` bypass this entirely:

```yaml
validate:
  op: gha.run
  const: true
  transitions:
    - to: validate_fallback
      when: 'outputs.error_message == "workflow_not_found"'
    - to: ready_to_merge_review
      when: "true"
  inputs:
    workflow: 'repo://.github/workflows/ci.yaml'
    continue_on_error: true
    timeout: 30m

validate_fallback:
  op: recipe.run_and_get_result
  transitions:
    - to: ready_to_merge_review
      when: "true"
  inputs:
    name: ticket-validate
    inputs:
      commands: '...'
```

---

## Implementation Boundaries

What Colony2 builds:
- The `gha.run` / `gha.run_job` / `gha.runs` op handlers
- Git selector parsing and validation (shared with recipe/skill ref parser)
- Workflow file fetching and content recording (resolved commit + content hash for non-hash refs)
- Worktree creation from commit hash (`git worktree add`)
- gitea/act library integration with `logrus.Hook` for structured result capture, and `actions/checkout` shimming
- Structured output normalization (act hook events → output schema, GitHub API → output schema)
- GHA-specific materializers for the external artifact pointer system (see [EXTERNAL_ARTIFACTS.md](EXTERNAL_ARTIFACTS.md))
- Remote backend: push to temporary ref, trigger via API, poll, register artifact pointers, fetch thin pack if not `const`, cleanup
- Log capture and registration as external artifacts (`gha-logs`, `gha-logs/<job-id>`)

What Colony2 does NOT build:
- A GitHub Actions runner
- A workflow YAML parser/interpreter (act does this)
- Action compatibility layers (act handles action execution)
- GHA expression evaluation (act does this)

What gitea/act provides (as a Go library):
- Workflow YAML parsing and job scheduling (`model.ReadWorkflow`, `CombineWorkflowPlanner`)
- Action resolution and execution
- Runner environment simulation and Docker container management
- `github.*` context simulation
- Built-in artifact server and cache server
- Service container support
- Structured event delivery via `logrus.Hook`

---

## Act/Gitea Integration Details

Colony2 uses **gitea/act** (gitea's fork of nektos/act) as a Go library rather than invoking act as a CLI. gitea/act is explicitly designed for library use — its README states it "cannot be used as command line tool anymore, but only as a library." This gives Colony2 direct programmatic control over workflow execution and result capture.

### Structured Output

gitea/act does not provide a high-level "get results" API. Instead, Colony2 implements a `logrus.Hook` and injects it via `common.WithLoggerHook(ctx, hook)`. The hook's `Fire(*logrus.Entry)` method receives structured fields for every execution event:

| `entry.Data` field | Type | Content |
|---------------------|------|---------|
| `jobResult` | string | `"success"`, `"failure"`, or `"cancelled"` |
| `stepResult` | `*model.StepResult` | `Conclusion`, `Outcome`, `Outputs` |
| `stepNumber` | int | Step index within the job |
| `stage` | string | `"Main"` or other stage identifier |
| `raw_output` | bool | `true` when the entry contains a log content line |
| `job`, `jobID` | string | Job name and ID |
| `matrix` | map | Matrix values for this job instance |

Colony2's hook accumulates these events and builds the unified output schema (per-job status, per-step results, durations) as the workflow executes. This is the same pattern gitea's own `act_runner` uses — its [reporter](https://gitea.com/gitea/act_runner/src/branch/main/internal/pkg/report/reporter.go) is a reference implementation.

**Go API usage pattern:**

```go
import (
    "github.com/nektos/act/pkg/common"
    "github.com/nektos/act/pkg/model"
    "github.com/nektos/act/pkg/runner"
)

// Parse and plan
workflow, _ := model.ReadWorkflow(reader)
plan, _ := model.CombineWorkflowPlanner(workflow).PlanJob(jobID)

// Configure
config := &runner.Config{
    Workdir:            "/path/to/worktree",
    ArtifactServerPath: "/path/to/artifacts",
    Env:                env,
    Secrets:            secrets,
    // ...
}

// Execute with hook
rr, _ := runner.New(config)
executor := rr.NewPlanExecutor(plan)
ctx = common.WithLoggerHook(ctx, c2Hook)
err = executor(ctx)
```

### Artifact Location

act serves artifacts through a built-in HTTP server, configured via `ArtifactServerPath` in the runner config (CLI: `--artifact-server-path`). Artifacts are written to disk at:

```
{ArtifactServerPath}/{runId}/{itemPath}/{files}
```

Colony2 sets `ArtifactServerPath` to a temporary directory per op execution, then registers `file://` external artifact pointers to the resulting paths. Each GHA artifact name maps to a subdirectory under the run path.

The artifact server supports `actions/upload-artifact` v3 and v4. Related config: `ArtifactServerAddr` (bind address), `ArtifactServerPort` (default `34567`).

### Caching

act has a **built-in cache server** that is enabled by default. It uses BoltDB for metadata and local file storage for cache content, stored at `~/.cache/actcache` by default (configurable via `--cache-server-path`). `actions/cache` works out of the box with no additional configuration.

The cache server supports key lookup with exact match and prefix/regex fallback, matching GitHub's cache behavior. Cache entries are sorted by creation timestamp (newest first).

Colony2 relies on act's built-in cache support — no additional work needed.

### Service Containers

act supports `services:` — it creates a dedicated Docker network, pulls service images in parallel, starts containers with hostname aliases matching service IDs, and polls for health readiness (exponential backoff, up to 5 minutes). Services are cleaned up after job completion.

Colony2 relies on whatever act/gitea support for service containers. No additional work needed.

### Matrix Strategies

act expands matrix combinations into separate job instances, each with its own `RunContext` and Docker container. Naming convention: `{job-name}-{index}` (1-indexed) when there are multiple matrix combinations. If the job has a `name:` field with `${{ matrix.* }}` expressions, those are interpolated first.

In Colony2's output schema, matrix entries appear as separate jobs in the `jobs` map with structured keys that include the matrix values:

```json
{
  "jobs": {
    "test-1": {
      "status": "success",
      "matrix": { "node": "18", "os": "ubuntu-latest" },
      "steps": [...]
    },
    "test-2": {
      "status": "success",
      "matrix": { "node": "20", "os": "ubuntu-latest" },
      "steps": [...]
    }
  }
}
```

The `matrix` field on each job entry contains the matrix values for that instance. Recipe authors can iterate over jobs or check specific combinations in transition conditions.

### Remote Mutating Workflows

When `backend: github` and the op is not `const`, Colony2 pushes to an isolated temporary ref (`refs/c2/gha/<job-id>/<op-hash>`). Since recipes execute serially — only one op runs at a time — there is no possibility of conflicts. The remote ref contains exactly one commit beyond the pushed state, and Colony2 fetches it back as a thin pack with no merge required.
