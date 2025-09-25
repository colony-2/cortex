# VibeThis Ticket Event Log & Workflow Extensions

_This document builds on `TICKETS.md`, introducing a structured event log for tickets, workflow context, and audit-driven state reconstruction._

## Overview
The ticket event log records every mutation, workflow update, and attached artifact as immutable events. Events are append-only, actor-attributed, and grouped by ticket, enabling precise reconstruction of current state and historical playback. Workflow lifecycle information now travels exclusively through workflow events; no separate workflow link table is required.

## Event Log Data Model
```go
type WorkflowID string
type WorkflowRunID string
type TicketEventID string
type TicketEventKind string
type TicketResetID string

type TicketFieldName string

type TicketFieldChange struct {
    Field TicketFieldName `json:"field"`
    From  string          `json:"from,omitempty"`
    To    string          `json:"to,omitempty"`
}

const (
    TicketEventKindTicket      TicketEventKind = "ticket"
    TicketEventKindWorkflow    TicketEventKind = "workflow"
    TicketEventKindMarkdownDoc TicketEventKind = "markdown_doc"
    TicketEventKindChangeSet   TicketEventKind = "changeset"
)

type WorkflowRef struct {
    WorkflowID WorkflowID
    RunID      WorkflowRunID
}

type TicketEventPayloadType string

const (
    TicketEventPayloadTypeTicket      TicketEventPayloadType = "ticket"
    TicketEventPayloadTypeWorkflow    TicketEventPayloadType = "workflow"
    TicketEventPayloadTypeMarkdownDoc TicketEventPayloadType = "markdown_doc"
    TicketEventPayloadTypeChangeSet   TicketEventPayloadType = "changeset"
)

type TicketFieldChangeList []TicketFieldChange

type TicketEvent struct {
    ID          TicketEventID           `gorm:"primaryKey;type:char(26)"`
    TicketID    ticket.ID               `gorm:"type:char(26);index"`
    Kind        TicketEventKind         `gorm:"type:text"`
    PayloadType TicketEventPayloadType  `gorm:"column:payload_type;type:text;index"`
    Actor       ticket.Actor            `gorm:"embedded;embeddedPrefix:actor_"`
    EventTime   time.Time               `gorm:"index"`
    CreatedAt   time.Time
    ResetID     *TicketResetID          `gorm:"type:char(26);index"`

    TicketData    TicketEventPayload      `gorm:"embedded;embeddedPrefix:ticket_"`
    TicketChanges TicketFieldChangeList   `gorm:"column:ticket_changes;type:jsonb"`
    WorkflowData  WorkflowEventPayload    `gorm:"embedded;embeddedPrefix:workflow_"`
    MarkdownData  MarkdownDocEventPayload `gorm:"embedded;embeddedPrefix:markdown_"`
    ChangeSetData ChangeSetEventPayload   `gorm:"embedded;embeddedPrefix:changeset_"`

    Payload TicketEventBody `gorm:"-"`
}

type TicketEventBody struct {
    Ticket      *TicketEventPayload      `json:"ticket,omitempty"`
    Workflow    *WorkflowEventPayload    `json:"workflow,omitempty"`
    MarkdownDoc *MarkdownDocEventPayload `json:"markdown_doc,omitempty"`
    ChangeSet   *ChangeSetEventPayload   `json:"changeset,omitempty"`
}

type TicketEventPayload struct {
    Changes []TicketFieldChange `json:"changes" gorm:"-"`
    Notes   string              `json:"notes,omitempty"`
}

type WorkflowEventType string

const (
    WorkflowEventRunning   WorkflowEventType = "running"
    WorkflowEventCompleted WorkflowEventType = "completed"
    WorkflowEventFailed    WorkflowEventType = "failed"
)

type WorkflowEventPayload struct {
    Type       WorkflowEventType `json:"type"`
    WorkflowID WorkflowID        `json:"workflow_id"`
    RunID      WorkflowRunID     `json:"run_id"`
}

type MarkdownDocEventType string

const (
    MarkdownDocAttached   MarkdownDocEventType = "attached"
    MarkdownDocOverridden MarkdownDocEventType = "overridden"
    MarkdownDocRemoved    MarkdownDocEventType = "removed"
)

type MarkdownDocEventPayload struct {
    Type MarkdownDocEventType `json:"type"`
    Name string               `json:"name"`
    Path string               `json:"path"`
}

type ChangeSetEventType string

const (
    ChangeSetAttached   ChangeSetEventType = "attached"
    ChangeSetOverridden ChangeSetEventType = "overridden"
    ChangeSetRemoved    ChangeSetEventType = "removed"
)

type ChangeSetEventPayload struct {
    Type            ChangeSetEventType `json:"type"`
    Path            string             `json:"path"`
    CommitMessage   string             `json:"commit_message"`
    BaseGitHash     string             `json:"base_git_hash"`
    ParentGitHash   string             `json:"parent_git_hash"`
    TipGitHash      string             `json:"tip_git_hash"`
}

type TicketResetEventPayload struct {
    ResetID       TicketResetID   `json:"reset_id"`
    AnchorEventID *TicketEventID  `json:"anchor_event_id,omitempty"`
    Reason        string          `json:"reason,omitempty"`
}

type TicketReset struct {
    ID        TicketResetID `gorm:"primaryKey;type:char(26)"`
    TicketID  ticket.ID     `gorm:"type:char(26);index"`
    Actor     ticket.Actor  `gorm:"embedded;embeddedPrefix:actor_"`
    Reason    string        `gorm:"type:text"`
    CreatedAt time.Time
}
```
- Ticket event IDs reuse the base58 scheme and scan/valuer contracts defined alongside tickets.
- `TicketEventBody` remains a one-of envelope enforced by the service layer; exactly one payload pointer is materialised at query time based on `PayloadType`.
- `TicketEventPayloadType` is a discriminator column written alongside the event record, making it trivial to determine which embedded payload columns (`ticket_*`, `workflow_*`, etc.) hold data.
- Ticket field mutations are expressed as `field -> from/to` changes. Consumers fold these deltas to compute current values.
- `EventTime` captures the effective time of the event; `CreatedAt` records persistence time in storage.
- Reset-aware columns make it possible to mark ranges of events as superseded without deleting them. The event records remain immutable, with `ResetID` referencing the corresponding `TicketReset` entry when present.

## Event Semantics
- **Ticket events** capture stage/state transitions, field edits, or other direct ticket mutations. The change list calls out granular field-level deltas and optional notes for audit context.
- **Workflow events** represent lifecycle updates for Temporal workflows associated with the ticket. `running` is emitted when the workflow connection is established, replacing the prior separate "linked" concept. Additional workflow updates (completed/failed) append new events with the same workflow identifiers.
- **Markdown document events** track artifact attachment to the ticket. Override and removal events reference the relevant display `name` and filesystem `path` so consumers can determine the active document.
- **Change set events** mirror markdown semantics but point to a persisted change set path. Git hashes allow reconstructing before/after context without extra lookups.
- **Reset events** (`ticket_reset`) summarise rewind operations. The payload carries the `TicketReset` identifier, anchor event (if any), and contextual reason so consumers can render the action inline with the timeline.
- Events are never deleted or mutated. Current ticket state or artifact attachments are derived by replaying the log and ignoring entries marked with a non-null `ResetID`.

## Event Log Storage
- Events live in `ticket_events` with `(ticket_id, event_time, id)` indexes for chronological playback.
- `ticket_resets` records intentional rewinds. Applying a reset is transactional: the reset row and all referenced event updates (setting `ResetID`) occur within the same database transaction.
- Workflow lifecycle is fully encoded in workflow events. Consumers rely on the event log to discover active workflows, recent runs, and terminal states.
- Materialized views or projections can compose current state by folding ticket, markdown, and change set events while ignoring reset-tagged records.

## Extended Interfaces
```go
// internal/store/events
type EventStore interface {
    Append(ctx context.Context, event *model.TicketEvent) error
    AppendBatch(ctx context.Context, ticketID ticket.ID, events []*model.TicketEvent) error
    ListByTicket(ctx context.Context, ticketID ticket.ID, filter TicketEventFilter) (ticket.Iterator[*model.TicketEvent], error)
    MarkReset(ctx context.Context, ticketID ticket.ID, reset *model.TicketReset, eventIDs []TicketEventID) error
}

// pkg/ticket additions
type Service interface {
    AppendWorkflowEvent(ctx context.Context, id ticket.ID, input WorkflowEventInput) (*model.TicketEvent, error)
    AppendMarkdownEvent(ctx context.Context, id ticket.ID, input MarkdownEventInput) (*model.TicketEvent, error)
    AppendChangeSetEvent(ctx context.Context, id ticket.ID, input ChangeSetEventInput) (*model.TicketEvent, error)
    ListEvents(ctx context.Context, id ticket.ID, filter TicketEventFilter) (ticket.Iterator[*model.TicketEvent], error)
    ResetTicket(ctx context.Context, id ticket.ID, input TicketResetInput) (*model.TicketReset, error)
}

type WorkflowEventInput struct {
    Actor     ticket.Actor
    Payload   ticket.WorkflowEventPayload
    EventTime time.Time
}

type MarkdownEventInput struct {
    Actor     ticket.Actor
    Payload   ticket.MarkdownDocEventPayload
    EventTime time.Time
}

type ChangeSetEventInput struct {
    Actor     ticket.Actor
    Payload   ticket.ChangeSetEventPayload
    EventTime time.Time
}

type TicketEventFilter struct {
    Kinds        []TicketEventKind
    PayloadTypes []TicketEventPayloadType
    Types        []string
    Since        *time.Time
    Until        *time.Time
    At           *time.Time
    IncludeReset bool
}

type TicketResetInput struct {
    Actor          ticket.Actor
    Reason         string
    LastValidEvent *TicketEventID
}
```
- The append helpers validate actors and payloads, stamp missing `EventTime` values with `time.Now()`, and persist the event alongside its `PayloadType`.
- `ResetTicket` coordinates ticket rewinds in one transaction: it inserts the reset record, closes the active ticket slice, reconstructs the anchor slice, emits a `ticket_reset` event, and assigns `ResetID` to superseded events. When `LastValidEvent` is set, all later events without a `ResetID` are reset; when nil, all active events are rewound.
- `TicketEventFilter.At` enables point-in-time projections: queries return events with `event_time <= At` where any associated reset occurs after `At`, ensuring the event stream lines up with temporal ticket slices.
- Event iterators behave like the prior activity iterators; callers must close them and treat `ticket.ErrIteratorDone` as a normal termination signal.
- `TicketEventFilter.PayloadTypes` filters on the discriminator column, while `TicketEventFilter.Types` continues to match payload-specific subtype values (e.g. workflow status or artifact lifecycle transitions).

## Usage Example
```go
_, _ = svc.AppendMarkdownEvent(ctx, ticketID, ticket.MarkdownEventInput{
    Actor: ticket.AutomationActor("docs-sync", cellRef),
    EventTime: time.Now(),
    Payload: ticket.MarkdownDocEventPayload{
        Type: ticket.MarkdownDocAttached,
        Name: "design",
        Path: "docs/design.md",
    },
})

_, _ = svc.AppendWorkflowEvent(ctx, ticketID, ticket.WorkflowEventInput{
    Actor: ticket.AutomationActor("recipe-worker", cellRef),
    EventTime: time.Now(),
    Payload: ticket.WorkflowEventPayload{
        Type:       ticket.WorkflowEventRunning,
        WorkflowID: ticket.WorkflowID(run.WorkflowID),
        RunID:      ticket.WorkflowRunID(run.RunID),
    },
})

iter, _ := svc.ListEvents(ctx, ticketID, ticket.TicketEventFilter{Kinds: []ticket.TicketEventKind{ticket.TicketEventKindMarkdownDoc}})
defer iter.Close(ctx)
```
- Event consumers compute the active markdown attachment by walking the iterator and applying the latest non-reset `attached/overridden/removed` sequence per `name`/`path` pair.

## Testing
- Unit coverage extends to the store (append/query) and service layers, validating iterator semantics and payload validation.
- Integration tests simulate real-world workflows: creating tickets, appending various event types, listing events, and performing resets. Tests verify correct state reconstruction and reset behavior. Should use pg_embedded to complete.
