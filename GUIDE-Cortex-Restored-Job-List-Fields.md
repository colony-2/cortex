# Restore stored job-list fields in Cortex

> Current goal contract: use primary-only navigation and creation-time primary
> selection. Secondary IDs are deliberately empty in current detail/submission
> responses and excluded from raw display; inspection must not restore them.
> Historical decoding remains supported. A compact wait summary does not explain
> dependencies: selected-job inspection retains the complete wait, conditions,
> deadlines, reasons and exact result producers. An empty secondary-ID field says
> nothing about those waits. Goal reads, `FailGoal(message)` and ordinary input-file
> attachments remain supported; protected-record integration remains separate.

c2j again preserves completion detail, the full client payload and revision,
task coordinates, and prerequisite IDs in ordinary job lists. Restore their
display from the existing list response. A page still makes one JobDB
`ListJobs` call; rendering these fields must not introduce per-row detail reads.

## Dependencies and response mapping

Update Cortex to a c2j revision containing this restoration. c2j pins JobDB
`v0.0.64-0.20261029043940-9c44e3e3bfa1`. Remote JobDB/pgjobdb servers must also
implement the restored contract; updating Cortex alone does not update them.
See [the backend migration](MIGRATION-C2J-Restored-Job-List-Fields.md).

Audit backend response adapters, frontend response types, client caches, and row
components for fields discarded during the compact-list migration. The names
below are **c2j JSON names**, not the camelCase names of raw JobDB responses.
Use the same mapping for ordinary, archived, child-job, and keyed lists.

| c2j field | Cortex behavior |
| --- | --- |
| `completion_status` | Preserve unfamiliar strings. Keep terminal outcome separate from scheduler `status`; a retrying job need not have a terminal result. |
| `completion_detail` | Restore completion/error text in the row's existing message or expanded-message area. Preserve Unicode, whitespace, and line breaks; render as text, not HTML. Empty detail is valid. Visual truncation is fine if the full text remains expandable/copyable from the same response. |
| `execution` | Show a compact requirements link and readable Env details using c2j's interpreted view. |
| `client_payload`, `client_payload_revision` | Retain the complete opaque value and its paired revision in the client model. Do not replace it with only `c2j.execution`, prune other namespaces, or fetch it again per row. |
| `task_wait` | Retain `inputOrdinal`, `outputOrdinal`, `inputHash`, and `resumeJobType` for existing task UI. These identify an occurrence; they are not input/output task data. |
| `wait_for` | Restore prerequisite IDs in their supplied order. Link using the current job's tenant/project. Do not fetch target jobs just to populate row labels. |
| `blocking_wait_summary` | Retain `waitId` and optional `deadline`. Use them for the existing wait display and explicit selected-wait inspection. |
| `next_route` | Continue using the stored route for routing display. Do not infer a current task from archived coordinates alone. |

Workflow summaries also preserve `ExecutionState.RunPolicy` and `TaskWait`.
c2j's recipe/CLI list DTOs expose coordinates as `task_wait`; do not assume a new
`execution_state` field exists on those DTOs.

## Render execution requirements

The goal list shows one muted line such as `req: macOS-26-arm 4c/8g [docker,
network]`. It opens the exact attempt's **Env** tab and preserves list filters.
`c` means CPU cores (`500m` becomes `0.5c`), and `g` means GiB. Other memory units
remain explicit: `512Mi` becomes `512MiB`, while `8G` becomes `8GB`. Additional
requirement groups add a count, or a descriptive summary when present alone.
Empty results render no summary, placeholder, link or spacing. Long lines
ellipsize; Env keeps complete selectable values.

Cortex preserves the upstream `execution` field unchanged. Its additive
`recorded_execution` field appears only when current demand is `in_flight` or
`not_waiting`. Cortex computes it with c2j's `execution.Inspect` using metadata
and client payload already returned; it adds no JobDB reads. List, goal-filtered
list, children and exact detail share this projection. Deploy server and web
together; older responses without the additive field remain readable.

| Selected interpretation | List | Env |
| --- | --- | --- |
| `specified` | Known nonempty values on one line. | Full labeled values. |
| `unspecified` | Nothing. | No requirements specified. |
| `unresolved` | Validated known values, if any. | Explain partial resolution. |
| `malformed`, `unsupported` | Nothing. | Explain invalid/unsupported data and retain diagnostics. |
| Missing or unrecognized | Nothing. | Explain unavailable data. |

Use `execution` for waiting jobs and `recorded_execution` for running/finished
jobs. Do not reconstruct requirements from raw payloads in the browser. c2j
chooses the source: a valid published snapshot replaces submission values
completely; invalid published data never falls back to submission. Missing/null/
unrelated payloads can restore submission values regardless of payload revision.
Refresh replaces responses rather than merging old requirements or diagnostics.

Env includes base, CPU, memory, capabilities, ephemeral storage, packages, Python
and legacy image/platform. Source, resolution, diagnostics and exact data live in
**Details**. Stored requirements do not describe live scheduling demand or actual
allocation. The panel reuses exact-job metadata, its polling and retry behavior;
it does not fetch Trace, history or artifacts for requirements.

## Preserve the list boundary

- Keep one backend page per Cortex page and pass its continuation unchanged.
  Do not scan extra pages to fill a locally filtered page. c2j's compatibility
  list filters remain rejected; this restoration does not re-enable them.
- Do not call `GetJob`, `GetJobRun`, artifact/task APIs, goal-membership APIs,
  or dependency-explanation APIs for every row on refresh or expansion merely
  to show these restored fields.
- `other_goal_ids` remains omitted from current display and empty in detail responses.
  Full `blocking_wait` is unloaded in lists; load it only during explicit
  selected-job/wait inspection. Do not infer dependencies from either empty field.
- Opening an input form or taking a task action still requires the existing
  fresh task-handle lookup and stale-occurrence protection. List coordinates
  do not replace those checks.
- Keep payload and revision together when updating cached rows. Preserve large
  integers such as `9007199254740993`: ordinary JavaScript JSON number parsing
  cannot round-trip them exactly. If the UI exposes raw payload viewing/editing,
  retain raw JSON or use a lossless parser; do not rewrite the opaque payload
  from a normal parsed JavaScript object. Payloads can be 65,536 bytes.

## Acceptance checks

1. Verify ordinary, archived, child, and keyed rows render exact completion text,
   unfamiliar completion statuses, empty details, and retries without a terminal
   completion. Check text escaping and multiline/Unicode display.
2. Exercise payload absence, explicit null, arrays, unrelated namespaces, a
   large integer, and a 65,536-byte payload. Confirm the model retains payload
   and revision together without pruning or precision loss.
3. Check published demand overrides submission requirements. Check missing
   payloads with and without submission metadata, including nonzero revisions;
   then transition the same row to active and completed and verify its retained
   view replaces current demand. Remove both sources and verify the line disappears.
   Preserve invalid-demand diagnostics in Env.
4. Verify every task coordinate, prerequisite ordering, wait ID/deadline, and
   archived coordinates survive adapters. Keep task data and evaluated wait
   explanations behind explicit inspection.
5. Instrument pages of 1 and 200 rows: one backend list call, unchanged
   continuation, and no hydration calls. Verify that already-supplied fields
   render immediately without a detail request or a perpetual loading state.

The c2j restoration tests cover SQLite and an HTTP-wrapped runtime, JSON
projections, CLI parity, and the one-page rule. Cortex should add UI/adapter
coverage and verify its deployed pgjobdb endpoint separately.
