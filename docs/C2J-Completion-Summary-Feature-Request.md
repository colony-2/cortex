# Feature request: expose job completion status and detail in recipe-job APIs

Requested by Cortex, 2026-10-04.
Verified against c2j `v0.0.60-0.20261004014304-252b251f9f74`, using JobDB `v0.0.24`.

Implemented upstream and adopted by Cortex on 2026-10-04 using c2j
`v0.0.61-0.20261004050611-ade631081b4a` and JobDB `v0.0.25`. Cortex now uses these
fields for a combined display status and completion details in its jobs list.
The request below is retained as the original specification.

## Problem

Cortex uses `recipejob.ListRecipeJobs` to populate its jobs list. The returned
`RecipeJob.status` describes scheduling state. In particular, `COMPLETED` can
represent success or failure, so the UI cannot display a useful terminal result
without reading additional history for each job.

JobDB already stores `completion_status` and `completion_detail` in its scheduling
table. Its current public `JobSummary` omits those fields. The upstream request
is documented in
[JobDB-Completion-Summary-Feature-Request.md](JobDB-Completion-Summary-Feature-Request.md).

This request depends on JobDB exposing those persisted fields. c2j should adopt
that API and pass the information through, keeping the jobs list independent of
recipe resolution, history retrieval, and story replay.

## Requested response shape

Add optional fields to `recipejob.RecipeJob`:

```go
CompletionStatus string `json:"completion_status,omitempty"`
CompletionDetail string `json:"completion_detail,omitempty"`
```

If JobDB introduces a named completion-status type, reuse it where appropriate.
Suggested JSON for a failed job, with an abbreviated example detail:

```json
{
  "job_id": "example-job",
  "status": "COMPLETED",
  "store": "ARCHIVED",
  "completion_status": "failed_system",
  "completion_detail": "container exited with status 1"
}
```

Preserve the JobDB categories (`success`, `failed_app`, `failed_system`,
`failed_timeout`, `cancelled`) and any unknown future values. Keep `status` as
the existing scheduler status.

## Required coverage

1. Copy the fields in `recipejob.RecipeJobFromSummary` so ordinary listings,
   single-job reads, and child-job listings return the same completion data.
2. Preserve the fields through `ListRecipeJobs`, `GetRecipeJob`,
   `ListChildRecipeJobs`, and `ListChildRecipeJobsFromWorkflow`, including any
   intermediate `workflowctl.JobItem` conversion that could otherwise drop them.
3. Expose the same optional fields in the standalone `pkg/joblist` job model and
   JSON CLI listings, including child listings where applicable.
4. Update public API documentation and examples to distinguish scheduler status,
   persisted completion status, and execution-requirement status.

No Cortex-specific endpoint is needed: these fields belong in the existing
shared job projections.

## Semantics and compatibility

- Completion fields describe final job completion. A failed task or job attempt
  does not imply terminal failure while the job is still retrying or waiting.
- Absent completion status means unavailable, not success. Older JobDB servers
  and records without completion information must remain usable.
- Empty completion detail is valid. Preserve any supplied detail verbatim.
- `execution.status` continues to describe the scheduling-requirement view
  (`specified`, `unresolved`, `in_flight`, etc.); do not repurpose it as an outcome.
- Do not remap persisted `failed_timeout` or other categories to story-specific
  names. Consumers can format labels while retaining the original category.
- Cancellation requested alone must not be upgraded to final cancellation.
- Existing filters, pagination, and active/archive selection remain unchanged.

Do not fall back to `GetJobRun`, `GetJobRunStory`, `GetWorkflowOutcome`, recipe
loading, or direct database queries when a completion field is missing. The
list/read projection must use the information supplied by JobDB's job summary.

## Acceptance criteria

- Fixtures for each known completion category retain status and detail in
  ordinary, single-job, and child-job recipe responses.
- Standalone job listings and JSON CLI output expose equivalent data.
- Tests cover missing completion fields, an empty detail, an unknown category,
  and a retrying job whose earlier attempt failed.
- Tests exercise embedded and remote JobDB paths, including field serialization.
- A list-only fake runtime proves these projections need no history, chapter,
  artifact, or recipe reads.
- The existing problem job can be listed as scheduler status `COMPLETED`,
  completion status `failed_system`, with its persisted container error detail,
  without opening its story.

## Cortex follow-up

Once the upstream APIs expose these fields, Cortex can extend its shared
TypeScript model and display completion status and detail directly from the
current list response. Clearing the status filter to request all statuses is a
separate Cortex change; neither upstream request requires changing default
status-selection behavior.
