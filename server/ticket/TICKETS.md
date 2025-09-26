# VibeThis Tickets

## Overview
The tickets specification defines the core storage and mutation semantics for work items owned by a `core.Cell`. Each ticket tracks its current `Stage`, execution `State`, and the `Actor` (user or automation) responsible for the work. Tickets use base58 short identifiers for URL-safe references, persist optimistic versions for concurrency control, and expose APIs for creating, updating, and searching by stage/state without coupling to the richer activity log.

## Architecture
- Go module located at `server/ticket`
- Depends on `server/core` for `core.CellName`
- `internal/model` declares typed aliases (`Stage`, `State`, `ID`, `Actor`)
- `internal/store/tickets` wraps GORM with optimistic locking and short-ID generation
- `internal/service` orchestrates create/update/search flows plus helper constructors (`NewUserActor`, `NewAgentActor`, `StagePtr`, `StatePtr`)
- `internal/testutil` supplies `pg_embedded` harnesses for integration tests

## Data Model
```go
type Stage string

type State string

type ID string

type EmailAddress string

const (
    StateWaitingUser       State = "waiting_user"
	StateWaitingDependency State = "waiting_dependency"
	StateWaitingCapacity State = "waiting_capacity"
    StateWorking           State = "working"
)

type ActorType string

const (
    ActorTypeUser  ActorType = "user"
    ActorTypeAgent ActorType = "agent"
)

type ActorUser struct {
    Email EmailAddress `json:"email"`
}


type ActorAgent struct {
    CellName         string `json:"cell"`
    WorkflowName string       `json:"workflow_name"`
    ExecutionID  string       `json:"execution_id"`			
	InvocationHash string       `json:"invocation_hash"`			
}

type Actor struct {
    Type ActorType
    User *ActorUser   `gorm:"embedded;embeddedPrefix:actor_user_"`
    Agent *ActorAgent `gorm:"embedded;embeddedPrefix:actor_agent_"`
}

type Ticket struct {
    ID           ID                        `gorm:"primaryKey;type:char(26)"`
    Version      optimisticlock.Version    `gorm:"column:version"`
    CellName     core.CellName
    Title        string
    Description  string
    Stage        Stage
    State        State
    Creator      Actor                     `gorm:"embedded;embeddedPrefix:creator_"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
    CompletedAt  *time.Time
    ValidFrom    time.Time                 `gorm:"primaryKey;type:timestamp"`
    ValidUntil   time.Time                 `gorm:"type:timestamp"`
    LastResetID  *model.TicketResetID      `gorm:"-"`
    LastResetAt  *time.Time                `gorm:"-"`
}

type ActorPatch struct {
    Type  ActorType
    User  *ActorUser
    Agent *ActorAgent
}

type ShortIDGenerator interface {
    NewID() (string, error)
}

const CompletedStage Stage = "__completed__"
```
- `Stage` is a dedicated alias chosen by automations or users (e.g. `triage`, `review`, `blocked`). The reserved `CompletedStage` value marks completion.
- `State` captures constrained execution status; the built-in values reflect whether the ticket waits on a user, dependency, capacity, or is actively being processed.
- `Actor` embeds either a user email or agent metadata and is stored inline for auditing.
- `ID` values are base58 strings persisted as `CHAR(26)` and generated via `ShortIDGenerator`.
- `Version` relies on `gorm.io/plugin/optimisticlock` to reject stale updates.

### Stage & State Semantics
- Moving to `CompletedStage` sets `CompletedAt` and enables downstream clean-up flows.
- `UpdateTicket` applies optimistic concurrency using the supplied `Version`.
- `SearchStages` surfaces distinct current stages filtered by cell, actor type, or timestamp windows.

## Query Filters
```go
type SearchFilter struct {
    StageAny      []Stage
    StageNotIn    []Stage
    States        []State
    Actors        []ActorType
    Cells         []core.CellName
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}
```
- `StageAny`/`StageNotIn` use typed `Stage` values and mirror ticket list semantics across services.
- `States` and `Actors` default to all when omitted.
- Timestamp bounds support incremental sync and analytics. Default to all when ommitted.

## Storage & Migrations
- Single `tickets` table stores actors via embedded prefixes (`creator_type`, `creator_user_email`, etc.) alongside stage/state timestamps and the optimistic `version` column.

## Key Interfaces
```go
// internal/store/tickets
type Store interface {
    WithTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error
    Create(ctx context.Context, ticket *model.Ticket) error
    Get(ctx context.Context, id model.ID) (*model.Ticket, error)
    Search(ctx context.Context, filter SearchFilter) (Iterator[*model.Ticket], error)
    SearchStages(ctx context.Context, filter SearchFilter) (Iterator[model.Stage], error)
    Update(ctx context.Context, ticket *model.Ticket, fields ...string) error
}

type Iterator[T any] interface {
    Next(ctx context.Context) (T, error)
    Close(ctx context.Context) error
}

// pkg/ticket
type Service interface {
    CreateTicket(ctx context.Context, input CreateInput) (*model.Ticket, error)
    UpdateTicket(ctx context.Context, id model.ID, patch UpdateInput) (*model.Ticket, error)
    SearchTickets(ctx context.Context, filter SearchFilter) (Iterator[*model.Ticket], error)
    SearchStages(ctx context.Context, filter SearchFilter) (Iterator[model.Stage], error)
    GetStates(ctx context.Context) ([]model.State, error)
}

type CreateInput struct {
    Cell       core.CellName
    Title      string
    Description string
    Stage      Stage
    State      State
    Actor      Actor
}

type UpdateInput struct {
    ExpectedVersion optimisticlock.Version
    Stage            *Stage
    State            *State
    CompletedAt      *time.Time
    Actor            *ActorPatch
}
```
- Stores/services live under `internal/store/tickets` and `pkg/ticket`; the exported service composes the internal store while exposing iterators for streaming results.
- `Iterator[T]` is a Go-idiomatic generic cursor; implementations may fetch rows lazily and must be closed by callers.
- The service exposes a sentinel `ErrIteratorDone` to signal graceful exhaustion of iterator streams.
- `CreateInput` and pointer-based `UpdateInput` prevent direct manipulation of embedded actor structs or raw stage/state values.
- `SearchStages` reuses `SearchFilter`, keeping filters consistent between ticket listings and aggregate queries.

## Usage Example
```go
svc := ticket.NewService(ticket.ServiceConfig{
    Store:  ticketStore,
    Clock:  clock,
    IDGen:  idGen,
})

created, err := svc.CreateTicket(ctx, ticket.CreateInput{
    Cell:  cellRef,
    Title: "Review dependency graph proposal",
    Stage: Stage("triage"),
    State: StateWaitingUser,
    Actor: ticket.NewUserActor("designer@example.com"),
})
if err != nil {
    return err
}

tickets, err := svc.SearchTickets(ctx, ticket.SearchFilter{StageAny: []Stage{Stage("triage")}})
if err != nil {
    return err
}
defer tickets.Close(ctx)
for {
    t, err := tickets.Next(ctx)
    if errors.Is(err, ticket.ErrIteratorDone) {
        break
    }
    if err != nil {
        return err
    }
    // process ticket t
}

patch := ticket.UpdateInput{
    ExpectedVersion: created.Version,
    Stage:           ticket.StagePtr(Stage("analysis")),
    State:           ticket.StatePtr(StateWorking),
}
updated, err := svc.UpdateTicket(ctx, created.ID, patch)
if err != nil {
    return err
}
```
- Helper constructors (`NewUserActor`, `StagePtr`, `StatePtr`) avoid hand-crafted embedded structs or pointer helpers.
- `UpdateTicket` returns a fresh version so callers can chain subsequent edits safely.

## Testing
- Unit tests cover stage/state validation, optimistic locking failures, and actor patch semantics.
- Integration tests (via `pg_embedded`) assert `CreateTicket`/`UpdateTicket` persist changes and indexes update correctly.
- Projection tests verify `SearchStages` honours filters (stage lists remain actor/cell scoped) and iterators enforce closure semantics.

## Configuration
- `TICKET_DATABASE_DSN`: PostgreSQL connection string.
- Moon tasks come from existing workspace go definitions. No project specific tasks needed.
