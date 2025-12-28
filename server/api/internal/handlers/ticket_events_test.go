package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockTicketService for testing
type MockTicketService struct {
	mock.Mock
}

func (m *MockTicketService) CreateTicket(ctx context.Context, input ticket.CreateInput) (*ticket.Ticket, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.Ticket), args.Error(1)
}

func (m *MockTicketService) UpdateTicket(ctx context.Context, id ticket.ID, patch ticket.UpdateInput) (*ticket.Ticket, error) {
	args := m.Called(ctx, id, patch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.Ticket), args.Error(1)
}

func (m *MockTicketService) SearchTickets(ctx context.Context, filter ticket.SearchFilter) (ticket.Iterator[*ticket.Ticket], error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ticket.Iterator[*ticket.Ticket]), args.Error(1)
}

func (m *MockTicketService) SearchStages(ctx context.Context, filter ticket.SearchFilter) (ticket.Iterator[ticket.Stage], error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ticket.Iterator[ticket.Stage]), args.Error(1)
}

func (m *MockTicketService) GetStates(ctx context.Context) ([]ticket.State, error) {
	args := m.Called(ctx)
	return args.Get(0).([]ticket.State), args.Error(1)
}

func (m *MockTicketService) GetTicketAt(ctx context.Context, id ticket.ID, at time.Time) (*ticket.Ticket, error) {
	args := m.Called(ctx, id, at)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.Ticket), args.Error(1)
}

func (m *MockTicketService) AppendWorkflowEvent(ctx context.Context, id ticket.ID, input ticket.WorkflowEventInput) (*ticket.TicketEvent, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.TicketEvent), args.Error(1)
}

func (m *MockTicketService) AppendMarkdownEvent(ctx context.Context, id ticket.ID, input ticket.MarkdownEventInput) (*ticket.TicketEvent, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.TicketEvent), args.Error(1)
}

func (m *MockTicketService) AppendChangeSetEvent(ctx context.Context, id ticket.ID, input ticket.ChangeSetEventInput) (*ticket.TicketEvent, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.TicketEvent), args.Error(1)
}

func (m *MockTicketService) AppendTicketEvent(ctx context.Context, id ticket.ID, input ticket.TicketEventInput) (*ticket.TicketEvent, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.TicketEvent), args.Error(1)
}

func (m *MockTicketService) ListEvents(ctx context.Context, id ticket.ID, filter ticket.TicketEventFilter) (ticket.Iterator[*ticket.TicketEvent], error) {
	args := m.Called(ctx, id, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ticket.Iterator[*ticket.TicketEvent]), args.Error(1)
}

func (m *MockTicketService) ResetTicket(ctx context.Context, id ticket.ID, input ticket.TicketResetInput) (*ticket.TicketReset, error) {
	args := m.Called(ctx, id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ticket.TicketReset), args.Error(1)
}

// MockEventIterator for testing
type MockEventIterator struct {
	events []*ticket.TicketEvent
	index  int
}

func (m *MockEventIterator) Next(ctx context.Context) (*ticket.TicketEvent, error) {
	if m.index >= len(m.events) {
		return nil, ticket.ErrIteratorDone
	}
	event := m.events[m.index]
	m.index++
	return event, nil
}

func (m *MockEventIterator) Close(ctx context.Context) error {
	return nil
}

func TestHandleListTicketEvents_Success(t *testing.T) {
	mockTicketService := new(MockTicketService)
	handlers := &Handlers{
		tickets: mockTicketService,
	}

	projectID := ticket.ProjectID("proj-123")
	ticketID := ticket.ID("ticket-456")
	now := time.Now()

	// Mock ticket exists
	mockTicket := &ticket.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     "Test Ticket",
		Stage:     "todo",
		State:     ticket.StateWorking,
	}
	mockTicketService.On("GetTicketAt", mock.Anything, ticketID, mock.AnythingOfType("time.Time")).
		Return(mockTicket, nil)

	// Mock events
	mockEvents := []*ticket.TicketEvent{
		{
			ID:        "event-1",
			TicketID:  ticketID,
			ProjectID: projectID,
			Kind:      ticket.TicketEventKindWorkflow,
			EventTime: now,
			Actor:     ticket.Actor{Type: ticket.ActorTypeUser, User: &ticket.ActorUser{Email: "test@example.com"}},
			Payload: ticket.TicketEventBody{
				Workflow: &ticket.WorkflowEventPayload{
					Type:       ticket.WorkflowEventCompleted,
					WorkflowID: "wf-123",
					RunID:      "run-456",
				},
			},
		},
	}
	mockIterator := &MockEventIterator{events: mockEvents}
	mockTicketService.On("ListEvents", mock.Anything, ticketID, mock.Anything).
		Return(ticket.Iterator[*ticket.TicketEvent](mockIterator), nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj-123/tickets/ticket-456/events", nil)
	req = mux.SetURLVars(req, map[string]string{
		"projectId": "proj-123",
		"ticketId":  "ticket-456",
	})
	w := httptest.NewRecorder()

	// Execute handler
	handlers.handleListTicketEvents(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var events []openapi.TicketEvent
	err := json.NewDecoder(w.Body).Decode(&events)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "event-1", events[0].Id)
	assert.Equal(t, openapi.TicketEventKindWorkflow, events[0].Kind)
	assert.NotNil(t, events[0].WorkflowPayload)
	assert.Equal(t, openapi.WorkflowEventTypeCompleted, events[0].WorkflowPayload.Type)
}

func TestHandleListTicketEvents_TicketNotFound(t *testing.T) {
	mockTicketService := new(MockTicketService)
	handlers := &Handlers{
		tickets: mockTicketService,
	}

	ticketID := ticket.ID("nonexistent")

	// Mock ticket not found
	mockTicketService.On("GetTicketAt", mock.Anything, ticketID, mock.AnythingOfType("time.Time")).
		Return(nil, gorm.ErrRecordNotFound)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj-123/tickets/nonexistent/events", nil)
	req = mux.SetURLVars(req, map[string]string{
		"projectId": "proj-123",
		"ticketId":  "nonexistent",
	})
	w := httptest.NewRecorder()

	// Execute handler
	handlers.handleListTicketEvents(w, req)

	// Verify response
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleListTicketEvents_WrongProject(t *testing.T) {
	mockTicketService := new(MockTicketService)
	handlers := &Handlers{
		tickets: mockTicketService,
	}

	ticketID := ticket.ID("ticket-456")

	// Mock ticket from different project
	mockTicket := &ticket.Ticket{
		ID:        ticketID,
		ProjectID: "different-project",
		Title:     "Test Ticket",
	}
	mockTicketService.On("GetTicketAt", mock.Anything, ticketID, mock.AnythingOfType("time.Time")).
		Return(mockTicket, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj-123/tickets/ticket-456/events", nil)
	req = mux.SetURLVars(req, map[string]string{
		"projectId": "proj-123",
		"ticketId":  "ticket-456",
	})
	w := httptest.NewRecorder()

	// Execute handler
	handlers.handleListTicketEvents(w, req)

	// Verify response
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleListTicketEvents_WithKindFilter(t *testing.T) {
	mockTicketService := new(MockTicketService)
	handlers := &Handlers{
		tickets: mockTicketService,
	}

	projectID := ticket.ProjectID("proj-123")
	ticketID := ticket.ID("ticket-456")

	// Mock ticket exists
	mockTicket := &ticket.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     "Test Ticket",
	}
	mockTicketService.On("GetTicketAt", mock.Anything, ticketID, mock.AnythingOfType("time.Time")).
		Return(mockTicket, nil)

	// Mock events
	mockIterator := &MockEventIterator{events: []*ticket.TicketEvent{}}
	mockTicketService.On("ListEvents", mock.Anything, ticketID, mock.Anything).
		Return(ticket.Iterator[*ticket.TicketEvent](mockIterator), nil)

	// Create request with kind filter
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj-123/tickets/ticket-456/events?kind=workflow", nil)
	req = mux.SetURLVars(req, map[string]string{
		"projectId": "proj-123",
		"ticketId":  "ticket-456",
	})
	w := httptest.NewRecorder()

	// Execute handler
	handlers.handleListTicketEvents(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	mockTicketService.AssertExpectations(t)
}

func TestHandleListTicketEvents_WithTimeRange(t *testing.T) {
	mockTicketService := new(MockTicketService)
	handlers := &Handlers{
		tickets: mockTicketService,
	}

	projectID := ticket.ProjectID("proj-123")
	ticketID := ticket.ID("ticket-456")

	// Mock ticket exists
	mockTicket := &ticket.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     "Test Ticket",
	}
	mockTicketService.On("GetTicketAt", mock.Anything, ticketID, mock.AnythingOfType("time.Time")).
		Return(mockTicket, nil)

	// Mock events
	mockIterator := &MockEventIterator{events: []*ticket.TicketEvent{}}
	mockTicketService.On("ListEvents", mock.Anything, ticketID, mock.Anything).
		Return(ticket.Iterator[*ticket.TicketEvent](mockIterator), nil)

	// Create request with time range
	since := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	until := time.Now().Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/proj-123/tickets/ticket-456/events?since="+since+"&until="+until, nil)
	req = mux.SetURLVars(req, map[string]string{
		"projectId": "proj-123",
		"ticketId":  "ticket-456",
	})
	w := httptest.NewRecorder()

	// Execute handler
	handlers.handleListTicketEvents(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	mockTicketService.AssertExpectations(t)
}

func TestHandleListTicketEvents_EmptyEventList(t *testing.T) {
	mockTicketService := new(MockTicketService)
	handlers := &Handlers{
		tickets: mockTicketService,
	}

	projectID := ticket.ProjectID("proj-123")
	ticketID := ticket.ID("ticket-456")

	// Mock ticket exists
	mockTicket := &ticket.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     "Test Ticket",
	}
	mockTicketService.On("GetTicketAt", mock.Anything, ticketID, mock.AnythingOfType("time.Time")).
		Return(mockTicket, nil)

	// Mock empty events
	mockIterator := &MockEventIterator{events: []*ticket.TicketEvent{}}
	mockTicketService.On("ListEvents", mock.Anything, ticketID, mock.Anything).
		Return(ticket.Iterator[*ticket.TicketEvent](mockIterator), nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj-123/tickets/ticket-456/events", nil)
	req = mux.SetURLVars(req, map[string]string{
		"projectId": "proj-123",
		"ticketId":  "ticket-456",
	})
	w := httptest.NewRecorder()

	// Execute handler
	handlers.handleListTicketEvents(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var events []openapi.TicketEvent
	err := json.NewDecoder(w.Body).Decode(&events)
	assert.NoError(t, err)
	assert.Len(t, events, 0)
}

func TestConvertEventToAPI_AllEventTypes(t *testing.T) {
	now := time.Now()
	projectID := ticket.ProjectID("proj-123")
	ticketID := ticket.ID("ticket-456")

	tests := []struct {
		name      string
		event     *ticket.TicketEvent
		checkFunc func(*testing.T, openapi.TicketEvent)
	}{
		{
			name: "Workflow Event",
			event: &ticket.TicketEvent{
				ID:        "event-1",
				TicketID:  ticketID,
				ProjectID: projectID,
				Kind:      ticket.TicketEventKindWorkflow,
				EventTime: now,
				Actor:     ticket.Actor{Type: ticket.ActorTypeUser, User: &ticket.ActorUser{Email: "test@example.com"}},
				Payload: ticket.TicketEventBody{
					Workflow: &ticket.WorkflowEventPayload{
						Type:       ticket.WorkflowEventCompleted,
						WorkflowID: "wf-123",
						RunID:      "run-456",
					},
				},
			},
			checkFunc: func(t *testing.T, e openapi.TicketEvent) {
				assert.NotNil(t, e.WorkflowPayload)
				assert.Equal(t, openapi.WorkflowEventTypeCompleted, e.WorkflowPayload.Type)
			},
		},
		{
			name: "Ticket Event",
			event: &ticket.TicketEvent{
				ID:        "event-2",
				TicketID:  ticketID,
				ProjectID: projectID,
				Kind:      ticket.TicketEventKindTicket,
				EventTime: now,
				Actor:     ticket.Actor{Type: ticket.ActorTypeAgent, Agent: &ticket.ActorAgent{CellName: "test-cell"}},
				TicketChanges: []ticket.TicketFieldChange{
					{Field: "stage", From: "todo", To: "doing"},
					{Field: "state", From: "waiting_user", To: "working"},
				},
				Payload: ticket.TicketEventBody{
					Ticket: &ticket.TicketEventPayload{
						Notes: "Updated stage",
					},
				},
			},
			checkFunc: func(t *testing.T, e openapi.TicketEvent) {
				assert.NotNil(t, e.TicketPayload)
				assert.NotNil(t, e.TicketPayload.Stage)
				assert.Equal(t, "doing", *e.TicketPayload.Stage)
			},
		},
		{
			name: "Markdown Event",
			event: &ticket.TicketEvent{
				ID:        "event-3",
				TicketID:  ticketID,
				ProjectID: projectID,
				Kind:      ticket.TicketEventKindMarkdownDoc,
				EventTime: now,
				Actor:     ticket.Actor{Type: ticket.ActorTypeUser, User: &ticket.ActorUser{Email: "test@example.com"}},
				Payload: ticket.TicketEventBody{
					MarkdownDoc: &ticket.MarkdownDocEventPayload{
						Type: ticket.MarkdownDocAttached,
						Name: "README.md",
						Path: "/path/to/README.md",
					},
				},
			},
			checkFunc: func(t *testing.T, e openapi.TicketEvent) {
				assert.NotNil(t, e.MarkdownPayload)
				assert.Equal(t, openapi.MarkdownEventTypeAttached, e.MarkdownPayload.Type)
			},
		},
		{
			name: "ChangeSet Event",
			event: &ticket.TicketEvent{
				ID:        "event-4",
				TicketID:  ticketID,
				ProjectID: projectID,
				Kind:      ticket.TicketEventKindChangeSet,
				EventTime: now,
				Actor:     ticket.Actor{Type: ticket.ActorTypeUser, User: &ticket.ActorUser{Email: "test@example.com"}},
				Payload: ticket.TicketEventBody{
					ChangeSet: &ticket.ChangeSetEventPayload{
						Type:          ticket.ChangeSetAttached,
						Path:          "/src/main.go",
						CommitMessage: "Fix bug",
						TipGitHash:    "abc123",
					},
				},
			},
			checkFunc: func(t *testing.T, e openapi.TicketEvent) {
				assert.NotNil(t, e.ChangesetPayload)
				assert.Equal(t, openapi.ChangeSetEventTypeAttached, e.ChangesetPayload.Type)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTicketEventToAPI(tt.event)
			assert.Equal(t, string(tt.event.ID), result.Id)
			assert.Equal(t, string(tt.event.TicketID), result.TicketId)
			tt.checkFunc(t, result)
		})
	}
}
