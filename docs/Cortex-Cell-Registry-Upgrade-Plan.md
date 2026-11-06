# Cortex cell registry upgrade

Implemented 2026-11-04; dependencies refreshed 2026-11-05. This document updates
the migration design in
[GUIDE-CORTEX-CELLS.md](../GUIDE-CORTEX-CELLS.md) for the current upstream
contract. In particular, root cells have no special role and submitting a job
cannot implicitly register a cell.

## Dependency baseline and scope

Cortex pins c2j `v0.0.120` and JobDB `v0.0.71`.
The bundled c2j skills are refreshed from the pinned worker. This pair includes
the remote route-validation fix described in
[JobDB-Cell-Route-Validation-Bug.md](JobDB-Cell-Route-Validation-Bug.md).

The required migration includes registry browsing, independent cell creation,
cell details, goals filtered by cell, and registry-based goal submission.
Authentication, per-user authorization and credential forwarding remain deferred
until JobDB supports them. Optional editing and retirement screens are also
separate follow-up work.

This remains a coordinated hard break: fresh execution state, required cell
fields, no metadata identity fallback, and no old-job migration or compatibility
UI. The deployment reset has **not** been performed by this implementation.

## Identity and repository contract

| Concept | Cortex behavior |
| --- | --- |
| Identity | Use tenant ID and stable cell ID for routes, selections and caches. |
| Name | Display the current registry name and send names to c2j selectors and submissions. Resolve stored IDs before querying jobs. |
| Current repository | Registry and fresh-submission views show the current canonical repository locator. |
| Job source | Required job `repo` remains the submitted repository snapshot; registry edits do not rewrite historical source links. |
| Lifecycle | Browse active, retired or all cells. Fresh goals require an active cell. |
| Root | `root` is an ordinary valid name. There is no root filter, badge, bootstrap requirement or reserved-name validation. |
| Invalid response | Missing required ownership fields fail visibly with retry/reload UI, including pending-input streams. |

Job responses retain c2j spelling: required `cell_id`, `cell_name` and `repo`.
Registry responses use JobDB's cell shape and paginated
`{ cells, nextPageToken? }` envelope. Metadata remains arbitrary nested JSON;
revisions are opaque. Git refs are execution settings, not registry identity.
Local c2j `self` and `root` configuration may still supply execution defaults;
they do not create registry records or confer authority.

## Server and client integration

Cortex receives an explicit `jobdb.CellRuntime` for registry operations and
passes the actual runtime to story services. Config discovery and job metadata
no longer provide a second cell catalog. Embedded test servers supply the trusted
runtime context required by JobDB; this is fixture setup, not authentication.

Routes under `/api/projects/:tenantId`:

| Route | Contract |
| --- | --- |
| `GET /cells` | Forward exact `name`, lifecycle `state`, `pageSize` and `pageToken`; return registry pagination. |
| `GET /cells/:cellId` | Fetch by stable ID, including retired cells; retain revision/ETag. |
| `POST /cells` | Register name/repo with optional description, metadata and request ID. |
| `GET /jobs` | Forward repeated `cellNames` before pagination, including display-status scans. |
| `POST /jobs` | Require a registered cell name; optional `repo` is a repository assertion. |

Legacy `/cells/self`, URL-as-cell submission, repo filter aliases and `isRoot`
queries are rejected. Request decoding is strict, and backend cell errors retain
their status/code. Name-only submission resolves through the registry without
synthesizing a repo assertion from local naming patterns. An explicit mismatched
assertion fails rather than changing the target.

## Browsing and filtering

The Cells page uses registry pagination, exact-name lookup and lifecycle filters,
so cells with no jobs are visible. The shared registry hook is tenant-scoped.
Selected IDs can be resolved independently of the currently loaded registry page.

Cell detail routes use stable IDs and offer **New goal in this cell** and
**Goals for this cell**. The latter opens the goals page with a `cellId` filter.
The jobs/goals pages resolve selected IDs to current names, then ask the backend
to filter before pagination. An unavailable selection produces an error instead
of silently broadening the query. Retired-cell history remains accessible.

Tenant or filter changes reset pagination and action selections. The
`executionState=cells-v1` URL marker discards legacy execution-state selections
and cursors. HTML is served with revalidation so old cached shells do not persist
across the release. Deployment must still avoid mixed application versions.

## Cell creation and goal submission

The required **Create cell** interface works independently of goal submission,
including when the registry is empty. It accepts a required name and repository,
optional description, and optional JSON-object metadata. Backend normalization
supplies the canonical repository. Registration does not create a Git repository.

The dialog preserves values on failure, blocks duplicate submissions and reuses
the request ID when retrying the same payload after an uncertain result. Success
opens the returned cell's detail route, where users can view its goals or create
a goal with the stable ID preselected.

New goal has two modes:

1. **Existing cell:** select an active registry cell, refresh it by ID, and submit
   its current name. No implicit repository assertion is derived from local config.
2. **New cell:** explicitly register name/repo first, then submit the goal with
   the returned canonical identity and repository assertion. The cell remains
   registered if the later goal submission fails; the dialog explains this and
   allows retry or later use of the cell.

Registration retries use a separate request ID from goal submission. Editing the
prompt after an uncertain registration keeps the same cell request ID. A failed
goal submission does not repeat a successful registration. Retrying the same goal
payload retains its job ID to avoid duplicate work after an uncertain response.
Changing the target or goal payload starts a new goal attempt.

The backend rejects submissions to unregistered cells even when a repository is
provided. Tests explicitly create fixture cells before submitting work. `root`
uses exactly the same registration and submission path as other names.

## Secondary views and validation

Job summaries, goal attempts, inspection, story, pending inputs and reviews use
required first-class ownership. Source links preserve job repository snapshots.
No production path reconstructs ownership from loaded-job metadata or repo URLs.

Fixtures register portable hosted repository identities. Browser recipe tests
map Git transport to temporary local repositories through a test-only Git shim;
production normalization and transport are unchanged. Workers are built from the
same c2j version as the server dependencies.

Validation covers:

- Registry pagination, tenant isolation, lifecycle, exact lookup and creation
  without jobs; metadata and error propagation.
- Name-only submission, explicit repo mismatch, rejection of unknown cells, and
  ordinary create/rename/retire behavior for a cell named `root`.
- Stable-ID filtering and rename handling, historical repository snapshots,
  registration/submission retry identities and visible protocol errors.
- Cell creation and filtered-goals links on desktop and mobile, review streams,
  recipe inputs, restart, tenant switching and browser zoom.
- Remote SQLite tenants `cells`, `cells-example` and `registry-example`, including
  job ID `cells-job`, list/input queries, submission and cancellation. This guards
  the upstream route-validation regression.

Run the Go server/CLI suites, web typechecking and unit suites, production and
pinned-worker builds, and the full Playwright suite. Browser image-review fixtures
require FFmpeg; a temporary installation was used in this environment.

## Deployment and follow-up

Coordinate JobDB/pgjobdb, c2j, workers, recipes and Cortex as one release. Stop old
submissions/processes, provision fresh execution stores and artifact namespaces,
invalidate old leases/cursors, reapply settings, explicitly register needed cells,
then deploy the new builds and recreate desired schedules/work. There is no
special root bootstrap. Use the release runbook for any destructive reset; Cortex
startup does not delete execution state. Rollback cannot run old code against the
new execution store.

Optional edit/retire UI should preserve strong revision preconditions, complete
metadata maps, conflict reload and idle-only retirement. Repository edits must
also expose affected schedules and their target-update requirements before that
management interface ships. These screens and future authentication do not block
the implemented registry migration.
