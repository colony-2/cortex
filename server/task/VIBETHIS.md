# VibeThis Ticket Library

## Overview
The ticket library extends the workflow orchestration stack with a persistent ticket ledger that tracks user-facing work items, their lifecycle stage, and current execution state. Each ticket belongs to a `core.Cell`, is backed by PostgreSQL via GORM, emits base58 short IDs for URL-safe references, and exposes APIs for searching tickets with combined stage/state conditions while enumerating all stages ever used. Tickets link to one or more Temporal workflows that drive recipe execution against the ticket. A reserved sentinel stage marks completion, enabling downstream services to stop polling once a ticket is done.

## Architecture
- Go module located at `server/ticket`
- Builds on `server/core` types (`core.CellRef`, `core.CellIdentifier`) and shares API patterns with other `server/*` services
- Storage layer uses PostgreSQL with migrations written in Goose and exercised by `pg_embedded` during tests
- GORM provides ORM mapping, soft transactions, and eager loading of workflow links
- HTTP handlers integrate with `server/api` by registering under `/ticket` in its router
- Temporal workers (`server/recipe-worker`) can call the service to advance ticket stage or state after workflow completion
- Short IDs use a base58 generator (e.g., `github.com/mr-tron/base58`) so tickets can be referenced cleanly in URLs and external systems

### Packages
- `internal/model`: GORM models, sentinel values, and enumeration helpers hidden from downstream consumers
- `internal/store`: Data access layer wrapping GORM DB and migration helpers
- `internal/service`: Business logic for stage transitions, state validation, workflow lifecycle, and short-ID generation utilities
- `internal/testutil`: `pg_embedded` harness spins up isolated PostgreSQL for integration tests
- Root package `ticket`: exported façade that exposes the public service interface and wiring helpers
- `internal/store` registers `gorm.io/plugin/optimisticlock` to enforce version-based concurrency on updates

### Data Model
```go
const CompletedStage = "__completed__"

var TicketStateValues = []TicketState{
    TicketStateWaitingUser,
    TicketStateWaitingDependency,
    TicketStateWorking,
}

type Ticket struct {
    ID             string          // base58 identifier for URLs and lookups
    Version        optimisticlock.Version
    CellRef        core.CellRef
    Title          string
    Description    string
    Stage          string         // user-defined pipeline lane
    State          TicketState    // constrained enum: WaitingUser, WaitingDependency, Working
    Originator     TicketOriginator `gorm:"embedded;embeddedPrefix:originator_"`
    Workflows      []TicketWorkflow `gorm:"foreignKey:TicketID"`
    CreatedAt      time.Time
    UpdatedAt      time.Time
    CompletedAt    *time.Time
}

type TicketWorkflow struct {
    ID         string
    TicketID   string
    WorkflowID string
    RunID      string
    LinkedAt   time.Time
}

type WorkflowRef struct {
    WorkflowID string
    RunID      string
}

type TicketOriginator struct {
    Type       OriginatorType
    User       *OriginatorUser   `gorm:"embedded;embeddedPrefix:originator_user_"`
    Agent      *OriginatorAgent  `gorm:"embedded;embeddedPrefix:originator_agent_"`
}

type OriginatorType string

const (
    OriginatorTypeUser  OriginatorType = "user"
    OriginatorTypeAgent OriginatorType = "agent"
)

type OriginatorUser struct {
    Email string
}

type OriginatorAgent struct {
    Cell          core.CellRef
    WorkflowName  string
    ExecutionID   string
    NodePath      string
    NodeKey       string
}

type TicketStageHistory struct {
    ID             string
    TicketID       string
    FromStage      string
    ToStage        string
    StateSnapshot  TicketState
    ActorType      HistoryActorType
    ActorID        string
    Reason         string
    ChangedAt      time.Time
}

type ShortIDGenerator interface {
    NewID() (string, error) // returns URL-safe base58 identifier without ambiguous characters
}

// TicketState uses text storage; Stage remains a plain string.
```
- `Stage` is free-form text chosen by automations or user actions (e.g. `triage`, `review`, `blocked`). The reserved `CompletedStage` value means the ticket is done.
- `State` is the constrained execution status. Allowed values are `WaitingUser`, `WaitingDependency`, and `Working`. This separates domain stages from runtime execution and is exposed through `TicketStateValues`.
- `TicketStageHistory` snapshots each transition with actor metadata (`HistoryActorType` values for `User`, `Automation`, `System`) and the ticket's concurrent `State` for auditability.
- The workflow linkage model stores the Temporal `workflow_id` and `run_id` needed to resolve live status via `workflowcontrol` without duplicating execution details.
- `WorkflowRef` is the lightweight input used by services and APIs when linking a ticket to a Temporal workflow execution.
- `TicketOriginator` captures whether a ticket was opened by a user (`OriginatorUser`) or automation agent (`OriginatorAgent`) and persists the email or agent tuple (cell, workflow name, execution ID, node path/key).
- All identifiers are base58 (Bitcoin alphabet without ambiguous characters) stored as strings, enabling short, URL-friendly references; the generator abstraction allows swapping implementations in tests versus production.
- `Version` relies on `gorm.io/plugin/optimisticlock` to provide optimistic concurrency control without manual timestamp comparisons.

### Stage & State Semantics
- Stage transitions are idempotent. Moving to `CompletedStage` sets `CompletedAt` and can optionally unlink all active workflow mappings.
- State updates fail when attempting to set `WaitingDependency` if the ticket has no outstanding dependency metadata.
- Every stage change persists a `TicketStageHistory` row capturing the actor, previous stage, and state snapshot.
- `DistinctStages` queries recent history to populate filters without scanning the entire ticket table.
- `GetStates` simply exposes `TicketStateValues`, ensuring API clients and Temporal workers stay in sync with the allowed enum.
- Workflow links hold Temporal identifiers and rely on `workflowcontrol` to infer execution status during ticket orchestration; workers call `UpdateTicket` with the desired stage/state once a new workflow heartbeat is processed.
- Optimistic concurrency protects against lost updates: `UpdateTicket` requires the latest `Version` and the store relies on GORM's optimistic-lock plugin to reject stale writes.

### Query Filters
```go
type TicketSearchFilter struct {
    StageAny       []string
    StageNotIn     []string
    States         []TicketState
    Originators    []OriginatorType
    Cells          []core.CellRef
    UpdatedAfter   *time.Time
    UpdatedBefore  *time.Time
}
```
- `StageAny` matches tickets in any of the provided stages; `StageNotIn` excludes lanes (e.g. suppress completed work).
- `States` holds the constrained execution states (defaults to all if unspecified).
- `Originators` limits results to user- or agent-sourced tickets; defaults to both when empty.
- `Cells` allows slicing by cell ownership; timestamp bounds support incremental sync use cases.

### Storage & Migrations
- Goose migrations create `tickets`, `ticket_workflows`, and `ticket_stage_history` tables.
- `tickets` stores originator columns via embedded prefixes (`originator_type`, `originator_user_email`, agent metadata) alongside stage/state timestamps and a `version` bigint managed by the optimistic-lock plugin.
- `ticket_stage_history` captures every stage change with actor metadata, including actor type, reason, and previous stage for auditing. Entries store the ticket state snapshot alongside stage to power time-travel queries and "stages ever used" reports.
- Workflow linkage records store the Temporal `workflow_id`/`run_id` pair with a unique constraint to prevent duplicate links and a composite index `(ticket_id, workflow_id)` for quick lookups.
- Foreign keys enforce referential integrity (`ticket_workflows.ticket_id -> tickets.id`).
- Composite index `(stage, state, cell_ref)` supports stage-filtered queries per cell.
- Base58 IDs persist as `CHAR(26)` columns, balancing readability with index selectivity.

## Key Interfaces
```go
type TicketStore interface {
    WithTx(ctx context.Context, fn func(ctx context.Context, store TicketStore) error) error
    Create(ctx context.Context, ticket *model.Ticket) error
    Get(ctx context.Context, id string) (*model.Ticket, error)
    Search(ctx context.Context, filter TicketSearchFilter, opts ListOptions) ([]model.Ticket, error)
    DistinctStages(ctx context.Context, cell core.CellRef) ([]string, error)
    Update(ctx context.Context, ticket *model.Ticket, fields ...string) error
    AppendHistory(ctx context.Context, entry *model.TicketStageHistory) error
}

type WorkflowStore interface {
    Upsert(ctx context.Context, link *model.TicketWorkflow) error
    ListByTicket(ctx context.Context, ticketID string) ([]model.TicketWorkflow, error)
    DeleteByTicket(ctx context.Context, ticketID string) error
}

type TicketService interface {
    CreateTicket(ctx context.Context, input CreateTicketInput) (*model.Ticket, error)
    UpdateTicket(ctx context.Context, id string, patch UpdateTicketInput) (*model.Ticket, error)
    LinkWorkflow(ctx context.Context, id string, wf WorkflowRef) (*model.TicketWorkflow, error)
    GetStages(ctx context.Context, cell core.CellRef) ([]string, error)
    GetStates(ctx context.Context) ([]model.TicketState, error)
    SearchTickets(ctx context.Context, filter TicketSearchFilter, opts ListOptions) ([]model.Ticket, error)
}

type CreateTicketInput struct {
    Cell       core.CellRef
    Title      string
    Description string
    Stage      string
    State      TicketState
    Originator TicketOriginator
}

type UpdateTicketInput struct {
    ExpectedVersion optimisticlock.Version
    Stage            *string
    State            *TicketState
    CompletedAt      *time.Time
    Originator       *TicketOriginatorPatch
}

type TicketOriginatorPatch struct {
    Type OriginatorType
    User *OriginatorUser
    Agent *OriginatorAgent
}
```
- Interfaces mirror other services for easy injection and testing.
- `WithTx` centralises transactional handling, used by `UpdateTicket` and workflow linkage updates.
- `CreateTicketInput` accepts the originator payload so callers never touch embedded struct tags directly.
- `UpdateTicketInput.ExpectedVersion` powers optimistic concurrency; pointer fields allow partial updates while preserving existing data.
- `Search`/`SearchTickets` accept combined filters so API handlers can answer stage/state/originator queries without multiple endpoints.
- `WorkflowStore` persists Temporal workflow identifiers so the service can query live status through `workflowcontrol` without duplicating state.

## Usage Examples
```go
svc := service.NewTicketService(store, clock, logger)

// Create a ticket waiting on a user decision
created, err := svc.CreateTicket(ctx, service.CreateTicketInput{
    Cell:       cellRef,
    Title:      "Review dependency graph proposal",
    Stage:      "triage",
    State:      model.TicketStateWaitingUser,
    Originator: service.NewUserOriginator("designer@example.com"),
})
if err != nil {
    return err
}

// Link the Temporal workflow that will drive this ticket forward
wfLink, _ := svc.LinkWorkflow(ctx, created.ID, ticket.WorkflowRef{
    WorkflowID: run.WorkflowID,
    RunID:      run.RunID,
})

// Worker polls workflowcontrol and applies stage/state changes through UpdateTicket
desc, _ := workflowcontrol.Describe(ctx, wfClient, wfLink.WorkflowID, wfLink.RunID)
if workflowcontrol.IsRunning(desc) {
    patch := service.UpdateTicketInput{
        ExpectedVersion: created.Version,
        Stage:           service.StringPtr("analysis"),
        State:           service.StatePtr(model.TicketStateWorking),
    }
    created, _ = svc.UpdateTicket(ctx, created.ID, patch)
}
if workflowcontrol.IsSuccessful(desc) {
    patch := service.UpdateTicketInput{
        ExpectedVersion: created.Version,
        Stage:           service.StringPtr(model.CompletedStage),
        CompletedAt:     service.TimePtr(clock.Now()),
    }
    _, _ = svc.UpdateTicket(ctx, created.ID, patch)
}
```
- Services expose `UpdateTicket` alongside helpers (e.g., `WithTicket`) to bundle multi-step transactional updates when needed.
- Service layer helper constructors (e.g. `NewUserOriginator`, `NewAgentOriginator`, `StringPtr`) prevent callers from hand-crafting embedded structs or pointer helpers.
- HTTP handlers wrap these service calls; TypeScript clients in `web/shared` gain generated types from OpenAPI.

## Testing
- Unit tests validate stage transition rules, enforce `TicketStateValues`, and ensure workflow linking guards against duplicate bindings using an in-memory GORM DB.
- Integration tests run with `pg_embedded` to ensure migrations and data access logic behave against real PostgreSQL.
- API contract tests use `httptest` servers and OpenAPI golden files.
- Temporal worker smoke tests ensure recipes can move tickets between stages and states while querying `workflowcontrol` for execution status.

## Configuration
- `TICKET_DATABASE_DSN`: PostgreSQL connection string (default derived from platform secrets).
- `TICKET_COMPLETED_STAGE`: optional override for the sentinel completion stage; defaults to `__completed__`.
- `TICKET_MAX_OPEN_WORKFLOWS`: guardrail for concurrent workflow links per ticket (default 8).
- Module registers moon tasks:
  - `moon run server:ticket:lint`
  - `moon run server:ticket:test`
- Development helper: `make ticket-dev` starts `pg_embedded`, runs migrations, and serves the HTTP API.
