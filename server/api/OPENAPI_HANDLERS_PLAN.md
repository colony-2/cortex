# API Handler Implementation Plan

Updated after re-reading the regenerated `server/openapi` bindings. The API surface now centers on projects, cells, project graphs, tickets, and recipe-input management routes. This plan describes how to implement handlers in `server/api` that satisfy the OpenAPI contract using the generated Go types (do not reimplement schemas) and the existing service packages (`server/project`, `server/cell`, `server/graph`, `server/ticket`, and recipe-input management).

## Objectives & Scope
- Provide full coverage of the OpenAPI endpoints exposed in `server/openapi/pkg/openapi/generated.go`.
- Use the generated request/response types for marshaling/unmarshaling and validation helpers (e.g., `runtime.DecodeStyle` from `oapi-codegen`).
- Wire handlers to existing domain services:
  - Projects → `server/project/pkg/project` service + GORM store.
  - Cells → `server/cell/pkg/cell` service + store.
  - Project graph → `server/graph/pkg/graph` builder (project-scoped root).
  - Tickets → `server/ticket/pkg/ticket` service + stores/event store.
  - User-input (SSE + management) → register via recipe-input management service (`opssetup.SetupOps`).
- Keep router/middleware behavior from `internal/handlers` while adding OpenAPI routes under `/api`.

## OpenAPI Surface (what to implement)
- **Projects**  
  - `GET /api/projects` with filters (`ids`, `names`, `nameContains`).  
  - `POST /api/projects` create (body `ProjectCreateRequest`).  
  - `GET /api/projects/{projectId}` fetch.  
  - `PATCH /api/projects/{projectId}` update (body `ProjectUpdateRequest`).  
  - `DELETE /api/projects/{projectId}` delete.
- **Cells (per project)**  
  - `GET /api/projects/{projectId}/cells` with filters (`ids`, `names`, `nameContains`, `pathPrefix`, `includeDeleted`, `dependsOn`, `populator`, `populatorIds`).  
  - `POST /api/projects/{projectId}/cells` create (body `CellCreateRequest`).  
  - `GET /api/projects/{projectId}/cells/{cellId}` fetch.  
  - `PATCH /api/projects/{projectId}/cells/{cellId}` update (body `CellUpdateRequest`).  
  - `DELETE /api/projects/{projectId}/cells/{cellId}` soft delete.  
  - `PUT /api/projects/{projectId}/cells/{cellId}/dependencies` replace dependencies (body `CellDependenciesRequest`).
- **Project Graph**  
  - `GET /api/projects/{projectId}/graph` return graph as `openapi.Graph` (cells/edges).
- **Tickets (per project)**  
  - `GET /api/projects/{projectId}/tickets` filter by stages, states, actors, cells, created/updated ranges.  
  - `POST /api/projects/{projectId}/tickets` create (body `TicketCreateRequest`).  
  - `GET /api/projects/{projectId}/tickets/stages` aggregate stages with filters.  
  - `GET /api/projects/{projectId}/tickets/states` list states (simple pass-through from service).  
  - `GET /api/projects/{projectId}/tickets/{ticketId}` fetch.  
  - `PATCH /api/projects/{projectId}/tickets/{ticketId}` update (body `TicketUpdateRequest`, includes optimistic `expectedVersion`).  
  - `GET /api/projects/{projectId}/tickets/{ticketId}/at` historical view at timestamp (`at` query).
- **Recipe Input Management (auto-registered via ops management services)**  
  - `/api/user-inputs/pending`, `/api/user-inputs/stream`, `/api/user-inputs/{jobId}`, `/api/user-inputs/{jobId}/respond`, `/api/user-inputs/{jobId}/cancel` are provided by the recipe-input `ManagementService`. They must be attached by the ops management/extension pipeline (`opssetup.SetupOps` → `ExtensionRoutes`) rather than direct handler wiring inside `server/api`.

## Wiring & Dependencies
- Extend `web.Server` construction to inject new services:
  - Project store/service (GORM; configure DB connection or in-memory for tests).
  - Cell store/service (requires project service; same DB).
  - Ticket store/event store/service (requires project + cell services).
  - Graph builder per project: likely rooted at each project’s `GitRepoPath` (derive project root for `graph.NewBuilder`).
- Provide helper layer in `internal/handlers` to resolve `projectId` → project record → root path and to adapt service types to OpenAPI types.
- Reuse existing middleware (`Recovery`, `Logging`, `RouteLogging`, CORS) and the SPA routing behavior for non-API paths.

## Handler Design (per resource)
- **Projects**: Map create/update/list/get/delete to project service methods. Translate service errors (`ErrEmptyName`, `ErrEmptyGitRepo`, `ErrNotFound`, `ErrVersionConflict`) to HTTP statuses (400/404/409). Serialize using `openapi.Project`.
- **Cells**: Ensure project exists before operations. Use cell service for CRUD + `ReplaceDependencies`. Respect `includeDeleted` flag for search. Map optimistic lock and validation errors appropriately. Convert models to `openapi.Cell` (include dependencies via store/service accessor if needed for GET).  
  - Dependency update: ignore self refs/duplicates as per service; return 204/200 per spec (spec shows empty response body).
- **Graph**: Build using graph builder scoped to project’s repo path; convert `core.Graph` to `openapi.Graph` (cells/edges id/name/path/type/dependencies).
- **Tickets**: Use ticket service for CRUD, stage/state queries, time-travel fetch. Map actor/user/agent fields and optimistic locking (`expectedVersion`). Translate domain errors (`ErrInvalidProject`, `ErrInvalidCell`, `ErrInvalidState`, `ErrVersionConflict`, `ErrNotFound`) to HTTP 400/404/409.
- **User Inputs**: Expose routes via extension registration; ensure SSE stream uses existing recipe-input management service wiring (created in `opssetup.SetupOps`).

## Routing & OpenAPI Integration
- Add OpenAPI handler registration in `internal/handlers` (e.g., an adapter struct implementing handlers or a manual mux using generated `New*Request` decoding helpers). Ensure all routes live under `/api` without breaking SPA fallback.
- Keep extension route support; merge ops/management-provided routes (including recipe-input) from `opssetup.SetupOps` so they are attached to the `/api` subrouter via the extensions mechanism, not by importing their handlers directly.
- Consider adding request validation using the generated swagger (`openapi.GetSwagger()`) with `oapi-codegen` middleware if helpful, but prioritize matching types and status codes.

## Data Conversion & Error Mapping
- Centralize conversion helpers:
  - Project model ↔ `openapi.Project`.
  - Cell model ↔ `openapi.Cell`/`CellCreateRequest`/`CellUpdateRequest`.
  - Ticket model ↔ `openapi.Ticket` plus aggregations for stages/states.
  - Graph `core.Graph` ↔ `openapi.Graph`.
- Standardize error responses (JSON with message) and HTTP codes for validation (400), not found (404), conflict/optimistic lock (409), and internal errors (500).

## Tests & Verification
- Handler tests using `httptest`:
  - Projects: create/list/get/patch/delete success and validation errors.
  - Cells: create/list/get/update/delete/deps, include deleted toggle, dependency replacement behavior.
  - Graph: build against a temporary repo tree to verify edge/cell mapping.
  - Tickets: create/list/filter/update (optimistic lock), stage/state endpoints, historical fetch.
  - User-input routes: ensure extension wiring exposes endpoints and streaming endpoint responds with correct headers.
- Use in-memory or temporary SQLite/Postgres (matching existing ticket/project/cell store test utilities) to exercise real stores.
- Optionally validate request/response against swagger using generated client in tests for shape conformance.

## Open Questions / Follow-ups
- Confirm DB configuration expectations for `server/api` (env flags, DSNs) and document in `README`/`VIBETHIS.md`.
- Determine canonical project root → graph builder mapping (from `GitRepoPath` or separate field).  
- Decide on response shapes for dependency updates and deletions when service returns no content; align with status codes defined in the regenerated spec.
