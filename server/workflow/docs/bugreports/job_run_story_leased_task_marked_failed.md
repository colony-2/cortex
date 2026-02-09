# Bug: `GetJobRunStory` marks actively-running (LEASED) runtime task/state as failed

## Summary
When a workflow job is actively running and the next task is represented as a *runtime* task attempt in a non-terminal state (e.g. `LEASED`), `GetJobRunStory`/UI shows the task/state as **failed** even though it’s simply **in progress**.

## Expected
For jobs with a running status (`ACTIVE`, etc.), task attempts in `READY | LEASED | WAITING | RUNNING` (with no terminal outcome) should be represented as **running/pending**, and container nodes (state/stateMachine/recipe) should remain **running**.

## Observed
The job is running, unarchived, and SWF reports a runtime attempt in `LEASED`, but the story/UI shows failure at the task/state level.

### Evidence (SWF job run dump excerpt)
From `INFO GetJobRunStory: debug swf job run dump` (job IDs redacted; issue is data-shape based):

- `job.status = ACTIVE`
- `job.archived_at = <nil>`
- `job_attempts = []` (no job result written yet)
- `start.ordinal = 0`, start input present

The `tasks` list contains a single task run that looks like the *runtime* task representing the current in-flight work:

```text
tasks:[
  map[
    task_run_id: recipe:1
    task_type: recipe
    attempts:[
      map[
        attempt: 1
        state: LEASED
        worker_id: ""                     # empty
        created_at: 0001-01-01T00:00:00Z  # zero time
        ordinal: 1
        outcome:{ Status:"" PayloadKind:"" Error:<nil> }  # no terminal outcome
        output: map[]                     # empty
        input: map[
          data: {"context":{...},"git":"main","recipe":"new-ticket","tenantId":"..."}
          data_len_bytes: 548
          artifact_count: 1
          artifacts_brief: [new-ticket.recipe.yaml ...]
        ]
      ]
    ]
  ]
]
```

Key points:
- The job is **ACTIVE** and **not archived**.
- The task attempt `state` is **LEASED** (non-terminal).
- `outcome.Status` is empty (not failed/succeeded).
- Yet the story/UI shows this node as **failed**.

## Impact
- UI displays false failures while work is still executing.
- Users may assume the workflow is broken and attempt unnecessary restarts/retries.

## Reproduction
1. Start a new recipe job (e.g. `new-ticket`).
2. Immediately call `GET /api/projects/{projectId}/jobs/{jobId}/story`.
3. While the job is still `ACTIVE` and unarchived, observe that `GetJobRun` includes a task attempt in `LEASED`.
4. UI/story shows task/state as **failed** instead of **running/pending**.

## Notes / Suspected cause
- SWF `GetJobRun` can append a “runtime” task run for unarchived jobs (i.e., a task that exists in runtime state even before a persisted chapter/attempt is recorded).
- The story/status mapping path appears to treat missing/empty `outcome.Status` and/or zero `created_at` as an error condition, rather than honoring `attempt.state=LEASED` as “in progress”.

## Fix
Treat non-terminal runtime task runs as `in progress` (not replay mismatches) when the job is still running:
- If the next unconsumed task run (by ordinal) is a different task type but is in a non-terminal attempt state (`READY|LEASED|WAITING|RUNNING`), story replay returns `ErrReplayInProgress` instead of `ErrReplayMismatch`.
- If an op has no recorded task runs yet and the job is running, story replay returns `ErrReplayInProgress` (op not started) instead of failing the op.
