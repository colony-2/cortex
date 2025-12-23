# 04 - Ticket Detail API Handlers

**Cell**: `server/api`
**Dependencies**: 07, 08, 09
**Purpose**: Implement HTTP handler for ticket events endpoint

---

## Overview

Implement the HTTP handler for the `GET /api/projects/{projectId}/tickets/{ticketId}/events` endpoint. This handler retrieves events associated with a ticket and returns them in the OpenAPI-defined format.

---

## Changes Required

### 1. Add Handler Method

Add the `ListTicketEvents` handler method to implement the `ServerInterface` in `/src/server/api/internal/handlers/api.go`:

```go
func (a *API) ListTicketEvents(
    w http.ResponseWriter,
    r *http.Request,
    projectId string,
    ticketId string,
    params openapi.ListTicketEventsParams,
) {
    ctx := r.Context()

    // Verify ticket exists and belongs to project
    ticket, err := a.ticketService.GetTicket(ctx, projectId, ticketId)
    if err != nil {
        if errors.Is(err, service.ErrTicketNotFound) {
            http.Error(w, "Ticket not found", http.StatusNotFound)
            return
        }
        a.logger.Error("failed to get ticket", "error", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Build event filter from query parameters
    filter := &model.EventFilter{
        ProjectIDs: []string{projectId},
        TicketIDs:  []string{ticketId},
    }

    // Apply optional filters
    if params.Kind != nil {
        filter.Kinds = []model.EventKind{convertEventKind(*params.Kind)}
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

    // Retrieve events
    events, err := a.ticketService.ListEvents(ctx, filter)
    if err != nil {
        a.logger.Error("failed to list ticket events", "error", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Convert to OpenAPI types
    apiEvents := make([]openapi.TicketEvent, len(events))
    for i, event := range events {
        apiEvents[i] = convertEventToAPI(event)
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(apiEvents); err != nil {
        a.logger.Error("failed to encode events", "error", err)
    }
}
```

### 2. Add Conversion Helper Functions

Add helper functions to convert domain models to OpenAPI types in a new file `/src/server/api/internal/handlers/ticket_events.go`:

```go
package handlers

import (
    "github.com/yourusername/colony2/server/openapi/pkg/openapi"
    "github.com/yourusername/colony2/server/ticket/internal/model"
)

// convertEventToAPI converts a domain TicketEvent to OpenAPI TicketEvent
func convertEventToAPI(e *model.TicketEvent) openapi.TicketEvent {
    event := openapi.TicketEvent{
        Id:        e.ID,
        TicketId:  e.TicketID,
        ProjectId: e.ProjectID,
        Kind:      convertEventKind(e.Kind),
        Timestamp: e.Timestamp,
        Actor:     convertActorToAPI(e.Actor),
        ResetId:   e.ResetID,
    }

    // Set appropriate payload based on kind
    switch e.Kind {
    case model.EventKindWorkflow:
        payload := e.Payload.(*model.WorkflowEventPayload)
        event.WorkflowPayload = &openapi.WorkflowEventPayload{
            Type:       convertWorkflowEventType(payload.Type),
            WorkflowId: payload.WorkflowID,
            RunId:      payload.RunID,
        }

    case model.EventKindTicket:
        payload := e.Payload.(*model.TicketEventPayload)
        event.TicketPayload = &openapi.TicketEventPayload{
            Stage:       payload.Stage,
            State:       convertTicketStateToAPI(payload.State),
            Description: payload.Description,
            CompletedAt: payload.CompletedAt,
            Notes:       payload.Notes,
        }

    case model.EventKindMarkdownDoc:
        payload := e.Payload.(*model.MarkdownEventPayload)
        event.MarkdownPayload = &openapi.MarkdownEventPayload{
            Type: convertMarkdownEventType(payload.Type),
            Name: payload.Name,
            Path: payload.Path,
        }

    case model.EventKindChangeSet:
        payload := e.Payload.(*model.ChangeSetEventPayload)
        event.ChangesetPayload = &openapi.ChangeSetEventPayload{
            Type:          convertChangeSetEventType(payload.Type),
            Path:          payload.Path,
            CommitMessage: payload.CommitMessage,
            BaseHash:      payload.BaseHash,
            ParentHash:    payload.ParentHash,
            TipHash:       payload.TipHash,
        }

    case model.EventKindReset:
        payload := e.Payload.(*model.ResetEventPayload)
        event.ResetPayload = &openapi.ResetEventPayload{
            ResetId:       payload.ResetID,
            AnchorEventId: payload.AnchorEventID,
            Reason:        payload.Reason,
        }
    }

    return event
}

// convertEventKind converts domain EventKind to OpenAPI TicketEventKind
func convertEventKind(kind model.EventKind) openapi.TicketEventKind {
    switch kind {
    case model.EventKindTicket:
        return openapi.TicketEventKindTicket
    case model.EventKindWorkflow:
        return openapi.TicketEventKindWorkflow
    case model.EventKindMarkdownDoc:
        return openapi.TicketEventKindMarkdownDoc
    case model.EventKindChangeSet:
        return openapi.TicketEventKindChangeset
    case model.EventKindReset:
        return openapi.TicketEventKindReset
    default:
        return openapi.TicketEventKindTicket // fallback
    }
}

// convertWorkflowEventType converts domain to OpenAPI workflow event type
func convertWorkflowEventType(t model.WorkflowEventType) openapi.WorkflowEventType {
    switch t {
    case model.WorkflowEventTypeRunning:
        return openapi.WorkflowEventTypeRunning
    case model.WorkflowEventTypeCompleted:
        return openapi.WorkflowEventTypeCompleted
    case model.WorkflowEventTypeFailed:
        return openapi.WorkflowEventTypeFailed
    default:
        return openapi.WorkflowEventTypeRunning // fallback
    }
}

// convertMarkdownEventType converts domain to OpenAPI markdown event type
func convertMarkdownEventType(t model.MarkdownEventType) openapi.MarkdownEventType {
    switch t {
    case model.MarkdownEventTypeAttached:
        return openapi.MarkdownEventTypeAttached
    case model.MarkdownEventTypeOverridden:
        return openapi.MarkdownEventTypeOverridden
    case model.MarkdownEventTypeRemoved:
        return openapi.MarkdownEventTypeRemoved
    default:
        return openapi.MarkdownEventTypeAttached // fallback
    }
}

// convertChangeSetEventType converts domain to OpenAPI changeset event type
func convertChangeSetEventType(t model.ChangeSetEventType) openapi.ChangeSetEventType {
    switch t {
    case model.ChangeSetEventTypeAttached:
        return openapi.ChangeSetEventTypeAttached
    case model.ChangeSetEventTypeOverridden:
        return openapi.ChangeSetEventTypeOverridden
    case model.ChangeSetEventTypeRemoved:
        return openapi.ChangeSetEventTypeRemoved
    default:
        return openapi.ChangeSetEventTypeAttached // fallback
    }
}

// convertTicketStateToAPI converts domain TicketState to OpenAPI (if needed)
// Note: May already exist in ticket handlers
func convertTicketStateToAPI(state *model.TicketState) *openapi.TicketState {
    if state == nil {
        return nil
    }
    apiState := openapi.TicketState(*state)
    return &apiState
}

// convertActorToAPI converts domain Actor to OpenAPI Actor
// Note: This likely already exists in ticket handlers - reuse if available
func convertActorToAPI(actor model.Actor) openapi.Actor {
    // Implementation depends on existing actor conversion
    // Should already exist from ticket creation/update handlers
    // Example:
    apiActor := openapi.Actor{
        Type: openapi.ActorType(actor.Type),
    }

    switch actor.Type {
    case model.ActorTypeUser:
        apiActor.User = &openapi.ActorUser{
            Email: actor.User.Email,
        }
    case model.ActorTypeAgent:
        apiActor.Agent = &openapi.ActorAgent{
            Cell:            actor.Agent.Cell,
            WorkflowName:    actor.Agent.WorkflowName,
            ExecutionId:     actor.Agent.ExecutionID,
            InvocationHash:  actor.Agent.InvocationHash,
        }
    }

    return apiActor
}
```

### 3. Register Handler Route

The route should already be registered automatically by the OpenAPI-generated server wrapper. Verify in `/src/server/api/internal/handlers/server.go` or wherever routes are registered:

```go
// This is typically auto-generated or uses the OpenAPI-generated wrapper
router.HandleFunc("/api/projects/{projectId}/tickets/{ticketId}/events",
    apiHandler.ListTicketEvents)
```

---

## File Locations

- **Main handler**: `/src/server/api/internal/handlers/api.go` (add `ListTicketEvents` method)
- **Conversion helpers**: `/src/server/api/internal/handlers/ticket_events.go` (new file)
- **Route registration**: Likely auto-handled by OpenAPI server wrapper

---

## Testing

### Unit Tests

Create `/src/server/api/internal/handlers/ticket_events_test.go`:

```go
package handlers_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/yourusername/colony2/server/api/internal/handlers"
    "github.com/yourusername/colony2/server/openapi/pkg/openapi"
    "github.com/yourusername/colony2/server/ticket/internal/model"
)

func TestListTicketEvents(t *testing.T) {
    // Setup
    mockTicketService := new(MockTicketService)
    api := &handlers.API{
        ticketService: mockTicketService,
        logger:        testLogger,
    }

    projectID := "test-project"
    ticketID := "test-ticket-123"

    // Mock ticket exists
    mockTicketService.On("GetTicket", mock.Anything, projectID, ticketID).
        Return(&model.Ticket{ID: ticketID}, nil)

    // Mock events
    now := time.Now()
    mockEvents := []*model.TicketEvent{
        {
            ID:        "event-1",
            TicketID:  ticketID,
            ProjectID: projectID,
            Kind:      model.EventKindWorkflow,
            Timestamp: now,
            Actor:     model.Actor{Type: model.ActorTypeUser},
            Payload: &model.WorkflowEventPayload{
                Type:       model.WorkflowEventTypeRunning,
                WorkflowID: "wf-123",
                RunID:      "run-456",
            },
        },
    }
    mockTicketService.On("ListEvents", mock.Anything, mock.Anything).
        Return(mockEvents, nil)

    // Execute
    req := httptest.NewRequest("GET", "/api/projects/"+projectID+"/tickets/"+ticketID+"/events", nil)
    w := httptest.NewRecorder()

    api.ListTicketEvents(w, req, projectID, ticketID, openapi.ListTicketEventsParams{})

    // Assert
    assert.Equal(t, http.StatusOK, w.Code)

    var events []openapi.TicketEvent
    err := json.NewDecoder(w.Body).Decode(&events)
    assert.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "event-1", events[0].Id)
    assert.Equal(t, openapi.TicketEventKindWorkflow, events[0].Kind)
}

func TestListTicketEvents_TicketNotFound(t *testing.T) {
    mockTicketService := new(MockTicketService)
    api := &handlers.API{
        ticketService: mockTicketService,
        logger:        testLogger,
    }

    mockTicketService.On("GetTicket", mock.Anything, "proj", "nonexistent").
        Return(nil, service.ErrTicketNotFound)

    req := httptest.NewRequest("GET", "/api/projects/proj/tickets/nonexistent/events", nil)
    w := httptest.NewRecorder()

    api.ListTicketEvents(w, req, "proj", "nonexistent", openapi.ListTicketEventsParams{})

    assert.Equal(t, http.StatusNotFound, w.Code)
}
```

### Integration Tests

Test with real database:

```bash
cd /src/server/api
go test ./internal/handlers/... -v -tags=integration
```

**Test scenarios**:
- [ ] Successfully retrieve events for existing ticket
- [ ] Return 404 for non-existent ticket
- [ ] Filter by event kind works
- [ ] Filter by time range works
- [ ] IncludeReset parameter works
- [ ] Empty event list returns empty array (not null)

---

## Implementation Notes

### Error Handling

- **404**: Ticket not found → `http.StatusNotFound`
- **500**: Database errors → `http.StatusInternalServerError`
- Log errors with context for debugging

### Response Format

- Always return valid JSON array (even if empty)
- Use `application/json` content type
- Pretty-print not required (saves bandwidth)

### Actor Conversion

The `convertActorToAPI` function likely already exists from ticket creation/update handlers. Reuse it instead of duplicating.

### Performance

- No pagination yet (all events loaded)
- For tickets with 1000+ events, consider adding pagination in future
- Database indexes on `ticket_id` and `timestamp` ensure fast queries

---

## Next Steps

After completing this spec:
1. Proceed to **05-ticket-detail-web-bindings.md** to regenerate frontend TypeScript client
2. The generated client will provide type-safe methods for calling this endpoint

---

## Success Criteria

- [ ] Handler method implemented in `api.go`
- [ ] Conversion helpers created in `ticket_events.go`
- [ ] Unit tests written and passing
- [ ] Integration tests passing (if applicable)
- [ ] Endpoint returns 200 with event array for valid requests
- [ ] Endpoint returns 404 for non-existent tickets
- [ ] All query parameters (kind, since, until, includeReset) work correctly
- [ ] Changes committed to version control

---

## Estimated Duration

**~3-4 hours** - Handler implementation, conversion logic, and testing
