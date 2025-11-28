# SWF Migration - server/ops

## Objectives
- Run ops under SWF (no Temporal SDK or inline-op abstraction), keeping git/context semantics.
- Remove async child recipe support; enforce sync-only child execution.
- Rework input op to use SWF remote task polling; do **not** register input op in the job worker.

## Scope & Plan (by op and Temporal usage)
- **recipe (pkg/recipe)**: uses Temporal throughout (`op.go`, `child_launcher.go`, `metadata_signal.go`, tests). Replace child workflow/reset/signal APIs with inline compiler calls; remove async mode/handles; convert retry/timeout to recipe-core helpers; use standard `context.Context`.
- **recipe_set (pkg/recipeset)**: `op.go` relies on async children via Temporal child workflows; feature-flag or hard-error and update schema/docs/tests.
- **input (pkg/input)**: `workflow.go` uses Temporal workflows/signals/search attributes; replace with remote task pattern (capability emission + pause until `FindTasksWaitingForCapability`/`TaskHandle.Finish`); do **not** register capability in job worker; adjust SSE helpers to use storage/chapters; standard contexts only.
- **codex.exec (pkg/codex)**: `op.go` uses Temporal activity logger/errors; convert to SWF task style with plain contexts and local error taxonomy; ensure git/blobstore paths passed via inputs.
- **llm ops (pkg/llm)**: activity wrappers/tests currently assume Temporal; replace with SWF-friendly task functions using plain contexts/timeouts; align telemetry with recipe-core errors.
- **extensions (pkg/extensions)**: ensure discovery/register path is SWF-capability oriented (no inline/Temporal registration).
- **export list (pkg/export)**: remove Temporal inline/activity assumptions; exclude disabled ops (recipe_set) until re-enabled.
- **Docs/specs** (`RECIPE_CORE_INPUT_PROVIDER.md`, `recipe-op-execution-modes-spec.md`, etc.): update to SWF contexts, no inline ops, no async.
- **Testing**: update op tests for sync-only behavior; remove async handle assertions; add tests for input remote-task completion; drop Temporal testsuites.

## SWF Gaps Affecting This Project
- No async child execution or child jobs; recipe_set and async run modes must be disabled.
- No signals/search attributes; input flow relies on remote task polling and chapter/state persistence.
- No built-in timers; timeouts/backoff implemented manually.
