# c2j: include running task leaves and input in job stories

## Verified fix: 2026-10-27

Cortex now pins c2j `v0.0.107-0.20261027200902-6ecd59dcfa7f` with JobDB
`v0.0.59-0.20261027194840-510d817719ac`. The start-event projection gap is
fixed. The same public-recorder probe now returns:

```json
{"status":"running","input":{"prompt":"visible before completion"},"output":null,"task_ordinal":1}
```

The complete c2j story suite passes, including new integration tests for local
and externally dispatched running tasks, retries, multi-step operations, and
unfinished source resolution against SQLite and remote JobDB. The task tests
verify input before completion, absent output/artifacts, separation of previous
attempt output, stable identity through completion, and unchanged stored history.
No Cortex-specific reconstruction is required.

The original request and earlier findings below are retained for context.

## Earlier check: c2j v0.0.106, 2026-10-27

Cortex previously checked c2j `v0.0.106` (main commit `cd329e8381df`) and JobDB
`v0.0.59-0.20261027194840-510d817719ac`. JobDB's unfinished-task inspection
tests and c2j's existing story suite pass, but c2j's start-event projection
gap remains in this revision.

A direct probe of the public recorder initialized an operation, supplied an
unfinished `TaskStartEvent` with `State: TaskStateRunning`, ordinal 1, and
`Input: {"input":{"prompt":"visible before completion"}}`, then inspected
`Snapshot().Root.Children[0]` without sending an end event. It returned:

```json
{"status":"running","input":null,"output":null,"task_ordinal":null}
```

This confirms that the running leaf can exist while its input and ordinal remain
unavailable. `OnTaskStart` still populates only the separate attempt object;
copying its details onto the visible leaf remains deferred to `OnTaskEnd`.
This is a recorder-level reproduction, not yet a full running-recipe integration
test. The requested integration coverage below is still needed.

## Requested capability

`GetJobRunStory` should include the current task leaf before it finishes, with its
operation name, invocation/attempt identity, accurate state, and resolved input.
Its output and output artifacts should remain absent until available. Completion
should update the same logical leaf rather than introduce a previously invisible
task.

This lets Cortex users inspect what a running operation was asked to do. Cortex
already forwards the story response and retains running operation leaves in its
tree projection; it should not reconstruct missing tasks from private JobDB data.

## Source findings

Inspected c2j `v0.0.105-0.20261027024848-fbcd5cd24a8a` with JobDB `v0.0.57`.
These findings explain a plausible path to the reported missing leaf; the
specific production job has not been reproduced locally.

In `pkg/story/live/recorder.go`:

- `ensureReplayMissStepNode` requires a task type. Current JobDB unfinished-task
  misses can omit it, causing the fallback to return without adding a leaf.
- `OnTaskStart` creates a running leaf but applies input and other attempt details
  to a separate attempt node. `copyAttemptIntoStep` runs in `OnTaskEnd`, delaying
  those details on the visible leaf until completion.

Existing replay tests in `pkg/story/internal/story/replay_story_test.go` exercise
cache misses using a fake engine with a populated task type. They do not establish
that an actual unfinished JobDB task exposes the same information.

## Requested changes

- Consume JobDB's supported unfinished-invocation information and expose the
  active task in the public story response. Coordinate with
  [the JobDB request](JobDB-In-Progress-Task-Feature-Request.md); avoid compensating
  with private-history decoding or guessed task identities.
- Populate the visible leaf's input and available attempt details when the task
  starts, without waiting for `OnTaskEnd`. Preserve meaningful operation names
  and invocation paths so Cortex can use its existing labels.
- Keep state faithful to JobDB, including waiting versus actually executing.
  Do not mark a task complete or give it output merely because replay stopped at
  an unfinished boundary.
- Keep completed prior attempts separate from the current attempt. Do not copy
  an earlier attempt's output or artifacts onto the running attempt.
- Retain logical identity across repeated reads and completion so a client can
  keep the task selected as it refreshes.

## Verification

The primary regression test belongs in c2j's story integration suite, backed by
real JobDB. Hold a task behind an explicit barrier, call `GetJobRunStory` while it
is blocked, and assert the named leaf and resolved input are present with the
correct nonterminal state and no output/output artifacts. Release the task and
verify that the same logical leaf completes with its output.

Cover local and externally dispatched tasks, plus a running retry whose previous
attempt has output. Add a recorder-level test asserting that `OnTaskStart` alone
makes input visible. Inspection must not execute task work or mutate history.
After upstream integration, Cortex can verify the resulting API contract in a
browser test; a mocked browser test alone would miss this replay-layer gap.
