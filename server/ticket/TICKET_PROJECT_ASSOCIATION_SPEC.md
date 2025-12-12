# Ticket Project Association

## Goals
- Require every ticket to belong to a project so downstream consumers (UI, ops, analytics) can scope work to project boundaries.
- Keep ticket search/stage aggregation project-aware without losing existing cell filters.
- Reuse the `server/project` types and service to validate referential integrity instead of introducing ad-hoc project fields.

## Non-Goals
- Defining how cells map to projects; we only record the project ID alongside the existing `cell_name`.
- Allowing tickets to move between projects after creation. Any future move will need its own workflow and event semantics.
- Building UI changes in this spec; they will follow once the backend surfaces are stable.

## Current State
- `tickets` table tracks `cell_name`, stage/state, actor, and temporal slices but has no project foreign key.
- Search filters only accept `Cells`, `StageAny/NotIn`, `States`, and actor/time windows.
- Event and reset tables (`ticket_events`, `ticket_resets`) key off `ticket_id` only, so cross-project filtering requires joins or extra context.
- Ticket APIs (`CreateTicket`, `UpdateTicket`, `SearchTickets`, `ticket.manage` op) have no notion of project.

## Proposal
### Data Model
- **Project linkage:** Add `ProjectID project.ID` to `model.Ticket` with `gorm:"column:project_id;type:char(27);index"`. Export through `pkg/ticket.Ticket` and mirror on public DTOs. The field is required on creation and immutable afterward. Extend `model.SearchFilter` with `Projects []project.ID`. Add `ProjectID project.ID` to `model.TicketEvent` and `model.TicketReset` so event listings can filter without joining tickets.
- **Identifiers:** Switch tickets (and their related event/reset IDs) to KSUIDs for higher entropy and timestamp sorting:
  - Ticket `ID` → KSUID (27-char base62) via a new KSUID generator in `ticket/internal/idgen`; update GORM column to `char(27)`.
  - Event IDs and reset IDs also use KSUIDs for consistency; column lengths updated to `char(27)`.
  - Public helpers expose a `NewKSUIDGenerator()` similar to `project` for opt-in overrides; default service config uses KSUID.

### Service & Validation
- Reuse the existing `project.Service` (or `project.Store`) as a dependency rather than defining a new reader interface. `ServiceConfig` gains a `Projects project.Service` (preferred) or `ProjectsStore project.Store` field:
  - `CreateTicket` requires a non-empty `ProjectID` and resolves it via `Projects.GetProject` (or `ProjectsStore.Get`); missing/unknown IDs return `ErrInvalidProject`.
  - `UpdateTicket` rejects attempts to change `ProjectID`; slices inherit the original value.
- `CellName` stays as-is; future cell↔project validation can reuse the same project dependency once mappings are defined.

### Storage & Indexing
- `tickets` table: add `project_id CHAR(27) NOT NULL` once backfill completes; interim migrations allow NULL for legacy rows. Add compound indexes:
  - `(project_id, stage, state, updated_at)` for list queries
  - `(project_id, cell_name)` to support project+cell scoping
- `ticket_events` table: add `project_id CHAR(27)` (mirrors parent ticket) plus index `(project_id, event_time, ticket_id)`.
- `ticket_resets` table: add `project_id CHAR(27)` plus index `(project_id, created_at)`.
- Populate event/reset `project_id` automatically from the parent ticket inside service methods; callers do not need to pass it.

### Query Surfaces
- `SearchTickets`/`SearchStages`: apply the new `Projects` filter alongside existing predicates.
- `ListEvents`: accept `ProjectID` via the stored column so consumers can fetch events for a ticket already scoped to a project.
- `ticket.manage` op:
  - `create_ticket` action requires `project_id` (string, KSUID length 27).
  - Context patch includes `ticket.project_id`.

### API/DTO Touchpoints
- Add `project_id` to any JSON-exposed ticket representations (REST, SSE, op outputs) in `server/api` and `web/openapi` generation.
- Extend `CreateInput`/`UpdateInput` JSON tags accordingly and adjust validations/errors (`ErrInvalidProject`).

### Migration & Rollout Plan (fresh start)
- Ship schema with `project_id` as `NOT NULL` from day one for `tickets`, `ticket_events`, and `ticket_resets`.
- No backfill/compat paths required; fail fast if the field is missing anywhere (validation + DB constraint).

### Testing
- Unit: validation fails without project ID; create succeeds with valid project; update cannot change project; search filters by project.
- Store: filters hit only matching project rows; events/resets persist `project_id`; NOT NULL constraints enforced.
- Integration: ticket.manage create + search + append event asserts `project_id` in context patch and persisted rows.

### Impacted Packages
- `server/ticket/internal/model`, `internal/store/tickets`, `internal/store/events`, `internal/service`
- `server/ticket/pkg/ticket`, `server/ticket/pkg/op`
- `server/api`/`web/openapi` DTOs if they expose tickets
