# Job story replay investigation — 2026-10-03

Update, 2026-10-04: Cortex now pins c2j
`v0.0.59-0.20261004003418-6af179613628`, incorporating fixes for timed-form
retrieval and terminal outcome status. JobDB remains at v0.0.23. See the
[timed-input report](bugs/timed-input-v0.0.58.md) for the resolution. The
observations below describe the original database investigation against v0.0.58;
the separate story-summary issue is not claimed resolved by these changes.

## Reproduction and dependency upgrade

The supplied `problemdb/jobdb.db` contains tenant `c2`, job
`3KCnRK2LrEmd0H3nwmxcOaqGryg`, recipe `build`. Investigation used a copy under
`/tmp/cortex-problemdb-investigation`, served by JobDB v0.0.23. No workers were
started against this copy; replay reads the recorded results rather than running
commands or containers again. The supplied database is not a test fixture to
commit.

Cortex was using c2j `v0.0.56-0.20260930003717-614bfac82f15` and JobDB
`v0.0.19-0.20260919034646-71b6668a65db`. Upstream HEAD and latest c2j release were
both `c944978fc9e169b03284efb0d1e936e18497828c`, tagged `v0.0.58`.
The initial upgrade pinned c2j v0.0.58 and its JobDB dependency v0.0.23.

| Request against the same database | Previous dependencies | Upgraded dependencies |
| --- | --- | --- |
| Job metadata | HTTP 200 | HTTP 200 |
| Recorded outcome | HTTP 200, contains extension failure | HTTP 200, same recorded failure |
| Job story | HTTP 500, replay mismatch | HTTP 200, reconstructed story |

The old replay tries to execute `command_execution:command_execution` at ordinal
3. The persisted task at ordinal 3 is `recipe_timeout_checkpoint`. The actual
command is at ordinal 5. The first divergence is therefore a task-sequence
compatibility problem, not a changed recipe prompt or a missing document.
The old runtime reports an input hash mismatch at ordinal 3, then an unexpected
`TaskAttemptOutcome` chapter at ordinal 4. c2j replaces the original determinism
error with `workflow: job run story replay mismatch` at the public service layer.

The new durable timeout checkpoints were added after Cortex's previous pin.
c2j v0.0.58 understands them and replays all three recorded attempts. Workers and
story readers need compatible c2j compiler versions, even when separated by an
HTTP JobDB server.

## The actual recorded job failure

All three attempts failed in the Codex extension at this pinned selector:

`git+https://github.com/colony-2/c2ops.git//codex@ded76dfbd877d3d0749e509844ecdbc57197b572`

The container exited with status 1:

```text
go: creating work dir: stat /var/folders/y5/3tx8472d7wn32fxmfxwzgwgh0000gp/T/: no such file or directory
```

Failed extension task ordinals are 8–10, 19–21, and 30–32. Job attempt outcomes
are recorded at 11, 22, and 33. SQLite stores `completion_status=failed_system`;
the decoded attempt outcomes all report `FAILED` with the same error.

The likely environment cause is a host temporary-directory setting reaching the
container. In c2j v0.0.58, `pkg/ops/process/runtime.go` passes
`BuildProcessEnvMap(req.Env)` into Shai's `PostSetupExec.Env`, and that helper
copies the host's entire `os.Environ()` before applying explicit overrides.
The history does not retain the complete effective container environment, so the
exact originating variable is not proved by the database alone.

Check `TMPDIR` and `GOTMPDIR` in the worker and container. Override them with a
writable container path, such as `/tmp`, or remove inappropriate host values at
the sandbox boundary. Verify with `go env GOTMPDIR` and a temporary-file creation
inside the same sandbox before retrying a new job. Upgrading Cortex does not
repair the already-recorded execution failure.

## Remaining c2j reporting issues

With v0.0.58 the supplied history is readable, but these summaries disagree:

- JobDB records terminal failure; the job listing exposes coarse `COMPLETED`.
- `GetWorkflowOutcome` returns `status: completed` alongside a nonempty error and
  a latest attempt whose outcome is `FAILED`.
- `GetJobRunStory` returns `status: running` and a running root, while the recorded
  extension steps contain the terminal failure.

The outcome service currently changes a failed attempt into a failed status only
when its mapped job status is `running`. Archived failures therefore retain
`completed`. Story status also needs reconciliation with the persisted terminal
outcome rather than relying solely on replay's intermediate tree state.

Fix this in the c2j story/recipejob APIs so all applications share the same status
semantics. Add regressions for an archived failed job with three attempts,
terminal timeout, cancellation, and a successfully completed job. Preserve the
underlying replay error and partial story in diagnostic results rather than
reducing all determinism failures to the generic message. Cortex should not
reimplement recipe execution or reconstruct terminal status independently.

## Upgrade regression: pending input inside a timed recipe

The production browser suite passes job submission, tenant selection, ordinary
single/multiple-question inputs, cancellation, sequential prompts, and document
reviews. The timeout case now fails before the pending form is shown, reproduced
both in the full production suite and in isolation:

```bash
pnpm --filter @colony2/app test:e2e --project=production --grep 'input timeout'
```

The existing fixture is `web/app/tests/fixtures/recipes/browser-input-timeout.yaml`:
a recipe with `timeout: 20s` and one ordinary input question. During its pending
window the job is `READY`, routed to `input:collect_user_input`, with
`task_wait.inputOrdinal=5` and `outputOrdinal=6`. Both pending-input discovery and
input details return HTTP 500:

```text
decode pending input: input storage operation failed: invalid persisted form: unsupported task output envelope version 0
```

The relevant compatibility boundary is:

1. c2j's durable `timeoutJobContext.DoTask` inserts a timeout checkpoint before
   the external input task.
2. JobDB's external-task handoff records `InputOrdinal = ordinal - 1`.
3. `input.Runtime.getOutput` obtains `task.Data()` from that ordinal and expects
   the prepared `ActivityInvocationOutput` form envelope.
4. The intervening checkpoint is not a prepared form envelope.

The required upstream fix is to preserve the actual prepared input and its
artifacts across control checkpoints, retaining the exact waiting-task identity
and timeout budget. Do not recover by scanning for an arbitrary previous form or
by bypassing `SubmitFormResponse` validation. Test ordinary inputs and reviews
inside recipe/op timeouts, worker replacement, response submission, and expiry.

This was an unresolved upstream regression with v0.0.58. The timeout test was
left failing without being weakened or marked as a newly expected failure.
The older expected-failure branch applied only to an input that stayed
pending beyond its deadline; this failure happens earlier during discovery.

Follow-up on 2026-10-04: [independent c2j API tests](bugs/timed-input-v0.0.58.md)
confirm that expiry now works for preloaded recipes, runtime-resolved recipes,
and input-step timeouts. Each fails form retrieval and reports its terminal
outcome as `completed` despite an internal `FAILED` result. The old
expected-failure allowance has now been removed. The follow-up includes a
regression-test patch for c2j and explains the upstream test coverage gap.

## Read-only reproduction commands

Copy the supplied database and its blob directory before starting the server.
From `/src`, with the repository's Go toolchain:

```bash
mkdir -p /tmp/cortex-problemdb-check
cp -a problemdb/. /tmp/cortex-problemdb-check/
bash scripts/go.sh run github.com/colony-2/jobdb/cmd/jobdb \
  --db /tmp/cortex-problemdb-check/jobdb.db --listen 127.0.0.1:19047
```

In another terminal:

```bash
bash scripts/go.sh run ./server/cmd/cortex \
  --jobdb http://127.0.0.1:19047/c2 --addr 127.0.0.1:19049 --working-dir /c2j
```

Inspect metadata, outcome, and story separately:

```bash
job_url='http://127.0.0.1:19049/api/projects/c2/jobs/3KCnRK2LrEmd0H3nwmxcOaqGryg'
curl -sS "$job_url"
curl -sS "$job_url/outcome"
curl -sS "$job_url/story" > /tmp/cortex-replayed-story.json
```

The full story contains recipe source and application inputs. Keep diagnostic
exports local instead of checking them into source control.
