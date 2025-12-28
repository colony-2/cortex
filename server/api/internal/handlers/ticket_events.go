package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/gorilla/mux"
)

func (h *Handlers) handleListTicketEvents(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}

	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	ticketID := ticket.ID(vars["ticketId"])

	// Verify ticket exists and belongs to project
	tk, err := h.tickets.GetTicketAt(r.Context(), ticketID, time.Now().UTC())
	if err != nil {
		writeDomainError(w, err, ticketErrorStatus(err))
		return
	}
	if tk.ProjectID != projectID {
		writeError(w, errors.New("ticket does not belong to project"), http.StatusNotFound)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	filter := ticket.TicketEventFilter{}

	// Filter by kind
	if kindStr := query.Get("kind"); kindStr != "" {
		kind := ticket.TicketEventKind(kindStr)
		filter.Kinds = []ticket.TicketEventKind{kind}
	}

	// Filter by time range
	if sinceStr := query.Get("since"); sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		filter.Since = &since
	}
	if untilStr := query.Get("until"); untilStr != "" {
		until, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		filter.Until = &until
	}

	// Include reset events
	if includeResetStr := query.Get("includeReset"); includeResetStr == "true" {
		filter.IncludeReset = true
	}

	// Retrieve events
	iter, err := h.tickets.ListEvents(r.Context(), ticketID, filter)
	if err != nil {
		writeDomainError(w, err, ticketErrorStatus(err))
		return
	}

	// Collect events from iterator
	var events []*ticket.TicketEvent
	for {
		event, err := iter.Next(r.Context())
		if err != nil {
			if errors.Is(err, ticket.ErrIteratorDone) {
				break
			}
			writeDomainError(w, err, http.StatusInternalServerError)
			return
		}
		events = append(events, event)
	}

	// Convert to OpenAPI types
	apiEvents := make([]openapi.TicketEvent, len(events))
	for i, event := range events {
		apiEvents[i] = convertTicketEventToAPI(event)
	}

	// Return JSON response
	writeJSON(w, http.StatusOK, apiEvents)
}

// convertTicketEventToAPI converts a domain TicketEvent to OpenAPI TicketEvent
func convertTicketEventToAPI(e *ticket.TicketEvent) openapi.TicketEvent {
	event := openapi.TicketEvent{
		Id:        string(e.ID),
		TicketId:  string(e.TicketID),
		ProjectId: string(e.ProjectID),
		Kind:      convertEventKindToAPI(e.Kind),
		Timestamp: e.EventTime,
		Actor:     toOpenAPITicketActor(e.Actor),
	}

	// Set resetId if present
	if e.ResetID != nil {
		resetIDStr := string(*e.ResetID)
		event.ResetId = &resetIDStr
	}

	// Set appropriate payload based on kind
	if e.Payload.Workflow != nil {
		event.WorkflowPayload = &openapi.WorkflowEventPayload{
			Type:       convertWorkflowEventTypeToAPI(e.Payload.Workflow.Type),
			WorkflowId: string(e.Payload.Workflow.WorkflowID),
			RunId:      string(e.Payload.Workflow.RunID),
		}
	}

	if e.Payload.Ticket != nil {
		event.TicketPayload = convertTicketEventPayloadToAPI(e.Payload.Ticket, e.TicketChanges)
	}

	if e.Payload.MarkdownDoc != nil {
		event.MarkdownPayload = &openapi.MarkdownEventPayload{
			Type: convertMarkdownEventTypeToAPI(e.Payload.MarkdownDoc.Type),
			Name: e.Payload.MarkdownDoc.Name,
			Path: e.Payload.MarkdownDoc.Path,
		}
	}

	if e.Payload.ChangeSet != nil {
		payload := e.Payload.ChangeSet
		event.ChangesetPayload = &openapi.ChangeSetEventPayload{
			Type: convertChangeSetEventTypeToAPI(payload.Type),
			Path: payload.Path,
		}
		if payload.CommitMessage != "" {
			event.ChangesetPayload.CommitMessage = &payload.CommitMessage
		}
		if payload.BaseGitHash != "" {
			event.ChangesetPayload.BaseHash = &payload.BaseGitHash
		}
		if payload.ParentGitHash != "" {
			event.ChangesetPayload.ParentHash = &payload.ParentGitHash
		}
		if payload.TipGitHash != "" {
			event.ChangesetPayload.TipHash = &payload.TipGitHash
		}
	}

	if e.Payload.Reset != nil {
		payload := e.Payload.Reset
		event.ResetPayload = &openapi.ResetEventPayload{
			ResetId: string(payload.ResetID),
			Reason:  payload.Reason,
		}
		if payload.AnchorEventID != nil {
			anchorStr := string(*payload.AnchorEventID)
			event.ResetPayload.AnchorEventId = &anchorStr
		}
	}

	return event
}

// convertTicketEventPayloadToAPI converts ticket event payload
func convertTicketEventPayloadToAPI(payload *ticket.TicketEventPayload, changes []ticket.TicketFieldChange) *openapi.TicketEventPayload {
	result := &openapi.TicketEventPayload{}

	if payload.Notes != "" {
		result.Notes = &payload.Notes
	}

	// Extract stage, state, description from changes
	for _, change := range changes {
		switch change.Field {
		case "stage":
			if change.To != "" {
				result.Stage = &change.To
			}
		case "state":
			if change.To != "" {
				state := openapi.TicketState(change.To)
				result.State = &state
			}
		case "description":
			if change.To != "" {
				result.Description = &change.To
			}
		case "completed_at":
			if change.To != "" {
				completedAt, err := time.Parse(time.RFC3339, change.To)
				if err == nil {
					result.CompletedAt = &completedAt
				}
			}
		}
	}

	return result
}

// convertEventKindToAPI converts domain EventKind to OpenAPI TicketEventKind
func convertEventKindToAPI(kind ticket.TicketEventKind) openapi.TicketEventKind {
	switch kind {
	case ticket.TicketEventKindTicket:
		return openapi.TicketEventKindTicket
	case ticket.TicketEventKindWorkflow:
		return openapi.TicketEventKindWorkflow
	case ticket.TicketEventKindMarkdownDoc:
		return openapi.TicketEventKindMarkdownDoc
	case ticket.TicketEventKindChangeSet:
		return openapi.TicketEventKindChangeset
	case ticket.TicketEventKindReset:
		return openapi.TicketEventKindReset
	default:
		return openapi.TicketEventKindTicket // fallback
	}
}

// convertWorkflowEventTypeToAPI converts domain to OpenAPI workflow event type
func convertWorkflowEventTypeToAPI(t ticket.WorkflowEventType) openapi.WorkflowEventType {
	switch t {
	case ticket.WorkflowEventRunning:
		return openapi.WorkflowEventTypeRunning
	case ticket.WorkflowEventCompleted:
		return openapi.WorkflowEventTypeCompleted
	case ticket.WorkflowEventFailed:
		return openapi.WorkflowEventTypeFailed
	default:
		return openapi.WorkflowEventTypeRunning // fallback
	}
}

// convertMarkdownEventTypeToAPI converts domain to OpenAPI markdown event type
func convertMarkdownEventTypeToAPI(t ticket.MarkdownDocEventType) openapi.MarkdownEventType {
	switch t {
	case ticket.MarkdownDocAttached:
		return openapi.MarkdownEventTypeAttached
	case ticket.MarkdownDocOverridden:
		return openapi.MarkdownEventTypeOverridden
	case ticket.MarkdownDocRemoved:
		return openapi.MarkdownEventTypeRemoved
	default:
		return openapi.MarkdownEventTypeAttached // fallback
	}
}

// convertChangeSetEventTypeToAPI converts domain to OpenAPI changeset event type
func convertChangeSetEventTypeToAPI(t ticket.ChangeSetEventType) openapi.ChangeSetEventType {
	switch t {
	case ticket.ChangeSetAttached:
		return openapi.ChangeSetEventTypeAttached
	case ticket.ChangeSetOverridden:
		return openapi.ChangeSetEventTypeOverridden
	case ticket.ChangeSetRemoved:
		return openapi.ChangeSetEventTypeRemoved
	default:
		return openapi.ChangeSetEventTypeAttached // fallback
	}
}
