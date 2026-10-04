# Timed input failures in c2j v0.0.58

Fixed upstream on 2026-10-04 and incorporated into Cortex as
`v0.0.59-0.20261004003418-6af179613628`:

- `007d85a`: reconcile terminal workflow failures while keeping active retries pending.
- `6af1796`: resolve prepared input through its recorded admission reference,
  preserving existing checkpoint histories, artifacts, and completion identity.

The report and reproduction below describe the original v0.0.58 failures. The
patch is retained for reproducing that version only; current c2j already includes
`TestTimedInputPublicAPIs` and additional expiry, fallback, review, and recovery
coverage in `pkg/input/test-fixtures/timed_input_test.go`.

Validation with the new pin: all 14 browser tests pass, including all nine
production tests and the previously failing unanswered-input timeout. The
timeout test verifies the failed outcome, prompt removal in both open tabs,
and rejection of a late response. Go tests, both web typechecks, and all 102
web unit tests also pass.

Investigated 2026-10-04 against c2j `v0.0.58`
(`c944978fc9e169b03284efb0d1e936e18497828c`) and JobDB `v0.0.23`.
Cortex's production suite had eight passing tests and one failing timed-input
test. There were two independent backend defects along that test's path.

## Independent reproduction

[c2j-timed-input-regression.patch](c2j-timed-input-regression.patch) adds
`TestTimedInputPublicAPIs` to c2j's existing input fixture package. It is a
regression test, **not a fix**. It uses real SQLite storage behind HTTP JobDB,
the recipe compiler, the public input runtime, and the public story service.
It requires neither Cortex nor a browser nor the supplied production database.

Apply it in a disposable c2j v0.0.58 checkout, using Go 1.26.7:

```bash
git apply /src/docs/bugs/c2j-timed-input-regression.patch
go test ./pkg/input/test-fixtures -run '^TestTimedInputPublicAPIs$' -count=1 -v
```

The test was run in `/tmp/c2j-timed-input-investigation`; `/c2j` was not modified.
Results take about ten seconds:

| Case | Form retrieval / discovery | Deadline completion | Late answer after completion | Public outcome |
| --- | --- | --- | --- | --- |
| Untimed input | Pass | Timely answer completes successfully | — | — |
| Preloaded recipe, `timeout: 3s` | Envelope-version error | `FAILED`, timeout error | Rejected | Incorrectly `completed` |
| Runtime-resolved recipe, `timeout: 3s` | Envelope-version error | `FAILED`, timeout error | Rejected | Incorrectly `completed` |
| Input step, `timeout: 3s` | Envelope-version error | `FAILED`, timeout error | Rejected | Incorrectly `completed` |

All three timed cases disappear from pending-input discovery after expiry. A
fresh worker invocation resumes each wait after its original deadline. These
tests establish rejection **after terminal completion**; they do not establish
the result of a human-response race after the deadline but before a worker
records expiry. That race needs separate coverage.

The story API correctly reports `failed` for these simple timeout jobs. The
supplied three-attempt extension failure still exhibits the separate `running`
story summary described in [the original investigation](../Job-Story-Replay-Investigation.md).

## Defect 1: the waiting-task input points at a checkpoint

`compiler/timeout_scope.go`, `timeoutJobContext.DoTask`, records a
`recipe_timeout_checkpoint` before admitting a task. In the runtime-resolved
case the final sequence is:

```text
ordinal 4: input:generate_form          -> prepared form envelope + artifacts
ordinal 5: recipe_timeout_checkpoint   -> {"at": "..."}
ordinal 6: input:collect_user_input    -> external wait
```

JobDB's `workflow/worker_runner.go` records `TaskWait.InputOrdinal = ordinal - 1`,
so this wait points to 5. `workflow/runtime_task_handle.go`, `TaskHandle.Data`,
reads that chapter. c2j's `input/runtime.go`, `Runtime.getOutput`, expects the
prepared `ActivityInvocationOutput` envelope and instead receives a timestamp:

```text
decode pending input: input storage operation failed: invalid persisted form: unsupported task output envelope version 0
```

Both public `GetForm` and `ListPendingInputsPage` fail. Cortex's pending-list
endpoint also fails when it reads the form to classify it as an input or review.
Because discovery propagates this error, one malformed timed input can prevent
the tenant's entire pending page from loading.

Required upstream behavior:

- Preserve an explicit association between a waiting external task and its
  prepared application output, including artifacts and workspace context.
  The immediately preceding control chapter is not necessarily that output.
- Keep the admission timestamp used for deadline calculation separate from
  the prepared-form reference. Simply changing `InputOrdinal` can alter
  deadline behavior and completion guards; audit both before choosing the API.
- Preserve output ordinal, input hash, request identity, atomic completion,
  alternate routing, and immutable absolute deadlines across worker recovery.
- Keep existing recorded histories readable. Reordering/removing checkpoints
  or changing their hashed input would risk another replay mismatch.
- Do not fix Cortex by scanning backwards for an arbitrary form or loosening
  envelope validation. Cover ordinary forms, structured inputs, reviews with
  document artifacts, response submission, expiry, and worker replacement.

## Defect 2: archived failures are mapped to success

In c2j `pkg/story/internal/service/service.go`, `GetWorkflowOutcome` initially
maps JobDB's coarse `COMPLETED` state to `completed`. It only overrides that
status for a failed latest attempt when the initial status was `running`.
An archived failure therefore returns `completed` alongside its timeout error.

Fix status reconciliation in c2j, using the authoritative terminal attempt and
job state. Preserve cancellation/termination and distinguish active retries
from terminal failure. Verify successful, failed, cancelled, retried, and
timed-out jobs. Cortex should consume the same semantics as other c2j clients.

Fixing only form retrieval will expose this second failure in the browser
test's `status: failed` assertion.

## Why existing upstream tests missed this

`compiler/timeout_recovery_test.go` calls `GetWaitingTask` followed directly by
`TaskHandle.Finish` with a constructed output. This verifies durable timing but
never reads the prepared form through the input API. The input lifecycle tests
exercise public forms without an enclosing timeout. The added regression
combines those two paths and retains an untimed success control.

## Cortex test changes and original failure results

`recipe-input.spec.ts` now checks pending discovery separately from the browser
and preserves the first HTTP 500 in `pending-input-error.json`. Failed tests
also attach pending discovery, input details, job metadata, the recorded
outcome, and worker logs. The original no-refresh browser checks still run
after successful discovery.

The obsolete expected-failure branch for JobDB v0.0.19's non-expiring waits has
been removed. No failure has been skipped or converted to an expected pass.
Before the upstream fixes, the production suite reported **8 passed, 1 failed**;
the improved diagnostics showed the actual HTTP 500 rather than a missing-element
timeout. App typecheck passed. The c2j regression patch had one passing control
and three failing timeout cases, with the failures confined to form
reading/discovery and outcome status. Both upstream fixes were needed for the
browser test to pass.
