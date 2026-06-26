# Cortex c2j API Prerequisite Review

This document records the status of the c2j public API changes requested for
the Cortex rehab. It supersedes the earlier export request.

Reviewed source:

- `GUIDE-Cortex-RecipeJob-API.md`
- `/c2j/pkg/recipejob`
- `/c2j/pkg/starter`
- `/c2j/cmd/c2j/internal/submitjob`
- `/c2j/cmd/c2j/internal/listjobs`

## Status

All c2j API prerequisites for the Cortex rehab are satisfied.

Cortex can now submit, list, and read c2j recipe jobs with repo-url cell
identity without importing `cmd/c2j/internal/*`, without opening JobDB through
c2j internals, and without running c2j workers.

## Implemented Public Surface

Use:

```go
import "github.com/colony-2/c2j/pkg/recipejob"
```

`pkg/recipejob` now exposes:

- `ResolveTarget`
- `BuildStartJob`
- `SubmitRecipeJob`
- `ListRecipeJobs`
- `GetRecipeJob`
- `RecipeJobFromSummary`
- `DefaultVisibleStatuses`
- `StoresForStatuses`
- `NormalizeRepositorySource`
- `RepositorySourcesEqual`

`pkg/starter` now persists target repo metadata:

- metadata JSON field: `repo`
- Go field: `starter.JobMetadata.RepositorySource`
- field constant: `starter.MetaFieldRepo`

`starter.StartRecipeJob` also forwards `workflowctl.StartJob.JobID` when no
explicit `StartRecipeJobOptions.JobID` is supplied.

## Requirement Closure

| Previous Cortex Requirement | Implemented By | Status |
| --- | --- | --- |
| Repo-url cell identity in recipe job lists | `starter.MetaFieldRepo`, `starter.JobMetadata.RepositorySource`, `recipejob.ListRecipeJobs`, `recipejob.GetRecipeJob`, `recipejob.RecipeJobFromSummary` | Satisfied |
| Filter recipe jobs by repo URL or configured short name | `recipejob.ResolveTarget` plus `recipejob.ListRecipeJobsRequest.RepositorySource` | Satisfied |
| Public target resolution matching c2j submit/list semantics | `recipejob.ResolveTarget` | Satisfied |
| Public c2j-equivalent start request assembly | `recipejob.BuildStartJob` and `recipejob.SubmitRecipeJob` | Satisfied |
| Reusable c2j list defaults | `recipejob.DefaultVisibleStatuses` and `recipejob.StoresForStatuses` | Satisfied |
| Avoid importing `cmd/c2j/internal/*` | public `pkg/recipejob` package; c2j CLI now reuses it internally | Satisfied |
| Avoid c2j worker/runtime ownership in Cortex | `pkg/recipejob` does not open runtimes, start workers, execute recipes, or register ops | Satisfied |

## Cortex Integration Notes

### Target Resolution

Use `recipejob.ResolveTarget` wherever Cortex accepts a cell value. It supports
the Cortex target model:

- empty cell or `Self: true` means current cell
- `root`
- configured short names
- canonical repo strings
- git URLs
- local paths resolved relative to `WorkingDir`

The returned `ResolvedTarget.RepositorySource` is the durable repo-url identity
for Cortex cell/job filtering.

### Job Submission

Use `recipejob.BuildStartJob` to assemble `workflowctl.StartJob`, then submit
with `starter.StartRecipeJob` or `starter.StartRecipeJobWithOptions`.

`BuildStartJob` fills the c2j submit fields Cortex needs:

- `Workflow.CellName`
- `Workflow.ProjectId`
- `GitBase.BaseRepo`
- `GitBase.BaseRef`
- `RecipeSource.Repo`
- `RecipeSource.Ref`
- `GitRef`
- `SubmittedAt`
- `InputHash`

Use `recipejob.SubmitRecipeJob` only when the Cortex call site wants the helper
to both build the start request and submit through a caller-owned JobDB
submitter.

### Job Listing And Reading

Use `recipejob.ListRecipeJobs` for job lists and
`recipejob.GetRecipeJob` for one-job reads.

For short-name filtering, resolve the short name first:

```go
target, err := recipejob.ResolveTarget(ctx, recipejob.ResolveTargetRequest{
    WorkingDir: repoWorkingDir,
    TenantID:   tenantID,
    Cell:       shortName,
})
```

Then list with:

```go
resp, err := recipejob.ListRecipeJobs(ctx, engine, recipejob.ListRecipeJobsRequest{
    TenantID:         tenantID,
    RepositorySource: target.RepositorySource,
})
```

Use `recipejob.DefaultVisibleStatuses()` and
`recipejob.StoresForStatuses(statuses)` to match c2j CLI visible-list defaults.

### Stories, Outcome, Restart, Artifacts, And Inputs

No new c2j prerequisite remains for these surfaces:

- keep using `pkg/story` and `pkg/story/live` for story/outcome/restart/artifact
  behavior
- keep using `pkg/input` for user-input runtime and management behavior
- keep using JobDB remote runtime/engine wiring directly from JobDB packages

## Compatibility Note

Repo metadata is persisted for jobs submitted after this c2j change. Jobs
submitted before `starter.MetaFieldRepo` existed cannot be filtered by repo URL
at the JobDB metadata layer. `recipejob.RecipeJobFromSummary` can still populate
repo fields from a listed job payload when that payload is available.

This is not a blocker for the Cortex rehab because new Cortex/c2j submissions
will include repo metadata. If Cortex later needs repo-url filtering for legacy
jobs, that should be handled as a separate migration/backfill decision.
