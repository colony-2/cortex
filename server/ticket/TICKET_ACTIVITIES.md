# VibeThis Ticket Activity & Workflow Extensions

_This document builds on `TICKETS.md`, adding workflow linkage, ticket activity logs, and search enhancements that rely on audit data._

## Overview
The activity extension records every significant ticket event (updates, workflow launches, user notes) and persists Temporal workflow associations. These features reuse the core ticket ID, stage/state, and actor model while introducing additional storage, interfaces, and APIs.

## Additional Data Model
```go
type WorkflowLinkID string
type WorkflowID string
type WorkflowRunID string
type ActivityID string
type ActivityType string

const (
    ActivityTypeUpdate        ActivityType = "update"
    ActivityTypeWorkflowLinked ActivityType = "workflow_linked"
    ActivityTypeWorkflowRun    ActivityType = "workflow_run"
    ActivityTypeNote           ActivityType = "note"
)

type WorkflowRef struct {
    WorkflowID WorkflowID
    RunID      WorkflowRunID
}

type WorkflowLink struct {
    ID        WorkflowLinkID `gorm:"primaryKey;type:char(26)"`
    TicketID  ticket.ID      `gorm:"type:char(26);index"`
    WorkflowID WorkflowID
    RunID      WorkflowRunID
    LinkedAt   time.Time
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type Activity struct {
    ID        ActivityID                        `gorm:"primaryKey;type:char(26)"`
    TicketID  ticket.ID                        `gorm:"type:char(26);index"`
    Type      ActivityType                     `gorm:"type:text"`
    Actor     ticket.Actor                     `gorm:"embedded;embeddedPrefix:actor_"`
    Payload   datatypes.JSONType[ActivityPayload] `gorm:"type:jsonb"`
    CreatedAt time.Time
    UpdatedAt time.Time
}

type UpdateSnapshot struct {
    Stage ticket.Stage `json:"stage"`
    State ticket.State `json:"state"`
}

type UpdatePayload struct {
    From  UpdateSnapshot         `json:"from"`
    To    UpdateSnapshot         `json:"to"`
    Notes string                 `json:"notes,omitempty"`
    Extra map[string]string      `json:"extra,omitempty"`
}

type WorkflowPayload struct {
    WorkflowID WorkflowID   `json:"workflow_id"`
    RunID      WorkflowRunID `json:"run_id"`
}

type NotePayload struct {
    Body string `json:"body"`
}

type ActivityPayload struct {
    Update   *UpdatePayload   `json:"update,omitempty"`
    Workflow *WorkflowPayload `json:"workflow,omitempty"`
    Note     *NotePayload     `json:"note,omitempty"`
}

type ActivityPayloadInput struct {
    Update   *UpdatePayload
    Workflow *WorkflowPayload
    Note     *NotePayload
}
```
- Workflow/activity IDs reuse the base58 scheme and scan/valuer contracts defined alongside tickets.
- `ActivityType` defaults cover ticket updates, workflow linkage/run events, and free-form notes.
- `UpdatePayload` mirrors the snapshots passed to `ticket.UpdateInput`, allowing consistent audit trails for stage/state transitions and user resets.
- `ActivityPayload` is a one-of envelope; only one pointer may be non-nil and the service enforces this invariant before persistence.
- `WorkflowRef` is exported from `pkg/ticket` so callers can pass typed workflow identifiers without hand-rolling string pairs.

## Extended Stage & State Semantics
- `UpdateTicket` emits an `ActivityTypeUpdate` entry with before/after snapshots whenever the ticket mutates.
- `SearchStages` can aggregate historical stages from update activities, enabling actor-scoped stage discovery over time.
- Stage resets appear as update payloads whose `To` snapshot reflects the earlier stage/state.

## Activity Log
- Activities are stored in `ticket_activities` with `(ticket_id, created_at)` indexes for chronological playback.
- Workflow events (`workflow_linked`, `workflow_run`) and user notes (`note`) share the same table for a unified timeline.
- Embedded actor columns mirror the core ticket actor, keeping authorship queryable.

## Storage & Migrations
- Additional tables: `ticket_workflows` (linkage metadata) and `ticket_activities` (activity log).
- Unique constraint on `(workflow_id, run_id)` prevents duplicate workflow links per ticket.
- Materialized views can project stage timelines (derived from `UpdatePayload`) when analytics require historical state.

## Extended Interfaces
```go
// internal/store/workflows
type WorkflowStore interface {
    Upsert(ctx context.Context, link *model.WorkflowLink) error
    ListByTicket(ctx context.Context, ticketID ticket.ID) ([]model.WorkflowLink, error)
    DeleteByTicket(ctx context.Context, ticketID ticket.ID) error
}

// internal/store/activities
type ActivityStore interface {
    Append(ctx context.Context, activity *model.Activity) error
    ListByTicket(ctx context.Context, ticketID ticket.ID, filter ActivityFilter) (ticket.Iterator[*model.Activity], error)
}

// pkg/ticket additions
type Service interface {
    LinkWorkflow(ctx context.Context, id ticket.ID, wf WorkflowRef) (*model.WorkflowLink, error)
    AppendActivity(ctx context.Context, id ticket.ID, input ActivityInput) (*model.Activity, error)
    ListActivities(ctx context.Context, id ticket.ID, filter ActivityFilter) (ticket.Iterator[*model.Activity], error)
}

type ActivityInput struct {
    Type    ActivityType
    Actor   ticket.Actor
    Payload ActivityPayloadInput
}

type ActivityFilter struct {
    Types []ActivityType
    Since *time.Time
    Until *time.Time
}
```
- `LinkWorkflow` upserts workflow associations and emits a workflow-linked activity.
- `AppendActivity` is a generic entry point for user notes, workflow signals, or update snapshots supplied by external systems.
- Activity queries reuse the generic iterator from the core service; callers must close iterators and handle `ticket.ErrIteratorDone` on exhaustion.

## Usage Example
```go
wfLink, _ := svc.LinkWorkflow(ctx, ticketID, ticket.WorkflowRef{
    WorkflowID: WorkflowID(run.WorkflowID),
    RunID:      WorkflowRunID(run.RunID),
})

_, _ = svc.AppendActivity(ctx, ticketID, ticket.ActivityInput{
    Type:  ActivityTypeWorkflowLinked,
    Actor: ticket.AutomationActor("recipe-worker", cellRef),
    Payload: ticket.ActivityPayloadInput{
        Workflow: &WorkflowPayload{WorkflowID: wfLink.WorkflowID, RunID: wfLink.RunID},
    },
})

actions, _ := svc.ListActivities(ctx, ticketID, ticket.ActivityFilter{Types: []ActivityType{ActivityTypeUpdate}})
defer actions.Close(ctx)
```
- The service validates payload one-of constraints and records the actor automatically.

## Testing
- Workflow linkage tests assert idempotent upserts and unique `(workflow_id, run_id)` constraints.
- Activity log tests cover `AppendActivity` for updates, workflow launches, and user notes with payload validation.
- Stage projection tests verify `SearchStages` can operate on activity history when requested.

## Configuration
- `TICKET_MAX_OPEN_WORKFLOWS`: limits concurrent workflow links per ticket.
- `TICKET_ACTIVITY_MAX_PAYLOAD_BYTES`: optional guardrail for activity payload size.
- Moon tasks extend base targets with workflow/activity tests (e.g. `moon run server:ticket:workflow-test`).
