# VibeThis Recipe Ops Reference

This reference lists the recipe operations currently registered in VibeThis (`go run ./cmd/... schema`). Each section summarizes the operation's purpose, shows a practical YAML snippet, and calls out key inputs/outputs or configuration knobs.

## command_execution

**Purpose**

Runs an arbitrary shell command in the workflow worker, mirroring GitHub Actions style options for working directory, shell, env, and error handling.

**Example**

```yaml
- id: run-lint
  op: command_execution
  inputs:
    run: npm run lint
    working_directory: web/app
    env:
      NODE_ENV: production
    timeout: 2m
```

**Key configuration**

- `run` (required): command string executed via the chosen shell (`bash`/`sh`/`powershell`/`cmd`).
- `working_directory`: overrides process cwd (defaults to workflow task directory).
- `shell`: explicitly choose shell; defaults to OS-appropriate shell.
- `env`: map of additional environment variables.
- `timeout`: Go-style duration string; cancels the process if exceeded.
- `continue_on_error`: keep the recipe moving even when exit code != 0.

**Outputs**

Returns `stdout`, `stderr`, `exit_code`, flags for `success` and `timed_out`, plus `error_message` when the command fails.

## sleep

**Purpose**

Pauses workflow execution for a fixed duration, useful for backoff or scheduling gaps.

**Example**

```yaml
- id: wait-for-cache
  op: sleep
  inputs:
    duration: 30s
```

**Key configuration**

- `duration` (required): Go duration string (e.g. `5s`, `1m`, `500ms`).

**Outputs**

Returns `start_time`, `end_time`, the `slept` duration, and boolean flags `completed` / `interrupted` to show whether the pause finished normally.

## recipe

**Purpose**

Invokes another recipe as a child workflow with managed git context propagation and optional async execution.

**Example**

```yaml
- id: run-validation
  op: recipe
  config:
    recipe: validation::full
    timeout: 20m
  inputs:
    git_state: discrete        # shared (default) or discrete
    run_mode: sync             # sync (default) or async
    inputs:                    # forwarded to the child recipe
      target: '{{ inputs.cell }}'
      ticket_id: '{{ context.ticket.id }}'
```

**Key configuration**

- `config.recipe` (required): child recipe slug to invoke.
- `config.timeout` / `config.retry_policy`: overrides Temporal options for the child run.
- `git_state`: `shared` keeps the parent workspace; `discrete` provisions a detached workspace.
- `run_mode`: `sync` waits for child completion; `async` returns an `async_handle` with workflow IDs.
- `inputs`: arbitrary key/value payload passed to the child recipe.

**Outputs**

For synchronous runs, returns the child recipe outputs plus propagated `context` and `git_persist_hash`. Async runs emit `async_handle` (workflow metadata) so callers can poll or join later.

## recipe_set

**Purpose**

Fans out multiple recipe invocations in parallel, automatically using discrete async children and waiting for them all to finish.

**Example**

```yaml
- id: parallel-smoke
  op: recipe_set
  inputs:
    recipes:
      - recipe: release::smoke
        input:
          env: staging
      - recipe: release::smoke
        input:
          env: production
      - recipe: lint::full
        input:
          target: ui-app
```

**Key configuration**

- `recipes` (required, min 1): array of child descriptors. Each entry accepts the same keys as the `recipe` op (`recipe`/`name`, `input`/`inputs`, etc.).
- Per-child extras (e.g. `timeout`) are forwarded, but `git_state` and `run_mode` are enforced as discrete+async.

**Outputs**

Returns nothing on success. If any child fails, the op throws a non-retryable error with indexed status records describing each child outcome.

## input

**Purpose**

Suspends the workflow until a user supplies form input via the Cortex UI or API, supporting single question and multi-field forms.

**Example**

```yaml
- id: request-approver
  op: input
  inputs:
    box_id: '{{ context.box.id }}'
    activity_id: approval-gate
    config:
      question: "Approve deploying to production?"
      type: multiple_choice
      options:
        - label: Approve
          value: approve
        - label: Reject
          value: reject
      timeout: 600
```

**Key configuration**

- `box_id` / `activity_id` (required): identifies the UI inbox entry.
- `config.question`+`type`: single-question mode (short answer, paragraph, multiple choice, etc.).
- `config.fields`: array of rich form field definitions for multi-question flows.
- `config.timeout`: seconds before the op times out (defaults to 300s).
- `default_on_timeout`: value to auto-return when a timeout occurs.

**Outputs**

Provides either `response` (single question) or `fields` map (multi-form), plus `user_id` and optional metadata.

## codex.exec

**Purpose**

Runs Codex CLI in non-interactive mode inside the managed workspace, streaming results to blob storage and normalizing agent state.

**Example**

```yaml
- id: codex-plan
  op: codex.exec
  inputs:
    prompt: |
      Analyze the diff in ui-app and summarize UI regressions.
    sessionId: prev-session-123    # resume instead of a fresh run
    model: gpt-5-codex
    env:
      CODEX_API_KEY: '{{ secrets.codex }}'
    context:
      worktree: '{{ context.git.worktree_path }}'
      blobstore: '{{ context.git.blob_store_uri }}'
      git:
        base_hash: '{{ context.git.base_hash }}'
        persist_hash: '{{ context.git.persist_hash }}'
```

**Key configuration**

- `prompt` (required): task directive for the agent.
- `sessionId`: resume an earlier Codex session.
- `model`: override the default Codex model variant.
- `env`: extra environment variables passed to the Codex CLI.
- `context` (required): must supply `worktree` and `blobstore`; enrich with git metadata for better grounding.

**Outputs**

Returns run `status`, `sessionId`, `assistantSummary`, captured `stderr`, pending dependency hints, and a `stdoutBlobUri` pointing to the streamed transcript/stdout payload (plus `errorMessage` when Codex exits early).

## llm_inference

**Purpose**

Executes a basic LLM completion against OpenAI, Anthropic, or Gemini adapters with optional JSON-schema responses.

**Example**

```yaml
- id: summarize
  op: llm_inference
  inputs:
    provider: openai
    model: gpt-4-turbo
    prompt: |
      Summarize the risk section from the attached RFC.
    system_prompt: "You are a diligent release manager."
    temperature: 0.3
    response_schema: |
      {"type":"object","properties":{"summary":{"type":"string"}}}
```

**Key configuration**

- `provider` / `model` (required): adapter (`openai`, `anthropic`, `gemini`, …) and model name.
- `prompt` (required) and optional `system_prompt`.
- Sampling controls: `temperature`, `max_tokens`, `top_p`, `stop_sequences`.
- `response_schema`: JSON schema to request structured output.

**Outputs**

Returns raw `response` (text or structured JSON), `model`, `finish_reason`, and token usage statistics.

## llm_inference2

**Purpose**

Enhanced LLM execution that adds persona presets, file context ingestion, tool invocation, sandboxed command execution, and richer telemetry.

**Example**

```yaml
- id: design-review
  op: llm_inference2
  inputs:
    default_provider: anthropic
    default_model: claude-3-sonnet
    prompt: "Evaluate the UX copy in docs/feature.md"
    files:
      - path: docs/feature.md
        media_type: text/markdown
    execute_tools: true
    enable_tool_execution: true
    tools:
      - name: shell
        description: "Run read-only shell commands"
        parameters:
          type: object
          properties:
            command:
              type: string
    tool_working_dir: docs
    tool_timeout: 90s
    max_tool_rounds: 3
    continue_on_tool_error: false
    enable_sandbox: true
    allowed_paths: [docs]
    metadata:
      review_type: ux-copy
```

**Key configuration**

- Includes every `llm_inference` knob (`prompt`, `system_prompt`, sampling, `response_schema`).
- Provider defaults live in `default_provider` / `default_model`; override credentials and persona metadata with `api_keys` and `metadata`.
- File ingestion: supply `files`, choose `file_handling` (`native`, `text_fallback`, `hybrid`), set `default_file_handling`, cap size with `max_file_context_size`, and control workspace with `default_working_dir`.
- Tooling support: register `tools`, enable globally via `enable_tool_execution` and per call with `execute_tools`; combine with `tool_working_dir`, `tool_timeout`, `max_tool_rounds`, and `continue_on_tool_error`.
- Sandbox guards: `enable_sandbox`, `allowed_paths`, `restricted_paths`.

**Outputs**

Extends basic output with `tool_calls`, `tool_results`, file read/write tracking, execution time, provider metadata, and legacy `telemetry` for compatibility.

## git_file_collector

**Purpose**

Collects a curated set of repository files (optionally staged/untracked) for LLM consumption, enforcing size limits and metadata capture.

**Example**

```yaml
- id: capture-scope
  op: git_file_collector
  inputs:
    context_dir: '{{ context.git.worktree_path }}'
    file_patterns: ["**/*.go", "**/*.md"]
    exclude_patterns: ["**/*_test.go", "vendor/**"]
    max_total_size: 500000
    include_staged: true
    exclude_binary: true
    auto_detect_type: true
```

**Key configuration**

- `context_dir` (required): path inside a git repo.
- `file_patterns` / `exclude_patterns`: glob lists to include or omit files.
- Size controls: `max_file_size`, `max_total_size`.
- Git switches: `include_staged`, `include_untracked`, `use_gitignore`.
- Content switches: `exclude_binary`, `auto_detect_type`, `include_metadata`.

**Outputs**

Returns collected `files` (path + content + metadata), aggregate `file_count`, cumulative `total_size`, repository status, and detailed `statistics` buckets (by type, extension, skipped reasons, total size).

## git_shallow_clone

**Purpose**

Copies a local repository into a target directory as a shallow clone at a specific commit—ideal for preparing detached workspaces.

**Example**

```yaml
- id: stage-repo
  op: git_shallow_clone
  inputs:
    source_dir: '{{ context.git.worktree_path }}'
    target_dir: '{{ workspace }}/clones/ui-app'
    commit_hash: '{{ context.git.persist_hash }}'
```

**Key configuration**

- `source_dir` (required): existing git repository root.
- `target_dir` (required): destination directory (created as needed).
- `commit_hash` (required): commit to checkout.

**Outputs**

Returns `cloned_path` pointing to the freshly prepared worktree.

## git_persist_commit

**Purpose**

Creates a thin-pack representation of the current workspace state and records commit metadata for later restore or sharing.

**Example**

```yaml
- id: persist-work
  op: git_persist_commit
  inputs:
    repo_path: '{{ context.git.worktree_path }}'
    storage_location: s3://artifacts/thin-packs/ui-app
    root_hash: '{{ context.git.base_hash }}'
    commit_message: "Checkpoint after auto-fix"
    author: "CI Bot <ci@example.com>"
```

**Key configuration**

- `repo_path`, `storage_location`, `root_hash` are required.
- Optional `commit_message`, `author`, `timeout` (duration).

**Outputs**

Includes `commit_hash`, `parent_hash`, `thin_pack_path`, `thin_pack_size`, and `created_at` timestamp.

## git_restore_commit

**Purpose**

Restores a workspace to a previously persisted commit, using thin packs when necessary.

**Example**

```yaml
- id: restore-checkpoint
  op: git_restore_commit
  inputs:
    repo_path: '{{ context.git.worktree_path }}'
    target_commit: '{{ inputs.target_commit }}'
    root_hash: '{{ inputs.root_hash }}'
    storage_location: s3://artifacts/thin-packs/ui-app
    force: false
```

**Key configuration**

- `repo_path`, `target_commit`, `root_hash`, `storage_location` required.
- `force`: bypass dirty working tree guard.
- `timeout`: duration to bound the restore process.

**Outputs**

Returns `success` flag, `current_commit`, where it was `restored_from`, any `thin_packs_applied`, and `restored_at` timestamp.

## thinpackrebase

**Purpose**

Rebases a thin-pack backed workspace onto a new base commit while preserving metadata required for subsequent persist operations.

**Example**

```yaml
- id: refresh-base
  op: thinpackrebase
  inputs:
    target_base_hash: '{{ inputs.target_base }}'
    repo_path: '{{ context.git.worktree_path }}'
    upstream_remote: origin
    preserve_author: false
    update_refs: refs/heads/feature-refresh
    context: '{{ context }}'
```

**Key configuration**

- `target_base_hash` (required): commit to rebase onto.
- `repo_path`: overrides context-derived worktree.
- `upstream_remote`: fetch remote to validate availability.
- `preserve_author`: retain original commit authorship when replaying.
- `update_refs`: optional ref to update after the rebase.
- `context`: pass current git context when `repo_path` is omitted.

**Outputs**

Provides `target_base_hash`, `new_base_hash`, `new_persist_hash`, optional `updated_ref`, `rebased_from` summary, and a `git_context_patch` map for downstream ops.

## squashrebasemerge

**Purpose**

Squashes local commits, rebases onto the latest target branch tip, and pushes the consolidated result back to the remote.

**Example**

```yaml
- id: finalize-feature
  op: squashrebasemerge
  inputs:
    repo_path: '{{ context.git.worktree_path }}'
    target_branch: refs/heads/main
    upstream_remote: origin
    preserve_author: true
    context: '{{ context }}'
```

**Key configuration**

- `repo_path`: workspace to operate on (defaults from context).
- `target_branch`: ref to fast-forward/push (defaults to `refs/heads/main`).
- `upstream_remote`: remote name that hosts the target branch.
- `preserve_author`: keep original author metadata when squashing.
- `skip_rebase`: skip the rebase step and require the local tip to fast-forward the remote.
- `context`: supply when repo path not provided.

**Outputs**

Returns `target_branch`, `remote_ref`, `merged_hash`, summary of squashed commits, a boolean `fast_forward` indicator, and a `git_context_patch` for updated hashes.

## ticket.manage

**Purpose**

Executes a batch of ticket lifecycle actions (create, update, notes, markdown links, workflow events, reset) with optimistic locking and contextual telemetry.

**Example**

```yaml
- id: update-ticket
  op: ticket.manage
  inputs:
    ticket_id: '{{ context.ticket.id }}'
    actions:
      - type: create_ticket
        cell: core
        title: "Investigate failing integration tests"
        stage: triage
        state: open
        actor:
          type: agent
          agent:
            cell: release-bot
            workflow_name: '{{ context.recipe.name }}'
      - type: append_ticket_note
        note: "Reproduced failure in CI. Rolling back deployment."
      - type: update_ticket
        expected_version: '{{ outputs.ticket.version }}'
        state: in_progress
```

**Key configuration**

- `ticket_id`: optional; omit when the first action is `create_ticket`.
- `actions` (required): ordered list of action objects. Supported types:
  - `create_ticket`: requires `cell`, `title`, `stage`, `state`.
  - `update_ticket`: requires `expected_version`; set new `stage`, `state`, or `description`.
  - `append_ticket_note`: requires `note`; optional actor and timestamp.
  - `link_markdown_doc` / `override_markdown_doc` / `remove_markdown_doc`: manage associated docs via `name`, `path`, `reason`.
  - `append_workflow_event`: requires `workflow_id`, `run_id`, `status` (see `ticket.WorkflowEventType`).
  - `reset_ticket`: requires `reason`, optional `anchor_event_id`.
- Each action supports an `actor` block describing either a user email or agent identity.

**Outputs**

Returns the updated `ticket` (when available), per-action `results` (ticket/event/reset objects with timestamps), and `context_patch` keys (`ticket.id`, etc.) for downstream propagation.
