# Cells Sync API Spec

## Goal
Expose an HTTP endpoint that lets the UI trigger a cell sync against a graph provider (defaulting to the Moon-based graph builder), and plumb it through the generated OpenAPI client plus the existing cell service sync pipeline.

## Current State (what we can reuse)
- **Cell service sync**: `server/cell/internal/service/service.go` already implements `SyncFromPopulator(projectID, populator, opts)` and returns `SyncResult{Created, Updated, Restored, Deleted, Skipped, DependenciesUpdated, AffectedIDs}`.
- **Moon populator**: `server/cell/pkg/cell/graph_populator.go` exposes `NewGraphPopulator(rootPath)` with the name `graph/moon`, backed by `graph.NewBuilder` (executes `moon project-graph --json`).
- **API surface**: `api/openapi/vibethis-api.yaml` currently has cell CRUD only; no sync route. `server/api/internal/handlers/api.go` wires CRUD routes and has service + graph factory plumbing we can lean on. `server/openapi/pkg/openapi/generated.go` is generated from the spec via `codegen.yml`.
- **Project roots**: handlers fetch projects via `h.projects` (see list/get/update/delete); `GitRepoPath` is already used in the testserver’s `graphFactory` for graph building.

## Step 1 — Add OpenAPI route in `/src/api`
Add a new path to `api/openapi/vibethis-api.yaml`:
- **Path**: `POST /api/projects/{projectId}/cells/sync`
- **Tag**: `Cells`
- **Request body** (`application/json`, schema `CellSyncRequest`):
  - `populator` (string, optional, default `"graph/moon"`). Unknown values → 400.
  - `pruneMissing` (boolean, optional, default `false`) to soft-delete cells not returned by the populator.
- **Response** (`200`, schema `CellSyncResult`), fields mirror `cell.SyncResult` plus the resolved `populator`:
  - `populator`, `created`, `updated`, `restored`, `deleted`, `skipped`, `dependenciesUpdated`, `affectedIds` (array of strings).
- **Errors**:
  - `400`: invalid project ID or unsupported populator.
  - `404`: project not found.
  - `500`: populator execution/build failures (e.g., moon errors).
- **Defaults/behavioral notes**:
  - Default populator uses the Moon graph builder rooted at the project’s `gitRepoPath` (falls back to existing graph factory behavior when available).
  - `pruneMissing` opts into soft-deleting managed cells that no longer appear in the provider payload.

## Step 2 — Regenerate `server/openapi`
- Command: `moon run server-openapi:generate` (same flow as `AGENTS.md` describes).
- Expected output changes in `server/openapi/pkg/openapi/generated.go`:
  - New models: `CellSyncRequest`, `CellSyncResult`.
  - New client helpers: `PostApiProjectsProjectIdCellsSync*` and response wrappers.
  - Embedded spec includes the new route.
- No manual edits inside generated files.

## Step 3 — Wire handler in `server/api`
Add a handler that drives the existing sync pipeline:
- **Route registration**: in `server/api/internal/handlers/api.go`, add `POST /projects/{projectId}/cells/sync` via `registerAPIRoutes` with `withHandlerLog("cells:sync", ...)`.
- **Handler logic** (new `handleSyncCells`):
  - Parse `projectId` from path; fetch project via `h.projects` to validate existence and obtain `GitRepoPath`.
  - Decode `openapi.CellSyncRequest`; default `populator` to `"graph/moon"` and `pruneMissing` to `false`.
  - **Populator selection**:
    - If `populator` is empty or `"graph/moon"`, create `cell.NewGraphPopulator(project.GitRepoPath)` (this wraps `graph.NewBuilder` which shells out to Moon).
    - For any other populator name, return `400 Bad Request` (unsupported).
  - Call `h.cells.SyncFromPopulator(ctx, projectID, populator, cell.SyncOptions{PruneMissing: body.PruneMissing})`.
  - Map domain errors using existing helpers (`cellErrorStatus`, project errors) and return `openapi.CellSyncResult` on success.
  - If a `GraphFactory` is present (see `handlers.GraphFactory`), we can optionally use it to derive a project-scoped builder root; otherwise stick to `project.GitRepoPath`.
- **Response mapping**: translate `cell.SyncResult` to `openapi.CellSyncResult` (include `AffectedIDs`).
- **Testing hooks**: mirror `internal/handlers/api_integration_test.go` style to cover:
  - Happy path sync (creates cells from Moon populator stub and returns counts).
  - Unsupported populator → 400.
  - Missing project → 404.

## Pulling the pieces together
- The new OpenAPI path defines the contract → regeneration yields Go types and client calls.
- The handler uses the project service to locate the repo root, constructs the Moon-based populator, and reuses the cell service’s `SyncFromPopulator` to perform all CRUD/dependency reconciliation.
- The generated client can be used by the UI (cells view sync button) to trigger the sync and display the returned counts/affected IDs.
