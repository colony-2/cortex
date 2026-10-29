# Cortex: render compact job lists and inspect selected attempts

## Problem and required outcome

JobDB and c2j ordinary job lists now omit payloads, task coordinates, dependency
explanations, additional goal memberships, and completion detail. Cortex must
render compact rows and load details only for a selected/expanded attempt.
Opening or refreshing a collapsed list must issue one JobDB list operation per
page, with zero per-job inspection operations for its rows. Background navigation
counts and pending-input monitoring must also avoid per-job detail reads. The
goal is to eliminate detail reads that scale with individual list entries, not to ban
unrelated requests or explicitly selected inspections.

All pending inputs and document reviews appear together in one Reviews list and
one navigation badge. Rows use compact job summaries. Only opening an item loads
its form and determines whether to render a document review or a simple form.

This request is based on Cortex main
[`e7bfa71bcd91c9cfa5dbc8ce61874624d2ce71bf`](https://github.com/colony-2/cortex/tree/e7bfa71bcd91c9cfa5dbc8ce61874624d2ce71bf),
audited on 2026-10-28. See [c2j migration](MIGRATION-C2J-Compact-Job-Lists.md).

## Audit findings

| Source | Current behavior | Required change |
| --- | --- | --- |
| `server/internal/cortex/handlers.go:handleListJobs` | One `recipejob.ListRecipeJobs` call; no detail loop | Preserve this path and backend pagination. Never hydrate results in the handler. |
| `handlers.go:handleGetJob` | Calls `recipejob.GetRecipeJob` | Keep as selected inspection; upgraded c2j reads one summary and one detail. |
| `handlers.go:requireJob` | Uses `GetRecipeJob` just to check existence | Use a compact keyed list/existence helper if no details are consumed, avoiding the newly explicit detail fetch. |
| `web/app/src/components/JobsListPage.tsx` | Groups on primary and `other_goal_ids`; renders completion text and dependency links from rows | Group unfiltered rows only by primary goal. Use query goal context on goal-specific pages; backend goal filtering includes additional members. Remove list-only detail assumptions. |
| `GoalJobOverview.tsx` | Passes a selected list object directly into `JobListDetails`; renders progress for every attempt | Fetch the selected attempt through `getJob`, not every attempt. Keep the attempt table compact. |
| `JobWaitLinks.tsx` | Derives dependency-target links from `blocking_wait` or `wait_for` | Render a generic dependency-wait indicator from `blocking_wait_summary` / status on compact rows. Show target links only after inspection. Input/review links can still use `next_route`. |
| `JobListDetails.tsx` | Reads completion detail, dependency targets, execution demand, and task coordinates | Render loaded inspection data with loading/error states, rather than treating absent list values as empty details. |
| `JobStoryPage.tsx` | Already calls `getJob` for the selected job | Preserve selected detail fetching; do not introduce a list-wide prefetch. |
| `web/shared/src/types.ts` | One `RecipeJob` interface mixes summary and detail fields | Add the compact wait summary and distinguish list summaries from inspected details so absence cannot imply authoritative emptiness. |
| `server/internal/cortex/reviews.go:handlePendingInputs` | Fetches a form for every pending input to classify and label it | Return compact summaries for all pending inputs without fetching forms. |
| `server/internal/cortex/input_stream.go:handleInputStream` | Polls waiting task handles, requiring task coordinates | Monitor compact pending-job summaries without per-job inspection. |
| `PendingInputsListPage.tsx` / `useReviewJobMetadata.ts` | Splits inputs from reviews and fetches each review's job details | Show one Reviews list; derive labels from compact summaries and remove per-row detail fetching. |
| `App.tsx` / `InputActivityContext.tsx` | Separate navigation entries and counts based on form kind | Use one Reviews entry and combined pending count without reading forms. |

## Implementation requirements

1. Upgrade c2j and JobDB to the latest `main` of each at implementation time,
   recording the resolved versions in the Go dependency files.
2. Retain title/prompt, cell/repository labels, primary goal, parent, completion
   status, timestamps, route, and cancellation in the ordinary list. c2j now
   also exposes all application metadata, schema hash, and payload revision.
3. Add `blocking_wait_summary?: { waitId: string; deadline?: string }`. A
   summary says a stored wait is blocking; it cannot identify pending targets.
   Do not infer target goals, membership, or absence of dependencies from empty
   detail fields. Preserve unknown completion status values.
4. When a group is expanded, fetch only its selected attempt. On attempt change,
   fetch that attempt; prevent late responses from replacing another selection.
   Scope caches by tenant and job and retain loading/error/retry behavior.
   Refresh reloads the list and each visible selected attempt. Reopening a group
   revalidates cached details. Collapsed panels issue no detail requests, including
   previously opened panels that remain mounted. Payload revision alone is not a
   cache version for completion/dependency changes.
5. Keep detail state separate from list state. A list refresh must not overwrite
   already loaded details with zero-value compatibility fields. Conversely,
   previously inspected dependencies must not become stale list-wide facts.
6. Do not restore secondary membership grouping with one `GetJob` per row.
   Primary-goal grouping and server-side goal filtering are sufficient for the
   compact list. Authoritative membership/goal exploration belongs to explicit
   goal/job inspection.
7. Combine all pending inputs into the Reviews list, with one navigation entry
   and combined badge. Do not classify or filter entries by form kind before
   inspection. Use job title/prompt, cell, status, and other available summary
   fields for rows. Form-specific titles, document counts, and request identity
   are loaded only after opening an item; missing values must not appear as zero
   documents or an authoritative form classification.
8. Load the selected item's form and choose the renderer from its actual kind:
   document-review UI for reviews, the appropriate form UI otherwise. Preserve
   existing submission and request-identity validation. Remove legacy input-list and input-detail routes; use Reviews URLs throughout.
9. Use compact filtered list operations for pending rows, navigation counts,
   and background monitoring. Remove form-fetch loops, per-row `getJob` calls,
   and task-handle discovery from these paths. Keep backend eligibility filtering
   and pagination; a tenant-wide badge must account for all result pages, not
   just the visible jobs page. Monitoring must handle successive input requests
   on the same job without fetching task coordinates for every pending job.

## Where the removed loop could reappear

Do not add `Promise.all(page.jobs.map(getJob))`, a server-side `GetRecipeJob`
loop, eager hooks on every collapsed row, or requests for all attempts when a
single attempt is selected. Likewise do not call pending-input discovery that
loads forms or task coordinates to hydrate rows or update navigation badges.
A selected panel may fetch one job; the cost must scale with explicit visible
inspections, not the number of list rows.

## Acceptance tests

- Instrument the backend: each jobs-list request for a page of 100–200 collapsed
  jobs yields one `ListJobs` and zero `GetJob`, `GetJobRun`, goal, wait, or artifact
  reads. Separate compact requests for navigation counts or monitoring are
  allowed, but must not introduce per-entry detail reads.
- Compact fixtures have nonempty payload revision and wait summary, but no
  detail fields. The page retains labels, statuses, pagination, parent links,
  and input/review links. An empty page with a cursor remains navigable.
- Expanding one group fetches only the selected attempt; choosing another
  fetches only that attempt. Cover close/reopen, refresh, errors, rapid selection,
  tenant switches, and persisted expanded state in the URL.
- Full dependency links, completion messages, task coordinates, and execution
  demand appear after inspection. Missing list data is never shown as “none”.
- Goal-specific filtering finds additional members without exposing or fetching
  all their other memberships. Unfiltered grouping uses the primary goal only.
- Update `JobsListPage.test.tsx` and `web/app/tests/goals.spec.ts`, which currently
  supply full completion/dependency details directly in list fixtures. Add
  separate compact list and selected-job responses with request-count assertions.
- Mixed simple inputs and document reviews appear in the same Reviews list and
  combined badge without form or per-job detail reads. Opening one item loads
  only its form and renders the correct UI; both submission paths still work.
- Cover pending-list pagination, tenant-wide counts, removal of legacy input URLs, and
  background monitoring, including successive requests on the same job. Assert
  no per-entry detail reads from navigation counts, polling, or list refreshes.

Coordinate rollout with the c2j migration. Merely bumping the dependency leaves
expanded panels incomplete and changes grouping silently.
