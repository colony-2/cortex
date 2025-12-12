# Cell Service Spec

## Goals
- Persist cells in Postgres with KSUID identifiers and optimistic locking, matching the patterns used in `server/project` and `server/ticket`.
- Enforce that every cell belongs to a project and is uniquely named per project.
- Support both manual creation/update flows and bulk population from graph builders (starting with the Moon-based graph populator in `server/graph`).
- Track cell-to-cell dependencies (edges) and keep them in sync with the graph populator.
- Use soft deletion only; no hard deletes of cell rows.

## Non-Goals
- Building HTTP/CLI surfaces for cells in this change; the spec only covers the module/service layer.
- Enforcing cell-to-ticket relationships or UI wiring; those can build on top of the service.
- Implementing graph builders themselves (reuse `server/graph` and future builders via a pluggable interface).

## Module Layout (mirrors `project`/`ticket`)
- `internal/model`: `Cell` struct, `ID` typedef (`char(27)` KSUID), `Dependency` struct, search filters, and the populator DTOs.
- `internal/store`: GORM-backed store with `WithTx`, `Create`, `Get`, `Search`, `Update`, `SoftDelete`, `ReplaceDependencies`, `ListDependencies`, `DB`, and iterator utilities.
- `internal/service`: Business logic, validation, ID generation, clock, populator sync orchestration.
- `internal/idgen`: KSUID generator identical to `project/internal/idgen`.
- `pkg/cell`: Public facade exporting types/errors, `NewService`, `NewStore`, `NewKSUIDGenerator`, and helpers for populators.

## Data Model (Postgres via GORM)
```go
type Cell struct {
    ID          ID                     `gorm:"type:char(27);primaryKey"`
    Version     optimisticlock.Version `gorm:"column:version"`
    ProjectID   project.ID             `gorm:"column:project_id;type:char(27);index;not null"`
    Name        string                 `gorm:"column:name;not null"`          // Moon node ID / logical name
    Description string                 `gorm:"column:description;default:''"` // optional, trimmed
    WorkingPath string                 `gorm:"column:working_path;not null"`  // absolute or repo-relative path
    Populator   string                 `gorm:"column:populator;default:''"`   // e.g., "graph/moon"
    PopulatorID string                 `gorm:"column:populator_id;default:'';index"`
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time             `gorm:"column:deleted_at;index"`       // nil when active
}

type Dependency struct {
    ID          uint   `gorm:"primaryKey;autoIncrement"`
    ProjectID   project.ID `gorm:"column:project_id;type:char(27);index;not null"`
    FromCellID  ID     `gorm:"column:from_cell_id;type:char(27);index;not null"`
    ToCellID    ID     `gorm:"column:to_cell_id;type:char(27);index;not null"`
    CreatedAt   time.Time
}
```
- Table: `cells`. `TableName()` returns `"cells"`.
- Uniqueness: `(project_id, name)` unique where `deleted_at IS NULL` (partial unique index).
- Populator identity: unique `(project_id, populator, populator_id)` where `deleted_at IS NULL` to allow stable mapping when names change upstream.
- Foreign key: `project_id` references `projects.id` (deferrable constraint via GORM, enforced in DB).
- Timestamps stored in UTC; `DeletedAt` marks soft delete state.
- Dependency table: `cell_dependencies` with unique index `(project_id, from_cell_id, to_cell_id)` and FKs to `cells.id` (cascade soft delete handled in service). Dependencies are restricted to the same project; cross-project dependencies are rejected.

## Service Surface
```go
type Service interface {
    CreateCell(ctx, input CreateInput) (*model.Cell, error)
    GetCell(ctx context.Context, id model.ID) (*model.Cell, error)
    ListCells(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Cell], error)
    UpdateCell(ctx context.Context, id model.ID, patch UpdateInput) (*model.Cell, error)
    MarkDeleted(ctx context.Context, id model.ID) error
    ReplaceDependencies(ctx context.Context, id model.ID, deps []model.ID) error
    SyncFromPopulator(ctx context.Context, projectID project.ID, pop Populator, opts SyncOptions) (*SyncResult, error)
}
```
- `CreateInput`: `ProjectID`, `Name`, `Description`, `WorkingPath`. Trims strings; `Name` and `WorkingPath` required.
- `UpdateInput`: optional `Name`, `Description`, `WorkingPath`. No project changes; rejects empty/whitespace updates.
- `MarkDeleted`: sets `DeletedAt` timestamp (no hard delete). Double-delete is a no-op.
- `ReplaceDependencies`: replaces all outbound dependencies for the given cell with the provided list (ids must be in the same project, no self-edge). Removes missing edges and inserts new ones idempotently.
- Errors (exported via `pkg/cell`): `ErrEmptyName`, `ErrEmptyWorkingPath`, `ErrInvalidProject`, `ErrIDGeneration`, `ErrVersionConflict`, `ErrNotFound`, `ErrAlreadyDeleted`.
- Validation: uses `project.Service` (preferred) or `project.Store` in `ServiceConfig` to verify `ProjectID` exists before create/sync. Strings are trimmed; `Description` may be empty; paths stored as provided after trim.
- Concurrency: `UpdateCell` uses optimistic lock; store returns `ErrOptimisticLock` mapped to `ErrVersionConflict`.
- Defaults: `ServiceConfig` injects `Store`, `Clock`, `IDGen`; falls back to system clock + KSUID generator when nil.

## Store Behavior
- `Create`: inserts row; respects unique (project_id, name) on non-deleted rows.
- `Get`: filters out soft-deleted rows by default (`deleted_at IS NULL`).
- `Search`: accepts `SearchFilter{IDs, ProjectIDs, Names, NameContains, PathPrefix, IncludeDeleted, DependsOn}`; excludes deleted rows unless `IncludeDeleted` is true. Returns iterator sorted by `created_at ASC`. `DependsOn` filters to cells that depend on any of the provided IDs.
- `Update`: persists changes with optimistic lock; optionally accepts field whitelist like `project` store.
- `SoftDelete`: sets `deleted_at` (current clock time passed in by service) and bumps `UpdatedAt`.
- `ReplaceDependencies`: transactional delete-and-insert for outbound edges for one cell, scoped to project; idempotent with unique constraint.
- `DB()`: exposes underlying `*gorm.DB` for migrations/tests, same as `project`.
- Auto-migration runs in `store.New()` to create `cells` table and indexes.

## Populator Integration
- `Populator` interface (in `pkg/cell`): 
```go
type Populator interface {
    Name() string // e.g., "graph/moon"
    Populate(ctx context.Context, projectID project.ID) ([]PopulatorCell, error)
}
type PopulatorCell struct {
    Name        string
    Description string
    WorkingPath string
    ExternalID  string   // stable ID from the populator source (e.g., moon node ID)
    Dependencies []string // names of dependent cells in the same project
}
```
- Graph adapter: wrap `graph.Builder` to implement `Populator` by translating `core.Cell` values (`ID` -> `Name`, `Path` -> `WorkingPath`, description from Moon when available). Builder construction is outside the interface; service consumers pass a configured populator (root path comes from the adapter).
- `SyncFromPopulator` workflow:
  - Validates project exists.
  - Runs `Populate` to get current cells for the project.
  - Within a transaction:
    - Upsert by `(project_id, populator, populator_id)` first; fall back to `(project_id, name)` for legacy/no external ID.
      - Existing active row: update `Description`, `WorkingPath`, `UpdatedAt`.
      - Existing soft-deleted row: undelete and update fields.
      - Missing row: create new cell with fresh KSUID.
    - Optionally mark rows not returned by the populator as deleted when `opts.PruneMissing` is true (default false to avoid accidental deletes).
    - Rebuild outbound dependencies for each cell reported by the populator (name-based resolution within the transaction):
      - Resolve dependency names to cell IDs (creating cells first ensures lookup succeeds).
      - Call `ReplaceDependencies` so edges match the populator payload exactly.
      - Skip self-dependencies; reject cross-project references.
    - When `opts.PruneMissing` is true, also remove dependencies pointing to cells that were pruned (by virtue of `ReplaceDependencies` only using present names).
  - Returns `SyncResult{Created, Updated, Restored, Deleted, Skipped, DependenciesUpdated}` counts and a slice of affected IDs for logging/metrics.
- Populator errors bubble up; service wraps with context containing populator name.

## Exposed Types & Helpers (`pkg/cell`)
- Re-export model/store/service aliases similar to `project/pkg/project`.
- `NewService`, `NewServiceFromDB` (constructs store + service, wiring project service for validation).
- `NewStore` for direct store use.
- `NewKSUIDGenerator` for overrides/testing.
- Iterator and error exports mirror `project`/`ticket` conventions.

## Testing Expectations
- Unit: validation of empty name/path, missing/unknown project, optimistic lock mapping, soft delete idempotency, undelete on sync.
- Store: unique constraint behavior with `deleted_at IS NULL`, search filters (including deleted), path prefix filter, dependency filter, foreign key enforcement, `SoftDelete` timestamp set, dependency replacement idempotency.
- Sync: populator happy path (create/update/restore), dependency rebuild, prune behavior, populator error propagation, counts in `SyncResult`.
- Integration: service backed by embedded Postgres (reuse `project/internal/testutil/pg_embedded.go` pattern) to verify FK and partial unique index semantics.

## Rollout Notes
- New table only; no backfill required.
- Service mirrors `project` error shapes and constructor defaults for consistency across modules.
- Populators are opt-in; existing code can start with manual `CreateCell` usage and add sync later without breaking API.
