# 03 - Ticket Detail Service

**Cell**: `server/ticket`
**Dependencies**: 01, 02
**Purpose**: Service layer already has event retrieval methods - document usage for ticket detail feature

---

## Overview

The `server/ticket` service already implements event retrieval via the `ListEvents` method. This spec documents how to use the existing service methods for the ticket detail feature. No code changes are required in this cell.

---

## Existing Service Methods

The ticket service at `/src/server/ticket/internal/service/service.go` already provides the necessary methods:

### 1. GetTicket

Retrieves a single ticket by ID:

```go
func (s *Service) GetTicket(ctx context.Context, projectID, ticketID string) (*model.Ticket, error)
```

**Usage for ticket detail**:
- Fetch full ticket information including all fields
- Used as the primary data for the detail view header

### 2. ListEvents

Retrieves events associated with a ticket:

```go
func (s *Service) ListEvents(ctx context.Context, filter *model.EventFilter) ([]*model.TicketEvent, error)
```

**Filter Options** (from `/src/server/ticket/internal/model/filter.go`):
```go
type EventFilter struct {
    ProjectIDs []string           // Filter by projects
    TicketIDs  []string           // Filter by tickets
    Kinds      []EventKind        // Filter by event types
    Since      *time.Time         // Events after this time
    Until      *time.Time         // Events before this time
    At         *time.Time         // Point-in-time snapshot
    IncludeReset bool             // Include invalidated events
}
```

**Usage for ticket detail**:
```go
// Get all events for a ticket
events, err := service.ListEvents(ctx, &model.EventFilter{
    ProjectIDs: []string{projectID},
    TicketIDs:  []string{ticketID},
    IncludeReset: includeReset, // from query param
    Kinds: kinds,                // from query param (if specified)
    Since: since,                // from query param (if specified)
    Until: until,                // from query param (if specified)
})
```

---

## Event Model Structure

The service returns `model.TicketEvent` structs (from `/src/server/ticket/internal/model/events.go`):

```go
type TicketEvent struct {
    ID        string
    TicketID  string
    ProjectID string
    Kind      EventKind
    Timestamp time.Time
    Actor     Actor
    ResetID   *string
    Payload   EventPayload // Interface - use type assertion
}

type EventKind string
const (
    EventKindTicket      EventKind = "ticket"
    EventKindWorkflow    EventKind = "workflow"
    EventKindMarkdownDoc EventKind = "markdown_doc"
    EventKindChangeSet   EventKind = "changeset"
    EventKindReset       EventKind = "reset"
)
```

### Event Payloads

Each event has a payload that must be type-asserted based on `Kind`:

```go
switch event.Kind {
case model.EventKindWorkflow:
    payload := event.Payload.(*model.WorkflowEventPayload)
    // payload.Type, payload.WorkflowID, payload.RunID

case model.EventKindTicket:
    payload := event.Payload.(*model.TicketEventPayload)
    // payload.Stage, payload.State, payload.Description, payload.Notes

case model.EventKindMarkdownDoc:
    payload := event.Payload.(*model.MarkdownEventPayload)
    // payload.Type, payload.Name, payload.Path

case model.EventKindChangeSet:
    payload := event.Payload.(*model.ChangeSetEventPayload)
    // payload.Type, payload.Path, payload.CommitMessage, payload.BaseHash, etc.

case model.EventKindReset:
    payload := event.Payload.(*model.ResetEventPayload)
    // payload.ResetID, payload.AnchorEventID, payload.Reason
}
```

---

## Integration with API Handlers

The API handler (spec 04) will:

1. **Parse request parameters** (projectId, ticketId, query params)
2. **Call service methods**:
   ```go
   ticket, err := ticketService.GetTicket(ctx, projectID, ticketID)
   events, err := ticketService.ListEvents(ctx, filter)
   ```
3. **Convert to OpenAPI types** (domain models → OpenAPI-generated structs)
4. **Return JSON response**

---

## Data Flow

```
API Handler
    ↓
Service.GetTicket(projectID, ticketID)
    ↓
Store.GetTicket → GORM query on tickets table
    ↓
Return model.Ticket
```

```
API Handler
    ↓
Service.ListEvents(filter)
    ↓
Store.ListEvents → GORM query on ticket_events table with filters
    ↓
Return []model.TicketEvent (with typed payloads)
```

---

## Example Usage Pattern

```go
// In API handler
func (h *Handler) handleListTicketEvents(
    ctx context.Context,
    projectID, ticketID string,
    params ListTicketEventsParams,
) ([]openapi.TicketEvent, error) {
    // Build filter from query params
    filter := &model.EventFilter{
        ProjectIDs: []string{projectID},
        TicketIDs:  []string{ticketID},
    }

    if params.Kind != nil {
        filter.Kinds = []model.EventKind{convertKind(*params.Kind)}
    }
    if params.Since != nil {
        filter.Since = params.Since
    }
    if params.Until != nil {
        filter.Until = params.Until
    }
    if params.IncludeReset != nil {
        filter.IncludeReset = *params.IncludeReset
    }

    // Call service
    events, err := h.ticketService.ListEvents(ctx, filter)
    if err != nil {
        return nil, err
    }

    // Convert to OpenAPI types
    return convertToOpenAPIEvents(events), nil
}
```

---

## Event Payload Conversion

The API handler will need helper functions to convert domain event payloads to OpenAPI types:

```go
func convertToOpenAPIEvent(e *model.TicketEvent) openapi.TicketEvent {
    event := openapi.TicketEvent{
        Id:        e.ID,
        TicketId:  e.TicketID,
        ProjectId: e.ProjectID,
        Kind:      convertKind(e.Kind),
        Timestamp: e.Timestamp,
        Actor:     convertActor(e.Actor),
        ResetId:   e.ResetID,
    }

    // Set appropriate payload based on kind
    switch e.Kind {
    case model.EventKindWorkflow:
        payload := e.Payload.(*model.WorkflowEventPayload)
        event.WorkflowPayload = &openapi.WorkflowEventPayload{
            Type:       convertWorkflowType(payload.Type),
            WorkflowId: payload.WorkflowID,
            RunId:      payload.RunID,
        }
    case model.EventKindTicket:
        payload := e.Payload.(*model.TicketEventPayload)
        event.TicketPayload = &openapi.TicketEventPayload{
            Stage:       payload.Stage,
            State:       convertTicketState(payload.State),
            Description: payload.Description,
            CompletedAt: payload.CompletedAt,
            Notes:       payload.Notes,
        }
    // ... other cases
    }

    return event
}
```

---

## Testing

The service methods already have tests. Verify they work for ticket detail use case:

```bash
cd /src/server/ticket
go test ./internal/service/... -v
```

**Test scenarios to verify**:
- [ ] GetTicket returns full ticket data
- [ ] ListEvents returns events for specific ticket
- [ ] Filter by event kind works
- [ ] Filter by time range works
- [ ] IncludeReset flag works
- [ ] Events have correct payload types

---

## Implementation Notes

### No Code Changes Required

The `server/ticket` cell already has all necessary service methods. This spec documents:
- How existing methods will be used for ticket detail
- Expected data flow
- Integration points with API handlers

### Event Ordering

Events are returned in chronological order by default (oldest first). The UI may want to reverse this for display (newest first).

### Performance Considerations

- For tickets with many events (100+), consider pagination in future
- Current implementation loads all events in memory
- Database has indexes on ticket_id and timestamp for efficient queries

---

## Next Steps

After reviewing this spec:
1. Proceed to **04-ticket-detail-api-handlers.md** to implement HTTP handlers that use these service methods
2. Handlers will call `GetTicket` and `ListEvents`, then convert results to OpenAPI types

---

## Success Criteria

- [ ] Service methods documented and understood
- [ ] Event payload conversion pattern defined
- [ ] Integration approach with handlers clarified
- [ ] Existing tests verified to pass

---

## Estimated Duration

**~30 minutes** - Documentation review and understanding (no code changes)
