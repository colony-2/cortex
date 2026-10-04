# Historical story steps disappear after the job deadline

Resolved upstream and adopted on 2026-10-04: Cortex now uses c2j
`v0.0.60-0.20261004014304-252b251f9f74` and JobDB `v0.0.24` (`b7e1d5a`).
The report and candidate patch below are retained as the investigation of
v0.0.23; no local dependency replacement or replay patch is required now.

Verification with these pins and the normal production Cortex binary returned
all 96 story nodes from a fresh copy of the supplied database, with three
failed job attempts, nine recorded task failures, and final status `failed`.
The production UI displayed all three job attempts and 97 tree items (96 nodes
plus the previous-attempts group). Each of the nine failures was opened and its
task ordinal checked; there were no browser errors. The original database was
preserved and no job workers ran against its copy.
Go tests, both web typechecks, all 106 web unit tests, and all 14 browser tests
pass, including the unanswered-input timeout and story rendering regressions.

Reproduced on 2026-10-04 with Cortex's c2j pin
`v0.0.59-0.20261004003418-6af179613628` and JobDB `v0.0.23`.
This is separate from the fixed timed-input form/outcome bugs.

## What the supplied database shows

The original database was preserved. A copy under
`/tmp/cortex-story-ui-check` was served without running any workers.
Tenant `c2`, job `3KCnRK2LrEmd0H3nwmxcOaqGryg`, contains three job attempts.
Each has 32 story nodes and three failed extension task attempts:

| Job attempt | Failed task ordinals |
| --- | --- |
| 1 | 8, 9, 10 |
| 2 | 19, 20, 21 |
| 3 | 30, 31, 32 |

The recorded error is the same missing container temporary directory described
in [the original investigation](../Job-Story-Replay-Investigation.md).
No task history has been deleted.

Unmodified JobDB v0.0.23 replay gave HTTP 200 with `status: failed`, `root: null`,
and an empty recipe identity. Earlier replay, before the original job deadline
passed, returned the task trees. Thus the contents of this read-only response
change with the wall clock even though the persisted history is unchanged.

## JobDB defects

All three are in `pkg/workflow/worker_runner.go`:

1. `DoJob` checks the historical job's total deadline against the current time.
   Once it has expired, its recovery scan consumes task chapters until it finds
   `JobAttemptOutcome`. It then returns the recorded result without replaying
   the recipe or emitting the task observer events that c2j uses for its story.
2. Simply skipping that scan is insufficient. The job execution timer can
   interrupt historical orchestration, and `awaitUntil` checks expired
   deadlines before replaying already-elapsed retry backoffs. This can also
   produce a spurious determinism mismatch at an unconsumed task chapter.
3. `checkCachedJobResult` returns the terminal recorded application failure in
   both the outcome-error and cache-read-error slots. Callers return early on
   the latter and never emit the final `JobEnd` event. This explains the earlier
   story root incorrectly remaining `running` despite failed task leaves.

Replay must consume recorded outcomes and preserve their timestamps, errors,
and observer events. It must not reapply a historical execution deadline using
today's clock, execute task workers, or write chapters. Caller cancellation
and replay cache-miss behavior must still work.

## Original candidate fix and regression

[jobdb-story-replay-after-deadline.patch](jobdb-story-replay-after-deadline.patch)
contains the original proposed fix and regression test. For reproducing the
investigation only, apply to JobDB v0.0.23 (not the fixed v0.0.24):

```bash
git apply /src/docs/bugs/jobdb-story-replay-after-deadline.patch
go test ./pkg/workflow -count=1
```

The test records successful and failed/retried jobs, shifts fixture timestamps
into the past, then verifies task/job start and end events, original outcomes,
and zero task re-execution or chapter writes. Unmodified v0.0.23 emits zero
task events. With the patch, the entire workflow package passes, including
the new success and nine-failure cases.

Initial testing used an isolated module copy under `/tmp/cortex-story-jobdb`
and a temporary Go workspace without changing Cortex's dependency pin.
The candidate also restores all 96 nodes and nine failures from the supplied
database, and the final story status becomes `failed`. The upstream fix has
since been adopted through the normal dependency pins, without a Cortex replay
workaround.

## Cortex rendering fixes

There were independent UI gaps even when the API returned the full story:

- The tree initially expanded no nodes, so only its root was visible.
- It ignored `past_attempts` and `job_attempt`; previous job runs were absent.
- It never displayed the node's `error.message` or error code.
- Node IDs repeat across job attempts, requiring attempt-scoped selection keys.

The UI now expands the story on initial load, offers expand/collapse controls,
includes previous job attempts, and displays each selected task's recorded
failure. Links include the job-attempt identity; legacy node-ID links still
resolve to the latest attempt. An empty terminal story is identified as missing
API data instead of saying that the story is merely not ready yet.

Four UI regression tests cover all attempts, failure selection, refresh,
deep links, collapsing/expanding, and `prior_attempts` task retries. A browser
check using the database copy and isolated JobDB fix showed 97 tree items
(96 story nodes plus the previous-attempts group), opened all nine task failures,
and reported no browser errors. This initial check used the candidate patch;
the subsequent dependency upgrade incorporates the upstream fix.
The app typecheck and all 14 existing browser tests pass with the UI changes
and the normal, unmodified dependency pin.
