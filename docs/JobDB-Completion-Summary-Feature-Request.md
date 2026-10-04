# Feature request: expose persisted completion status and detail in job summaries

Requested by Cortex, 2026-10-04.
Verified against JobDB `v0.0.24`.

Implemented upstream in JobDB `v0.0.25` and adopted by Cortex on 2026-10-04.
The request below is retained as the original specification.

## Problem

Cortex's jobs list receives scheduler statuses such as `COMPLETED`, but cannot
distinguish successful execution from application failure, system failure, or
timeout using the public job summary. Reading execution history per listed job
would add unnecessary work and introduce history/replay dependencies into the
jobs list.

JobDB already persists the necessary information in the scheduling record. This
request is to expose that information through the supported API.

The corresponding c2j request is
[C2J-Completion-Summary-Feature-Request.md](C2J-Completion-Summary-Feature-Request.md).

## Evidence in the current implementation

In `pkg/jobdb/runtime/sqlite/`:

- `schema.go` declares `jobdb_jobs.completion_status` and `completion_detail`.
- `lease.go`, `CompleteJobWithLeaseByID`, writes both fields when finalizing a job.
- `scheduler.go`, `jobColumns` and `scanJobRow`, already select and decode both
  fields as part of loading the scheduling row.
- `runtime.go`, `ListJobs`, uses those columns, but does not copy the completion
  fields into its returned `jobdb.JobSummary`.
- `scheduler.go`, `statusFromRow`, reports an archived, noncancelled job as
  `COMPLETED`, regardless of whether its completion was successful or failed.

`pkg/workflow/worker_runner.go`, `completionStatusAndDetail`, produces these
completion categories:

| Stored value | Meaning |
| --- | --- |
| `success` | Execution succeeded |
| `failed_app` | Application error |
| `failed_system` | System error |
| `failed_timeout` | Execution timeout |
| `cancelled` | Execution cancelled |

The supplied Cortex problem database confirms this in existing data: job
`3KCnRK2LrEmd0H3nwmxcOaqGryg` has `completion_status = failed_system` and a
`completion_detail` explaining the container's missing Go temporary directory.
Its public scheduler status is `COMPLETED`.

## Requested API

Add optional completion information to `jobdb.JobSummary`. Suggested Go fields:

```go
CompletionStatus string
CompletionDetail string
```

A named string type with constants for the known categories is welcome, provided
unknown stored values remain representable. Match JobDB's existing naming and
serialization conventions; publish the fields in the remote API schema as well.

Required behavior:

1. Return the persisted completion category and detail from `ListJobs`, including
   archived records and listings narrowed to a single job.
2. Preserve the existing scheduler `Status` field and its filtering semantics.
   Completion is a separate dimension; `COMPLETED` must not be silently renamed
   to `FAILED` or `SUCCESS`.
3. Preserve the stored detail without synthesizing a different error from history.
4. Treat absent completion information as unknown/unavailable. An empty detail
   is valid, including for successful jobs. Do not infer success from `COMPLETED`.
5. Expose final job completion, not the most recent failed attempt of a job that
   is still running, waiting, or retrying. Cancellation requested alone must not
   be presented as confirmed terminal completion.
6. Preserve unfamiliar stored completion values for forward compatibility.

Implement this consistently across supported runtimes, scheduler adapters, and
remote client/server transport. Update generated API types, OpenAPI definitions,
and public API snapshots as appropriate. Any other public job summary projections
should retain these fields rather than dropping them.

## Cost and compatibility

For SQLite, both columns already exist and are selected by `ListJobs`. Passing
them through should require no schema migration, additional SQL query, chapter
read, artifact read, or workflow replay. Other backends should use their existing
scheduling completion records with the same constraint against history reads.

Make the change additive. Older stored rows and older remote servers that omit
the fields must remain readable. No database backfill or Cortex-specific direct
SQL access should be required.

Completion filtering is not required for this request. Existing pagination,
tenant scoping, scheduler-status filters, and active/archive selection should
continue to work unchanged.

## Acceptance criteria

- Successful, application-failed, system-failed, timed-out, and cancelled jobs
  expose their persisted completion category and detail through `ListJobs`.
- A failed attempt followed by a scheduled retry exposes no terminal completion;
  a later successful finalization exposes `success`.
- Missing fields, empty details, and unknown completion values round-trip safely.
- Remote responses and embedded runtime responses agree.
- Existing archived SQLite rows return their stored values without migration.
- A regression test proves listing completion information does not read chapter
  history or artifacts and does not invoke replay.
- Existing list filtering and pagination remain compatible.

## Consumer

c2j will pass these fields through its recipe-job APIs. Cortex can then display
the completion category beside scheduler status and show the detail on demand,
using the existing list response.
