# Nucleus `job run` Command Implementation Plan

## Goal

Add a new `nucleus` CLI command that can attach to an existing recipe job and try to advance it locally against an SWF remote runtime.

The command should:

- start from an existing job ID plus an SWF remote runtime URL
- acquire work for that job with `swf.GetJobForRun`
- execute the recipe job with the same `recipe-worker` runtime used elsewhere
- show recipe-centric progress while it runs
- support repeated invocations; each run should resume from persisted SWF state
- stop cleanly when the job is complete, terminally failed, or blocked on external work beyond a configurable wait budget
- support interactive input handling in a terminal, and non-interactive/CI handling where input steps are surfaced in output instead of opening a TUI

## Important Constraints Found In The Current Codebase

### 1. SWF job identity is `{tenantId, jobId}`, not just `jobId`

`swf.GetJobForRun` and the remote runtime both require a full `swf.JobKey`.

The SWF remote API is tenant-scoped:

- `/v1/tenants/{tenantId}/jobs/{jobId}`

So "job id + remote workflow URL" is only sufficient if the URL already encodes tenant/project scope. We need to make that explicit in the CLI contract.

Recommended approach:

- accept `--job-id`
- accept `--swf-url`
- also accept either:
  - `--tenant-id`, or
  - a full job URL form that can be parsed into `{baseURL, tenantId, jobId}`
- allow `tenantId` and `swf-url` to be sourced from environment when flags are omitted

Do not hide this requirement inside runtime errors.

### 2. `ops` + `recipe-worker` are not enough, and `nucleus` cannot assume a database

The `input` op is registered separately in [`recipe-input`](./recipe-input), not in [`ops`](./ops) or [`recipe-worker`](./recipe-worker):

- [`/src/server/ops/pkg/export/exports.go`](/src/server/ops/pkg/export/exports.go)
- [`/src/server/recipe-worker/pkg/export/exports.go`](/src/server/recipe-worker/pkg/export/exports.go)
- [`/src/server/recipe-input/pkg/input/activity.go`](/src/server/recipe-input/pkg/input/activity.go)

If this command must run jobs containing `input`, we either:

- add a direct dependency on `server/recipe-input`, or
- extract shared input inspection/submission helpers into a reusable package

Recommendation:

- depend on `server/recipe-input` for v1
- if we want a stricter dependency boundary later, extract shared non-HTTP helpers from [`/src/server/recipe-input/pkg/input/management.go`](/src/server/recipe-input/pkg/input/management.go)

More broadly, server execution parity currently comes from [`/src/server/api/pkg/serverdeps/opssetup/opssetup.go`](/src/server/api/pkg/serverdeps/opssetup/opssetup.go), which registers:

- `ops`
- `recipe-worker`
- `recipe-input`
- `recipe-child`
- `git`
- `ticket`

That full registration path is not appropriate for `nucleus`.

This command path should be treated as a remote-execution client with no database connection of its own. It should operate through SWF runtime access and `workflowctl.WorkflowControl`, not through `deps.Database()`.

That means:

- do not construct or inject a DB handle for `nucleus`
- do not register ops whose execution path requires database access
- do not register management/service wiring that assumes HTTP or DB-backed state
- do not aim for blind "full server parity" if parity would pull in DB-coupled behavior

There are already concrete examples of DB-bound behavior in the current tree:

- some ops under `server/ops` call `deps.Database()` directly
- `recipe-worker` passes `deps.Database()` into op execution when available
- `recipe-child` currently has a DB-backed transactional path for multi-child creation

Recommendation:

- extract op registration out of `server/api`, but make it profile-based rather than a single "register everything" path
- add a `nucleus` registration profile that includes only ops proven to work with SWF/workflow-control-only dependencies
- keep the registry composition explicit and curated; unsupported DB-dependent ops should fail at startup because they were never registered, not later at runtime after partial execution

For the new module, assume v1 direct dependencies include at least:

- `server/ops`
- `server/recipe-worker`
- `server/recipe-input`
- `server/recipe-child`

But dependency presence does not imply "register everything from this module." The `nucleus` profile should include only the subset that is safe without DB access.

### 3. `recipe-child` multi-start should stop using DB transactions

The multi-child recipe path currently tries to create all child jobs inside a DB transaction when a database handle is present.

That is the wrong shape for `nucleus`, and it is also an unnecessary coupling for the longer-term op model we want here.

Recommendation:

- update `server/recipe-child/pkg/recipe/launcher.go` so multi-child start always creates jobs in sequence via `WorkflowControl.StartJob`
- remove or ignore the DB-transactional branch for child creation
- accept partial creation semantics for now if the Nth child create fails after the first `N-1` succeeded
- defer idempotency / compensating behavior to later follow-up work

This change should be called out as part of the `nucleus` preparation work, because `recipe-child` is one of the ops we do want available in the remote/no-DB runtime profile.

### 4. We already have a recipe-centric replay/story seam

The workflow story implementation already proves that we can inject a custom recipe executor via `compiler.NewRecipeJobWorker`:

- [`/src/server/recipe-worker/pkg/compiler/job_worker.go`](/src/server/recipe-worker/pkg/compiler/job_worker.go)
- [`/src/server/workflow/internal/story/builder.go`](/src/server/workflow/internal/story/builder.go)

We should reuse this pattern instead of building a second independent recipe traversal stack.

### 5. Additional CEL functions need to move out of `server/cortex`

Current CEL wiring is split:

- default/shared functions like `jq` already come from `recipe-template` funcregistry defaults
- artifact helper functions are still registered inside [`/src/server/cortex/internal/setup/artifact_cel_functions.go`](/src/server/cortex/internal/setup/artifact_cel_functions.go)
- the builder assembly currently happens inside [`/src/server/cortex/internal/setup/setup.go`](/src/server/cortex/internal/setup/setup.go)

That is the wrong ownership for `nucleus`, because it needs the same CEL behavior without depending on `server/cortex/internal/...`.

Recommendation:

- create a shared package that assembles the canonical Colony CEL provider
- move artifact CEL helper registration into that shared package
- keep `funcregistry.NewBuilder().WithDefaults()` as the base so `jq` and the other default functions remain available everywhere
- have both `cortex` and `nucleus` construct their `CELOptionsProvider` from the same shared package

Suggested package shape:

- `server/recipe-template/pkg/colonycel`

Suggested API:

```go
type Options struct {
    CellsService cell.Service // optional
}

func NewBuilder(opts Options) *funcregistry.Builder
func RegisterArtifactFunctions(b *funcregistry.Builder)
```

Notes:

- keep stateless helpers like artifact functions in the shared package unconditionally
- keep context/service-backed helpers like `cells()` behind optional dependency injection
- ensure `nucleus` uses this shared builder when creating its `CELOptionsProvider`

## Proposed Module Layout

Create a new module:

- `/src/server/nucleus`

Initial structure:

- `go.mod`
- `project.json`
- `cmd/nucleus/main.go`
- `internal/cmd/root.go`
- `internal/cmd/job_run.go`
- `internal/runtime/remote.go`
- `internal/runjob/service.go`
- `internal/runjob/options.go`
- `internal/runjob/progress.go`
- `internal/runjob/listener.go`
- `internal/runjob/ready_policy.go`
- `internal/runjob/wait_loop.go`
- `internal/runjob/input_bridge.go`
- `internal/tui/...`
- `internal/ci/...`

Repo wiring:

- add `./server/nucleus` to [`/src/go.work`](/src/go.work)
- add `/src/server/nucleus/project.json` following the existing Go project pattern
- add build/install targets for the new app

## Command Surface

Recommended command shape:

```text
nucleus job run --job-id <id> --swf-url <url> [flags]
```

Required flags:

- `--job-id`
- `--swf-url` or `NUCLEUS_SWF_URL`

Identity flags:

- `--tenant-id` or `NUCLEUS_TENANT_ID`
- optionally `--job-url` as an alternative to `--job-id` + `--tenant-id`

Runtime flags:

- `--wait-timeout 15m`
- `--poll-interval 5s`
- `--lease-duration 60s`
- `--await-threshold 30s`
- `--worker-id <id>`

Non-ready handling flags:

- `--on-not-ready wait|fail|fail-on-lease|fail-on-missing-capability|fail-on-pending-jobs|fail-on-future`
- optionally allow repeats: `--fail-on-status ACTIVE --fail-on-status PENDING_JOBS`

Interaction flags:

- `--ci`
- `--no-tui`
- `--input-mode tui|ops|fail`
- `--json`

Output flags:

- `--progress text|json|story`
- `--quiet`

## Execution Model

### Phase 1: Build the runtime and worker set

1. Construct an SWF remote runtime with `swf/runtime/remote`.
2. Register only the ops that are safe in the no-DB `nucleus` runtime profile.
3. Build the activity registry from `recipe-worker`.
4. Build the shared Colony CEL provider and pass it into recipe execution.
5. Build a recipe job worker with:
   - the standard compiler/runtime behavior
   - a decorated executor for recipe-centric progress capture
   - optional recipe-loaded callback

We should not depend on `server/api` setup code from the CLI app. The new module should build only the runtime and worker registration it can actually support without a DB connection.

Recommended implementation:

- move op registration out of `server/api/pkg/serverdeps/opssetup`
- place it in a shared package with no HTTP/web dependency
- make registration profile-based, for example `server` vs `nucleus-remote`
- have `server/api` use the full server profile
- have `server/nucleus` use the curated no-DB profile
- move cortex-only artifact CEL registration into the shared Colony CEL package
- use the same shared CEL builder from both `server/cortex` and `server/nucleus`

Runtime assumption for `nucleus`:

- remote stateful coordination goes through SWF runtime access and `WorkflowControl`
- local execution helpers such as artifact/job tooling and worktree support are available as needed for recipe execution
- `deps.Database()` is not available and must not be required by registered ops

### Phase 2: Claim a specific job

Use:

- `swf.GetJobForRun(ctx, runtime, swf.GetJobForRunRequest{...})`

This gives us:

- a runnable with a lease already acquired, or
- an immediate classified outcome (`COMPLETED`, `FAILED`, `SUSPENDED`, `NOT_LEASEABLE`)

This is the right abstraction for this command because it already encapsulates:

- lease acquisition
- capability matching
- suspended/non-leaseable classification
- post-run outcome classification

### Phase 3: Run once

Call:

- `runnable.Run(listener)`

Use the listener for task/job lifecycle only when it adds value beyond executor-driven recipe progress.

Recommendation:

- make the recipe-executor recorder the primary progress source
- keep `JobRunListener` as a secondary event stream for raw SWF timing/status information

## Progress Rendering Strategy

### Primary source: decorated recipe executor plus a hierarchical story model

Implement a `RecipeExecutor` decorator, similar in spirit to the workflow story builder, that emits progress events for:

- attempt/invocation entered
- recipe loaded
- sequence entered/exited
- attempt loop entered/exited
- op entered/exited
- op step started/completed
- state machine entered/exited
- state entered/exited
- transition evaluation
- input op reached
- child-job wait reached

This gives us the recipe-centric view the user wants, but the renderer should not present it as a flat task log.

Recommendation:

- maintain an in-memory hierarchical progress tree that mirrors the story model used by the web UI
- render progress as nested execution scopes, for example recipe -> attempt -> sequence -> op / state machine -> op step / state / transition
- use indentation, scope enter/exit rows, and stable path keys so the output reads like a walk of the execution tree
- review [`/src/web/WORKFLOW_STORY_UI_SPEC.md`](/src/web/WORKFLOW_STORY_UI_SPEC.md) and [`/src/web/app/src/components/WorkflowStoryPage.tsx`](/src/web/app/src/components/WorkflowStoryPage.tsx) for the tree/disclosure model we want to approximate in CLI/TUI form

### Secondary source: `JobRunListener`

Use `JobRunListener` to enrich progress with:

- SWF attempt number
- task ordinal
- task timing
- raw task errors

Do not make the UI depend solely on raw task names.

### Replay/story support

On startup, before attempting execution, load a story/progress baseline from persisted job state:

- either reuse the workflow story builder logic directly
- or extract the reusable recorder/executor instrumentation from `server/workflow/internal/story`

Recommended v1:

- extract the recorder/decorated-executor pieces needed for a local "current progress snapshot"
- avoid depending on `internal` workflow packages from the new module
- reconstruct the same hierarchical story before doing new work and feed it through the same renderer used for live execution
- mark replayed/cached nodes with a subtle visual embellishment in text/TUI output so users can distinguish "historical durable step" from "currently executing step"

Behavior requirements:

- if the process crashes and is restarted, the next run should render the same progress tree the user saw previously, now marked as cached where appropriate, and then continue from the next durable step
- if the job is already completed, `nucleus job run` should still render the full historical story as if it had been observed live the first time, except every displayed step is marked cached/replayed

This lets repeated `nucleus job run` invocations show where the job already is before doing new work, and makes completed jobs readable instead of returning a terse "already done" message.

## Non-Ready And Suspended Handling

`swf.GetJobForRun` can classify a job as suspended or not leaseable and exposes:

- `JobStatus`
- `NextNeed`
- `WaitForJobIDs`
- `MissingCapability`

These map cleanly to the requested CLI behaviors.

### Proposed policy model

Define:

```go
type NotReadyPolicy struct {
    Mode                 string
    FailOnActiveLease    bool
    FailOnPendingJobs    bool
    FailOnAwaitingFuture bool
    FailOnMissingCap     bool
}
```

Behavior:

- `COMPLETED`: return success
- `FAILED`: return failure with job error
- `SUSPENDED` or `NOT_LEASEABLE`:
  - inspect `JobStatus`, `MissingCapability`, `WaitForJobIDs`
  - either fail immediately or enter the wait loop

### Wait loop semantics

When blocked on external work, the command should:

1. print the blocking reason
2. wait up to `--wait-timeout`
3. re-check with `GetJobForRun`
4. if the job becomes runnable, execute one run attempt
5. repeat until:
   - complete
   - terminal failure
   - timeout budget exhausted

Default:

- wait on blocked external work for a bounded time
- exit non-zero on timeout if the job is still incomplete

This matches the user requirement and preserves the normal SWF retry/resume model.

## Input Operation Handling

### Shared input bridge

We should not duplicate the logic from [`/src/server/recipe-input/pkg/input/management.go`](/src/server/recipe-input/pkg/input/management.go), and we should not leave it trapped behind an HTTP-only management service.

Refactor the input management path so the core input-runtime behavior is usable in both HTTP and local/CLI contexts. The HTTP management service should become a thin wrapper over that shared core.

The shared/local core should be able to:

- detect whether the current waiting task is `input:collect_user_input`
- decode the prior `input:generate_form` output into `InputForm`
- submit a `FormResponse` by finishing the waiting task with the correct envelope/artifacts

Recommended extraction/refactor:

- add exported helpers under `server/recipe-input/pkg/input`
- or add a small subpackage like `server/recipe-input/pkg/inputruntime`
- keep the transport-agnostic logic there
- have both the HTTP management service and `nucleus` call into that shared implementation

The reusable surface should accept:

- `workflowctl.WorkflowControl` or direct SWF runtime access
- `project/tenant ID`
- `job ID`

And expose:

- `GetPendingInput(jobKey) (*UserInputDetails, error)`
- `SubmitInput(jobKey, FormResponse) error`
- `CancelInput(jobKey, reason string) error`

### Interactive mode

If attached to a TTY and not in CI:

- render input forms in a TUI
- support both single-question and multi-field forms
- preserve artifact/context metadata if useful for display

Recommendation:

- use Bubble Tea v2 for v1

Reasons:

- simpler ecosystem fit for form flows
- easier testing/model composition
- enough control to render a structured step view plus forms
- lower integration cost than adopting Vaxis directly for the first version

Design the form renderer behind an interface so Vaxis can be swapped in later if needed.

### CI mode

If `--ci` or no TTY:

- do not launch TUI
- include pending input requests in the command output stream as machine-readable "ops"
- exit with a distinct non-zero code when user input is required and no auto-response exists

Recommended CI output event shape:

```json
{
  "kind": "input_required",
  "job_id": "...",
  "tenant_id": "...",
  "form": { ... },
  "blocking": true
}
```

This keeps CI consistent with the requested "input operations should be included amongst ops."

## Resume Semantics

We do not need custom resume storage.

Resume should rely on existing SWF persistence:

- `GetJobForRun` claims the current executable point
- cached chapters prevent re-running completed work
- retries/restarts continue from persisted state

The CLI must therefore be stateless apart from its current process:

- no local checkpoint database
- no local resume token

## Proposed Internal Architecture

### `internal/runjob/service.go`

Owns the top-level orchestration:

- validate options
- build runtime/workers
- resolve job key
- pre-load progress snapshot
- run wait/claim/execute loop
- map outcomes to exit codes

### `internal/runjob/progress.go`

Defines a neutral event model:

```go
type ProgressEvent struct {
    Kind      string
    JobKey     swf.JobKey
    Attempt    int
    Path       []string
    TaskType   string
    Ordinal    *int64
    Message    string
    Payload    any
    At         time.Time
}
```

Consumers:

- text renderer
- JSON renderer
- TUI model

### `internal/runjob/listener.go`

Adapts `JobRunListener` into `ProgressEvent`s.

### `internal/runjob/ready_policy.go`

Maps SWF outcome/status into CLI decisions.

### `internal/runjob/wait_loop.go`

Handles:

- suspended polling
- timeout budget
- status transitions
- sleep/backoff

### `internal/runjob/input_bridge.go`

Wraps extracted `recipe-input` helpers and presents:

- detect pending input
- render or serialize input request
- submit response

## Testing Plan

### Unit tests

- CLI flag parsing and validation
- job key resolution from flags/URL
- non-ready policy decisions
- wait-loop timeout and retry behavior
- exit code mapping
- JSON event rendering
- op-profile selection excludes DB-dependent ops from the `nucleus` runtime
- shared CEL builder includes defaults plus Colony-specific helpers

### Integration tests against remote runtime wrapping toy runtime

Use the real remote-runtime client path in tests, but back it with the toy runtime from `swf-go` so behavior stays fast and deterministic.

Coverage:

- completed job
- failed job
- suspended on missing capability
- suspended on pending child jobs
- suspended on future await
- repeated invocation resumes correctly
- remote claim and run
- remote list/status polling
- remote input submission
- already-completed job renders full cached history
- restarted run replays cached history then continues live

### Recipe-worker integration tests

Cover real recipe execution with registered ops:

- command op recipe
- sleep/wait recipe
- child-job wait recipe
- multi-child recipe start executes child creation sequentially without DB transactions
- input recipe with interactive bridge mocked

### `recipe-child` regression tests

- update the multi-start tests to assert sequential `StartJob` calls rather than transactional DB usage
- add/keep coverage for partial-create failure semantics until idempotency work lands

### TUI tests

Keep them narrow:

- render single-question form
- render multi-field form
- submit response path
- cancel/quit behavior

## Exit Codes

Define explicit exit codes instead of a single generic failure:

- `0`: job completed
- `1`: terminal job failure / unexpected command failure
- `2`: timed out waiting on external work
- `3`: input required in non-interactive mode
- `4`: job not runnable under selected `--on-not-ready` policy
- `5`: invalid CLI usage / unresolved tenant-job identity

## Implementation Phases

### Phase 1: bootstrap module and non-interactive run

- create `server/nucleus`
- add Cobra app scaffold
- add remote runtime bootstrap
- add job key resolution
- add env sourcing for `tenantId` and `swf-url`
- add shared no-DB op registration profile
- exclude DB-dependent ops from the `nucleus` profile
- update `recipe-child` multi-start to create children sequentially without DB transactions
- extract and wire the shared Colony CEL builder
- add `GetJobForRun` + `Run` path
- add text/json status output

Acceptance:

- command can claim and run a simple recipe job to completion against remote runtime
- registered ops for this path do not require `deps.Database()`

### Phase 2: suspended handling and wait policy

- add `--wait-timeout`
- add `--on-not-ready`
- add polling/retry loop
- add exit code mapping

Acceptance:

- command behaves correctly for active lease, pending jobs, future waits, and missing capability

### Phase 3: recipe-centric progress

- add executor decorator
- add hierarchical progress/tree model
- add text/json progress renderers that walk the hierarchy instead of printing a flat log
- preload replay/story baseline before any new execution
- mark cached/replayed steps distinctly in the renderer

Acceptance:

- user sees recipe/attempt/sequence/state-machine/op/step-oriented progress rather than only raw task names
- restart/completed runs show the same story structure with cached markers

### Phase 4: input bridge

- refactor `recipe-input` management logic into a shared local/non-HTTP core
- detect pending input
- submit responses programmatically
- add CI-mode input event emission

Acceptance:

- non-interactive mode surfaces input requests without hanging
- TUI and HTTP input handling both use the same underlying input-runtime implementation

### Phase 5: TUI

- add Bubble Tea v2 app
- render forms and progress
- wire submission/cancel

Acceptance:

- local terminal runs can complete input steps fully within the command

## Recommended Early Decisions

1. Treat `tenantId` as a first-class CLI concept. Do not pretend job ID alone is enough.
2. Reuse `swf.GetJobForRun` as the authoritative claim/run primitive.
3. Reuse `compiler.NewRecipeJobWorker` with an injected executor decorator for recipe-centric progress.
4. Refactor `recipe-input` management logic into a shared local/non-HTTP core rather than copying or reimplementing the HTTP handler path.
5. Use Bubble Tea v2 for v1 TUI, with a renderer interface so we can swap or augment later.
6. Move the Colony-specific CEL builder and artifact helper registration into a shared package consumed by both `cortex` and `nucleus`.
7. Treat `nucleus` as a no-DB runtime and register only ops that can run with SWF/workflow-control-only dependencies.
8. Change multi-child recipe creation to sequential `StartJob` calls now; defer idempotency and transactional semantics work.
9. Render a hierarchical story-shaped progress view, not a flat log, and use cached/replayed markers on restart/completed runs.
10. Keep the CLI stateless and rely on SWF persistence for resume/retry behavior.

## Open Questions

1. What exact form of "SWF remote workflow URL" should the CLI accept?
   - base runtime URL
   - tenant-scoped URL
   - full job URL

2. Is interactive input support required in v1, or can it land behind `--experimental-tui` after CI/non-interactive support is working?

3. What exact op allowlist should ship in the first `nucleus` profile?
   - the direction is fixed: exclude DB-dependent ops
   - we still need to decide the initial safe subset within `ops`, `recipe-worker`, `recipe-input`, and `recipe-child`

4. Do we want a separate `nucleus job watch` later, or should `job run` own both execution and progress watching indefinitely?

## Suggested First PR

Keep the first PR narrow:

- bootstrap `server/nucleus`
- add `nucleus job run`
- remote runtime bootstrap
- full job-key resolution
- shared no-DB op registration profile
- `recipe-child` multi-start sequentialization
- shared Colony CEL builder extraction
- `GetJobForRun` execution
- suspended wait loop
- plain text + JSON output
- tests for completion/failure/suspended cases

Leave TUI and input submission for a follow-up PR after the command can reliably execute non-input jobs with the constrained remote profile.
