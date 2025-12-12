package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/cell/pkg/cell"
	"github.com/divisive-ai/vibethis/server/project/pkg/project"
	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	eventstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/events"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"github.com/divisive-ai/vibethis/server/ticket/internal/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/plugin/optimisticlock"
)

type stubStore struct {
	createFunc       func(ctx context.Context, ticket *model.Ticket) error
	getFunc          func(ctx context.Context, id model.ID) (*model.Ticket, error)
	getAtFunc        func(ctx context.Context, id model.ID, at time.Time) (*model.Ticket, error)
	updateFunc       func(ctx context.Context, ticket *model.Ticket, fields ...string) error
	searchFunc       func(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error)
	searchStagesFunc func(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error)
}

type stubEventStore struct {
	appendFunc      func(ctx context.Context, event *model.TicketEvent) error
	appendBatchFunc func(ctx context.Context, ticketID model.ID, events []*model.TicketEvent) error
	listFunc        func(ctx context.Context, ticketID model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error)
	markResetFunc   func(ctx context.Context, ticketID model.ID, reset *model.TicketReset, eventIDs []model.TicketEventID) error
	latestResetFunc func(ctx context.Context, ticketID model.ID) (*model.TicketReset, error)
}

type stubProjects struct {
	getFunc    func(ctx context.Context, id project.ID) (*project.Project, error)
	createFunc func(ctx context.Context, input project.CreateInput) (*project.Project, error)
	listFunc   func(ctx context.Context, filter project.SearchFilter) (project.Iterator[*project.Project], error)
	updateFunc func(ctx context.Context, id project.ID, patch project.UpdateInput) (*project.Project, error)
	deleteFunc func(ctx context.Context, id project.ID) error
}

type stubCells struct {
	createFunc  func(ctx context.Context, input cell.CreateInput) (*cell.Cell, error)
	getFunc     func(ctx context.Context, id cell.ID) (*cell.Cell, error)
	listFunc    func(ctx context.Context, filter cell.SearchFilter) (cell.Iterator[*cell.Cell], error)
	updateFunc  func(ctx context.Context, id cell.ID, patch cell.UpdateInput) (*cell.Cell, error)
	deleteFunc  func(ctx context.Context, id cell.ID) error
	replaceFunc func(ctx context.Context, id cell.ID, deps []cell.ID) error
	syncFunc    func(ctx context.Context, projectID project.ID, pop cell.Populator, opts cell.SyncOptions) (*cell.SyncResult, error)
}

type sliceIterator[T any] struct {
	items   []T
	idx     int
	doneErr error
}

func (it *sliceIterator[T]) Next(ctx context.Context) (T, error) {
	var zero T
	if it.idx >= len(it.items) {
		if it.doneErr != nil {
			return zero, it.doneErr
		}
		return zero, store.ErrIteratorDone
	}
	item := it.items[it.idx]
	it.idx++
	return item, nil
}

func (it *sliceIterator[T]) Close(ctx context.Context) error { return nil }

func (s *stubStore) WithTx(ctx context.Context, fn func(ctx context.Context, st store.Store) error) error {
	if s == nil {
		return errors.New("nil store")
	}
	return fn(ctx, s)
}

func (s *stubStore) Create(ctx context.Context, ticket *model.Ticket) error {
	if s.createFunc != nil {
		return s.createFunc(ctx, ticket)
	}
	return nil
}

func (s *stubStore) Get(ctx context.Context, id model.ID) (*model.Ticket, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (s *stubStore) GetAt(ctx context.Context, id model.ID, at time.Time) (*model.Ticket, error) {
	if s.getAtFunc != nil {
		return s.getAtFunc(ctx, id, at)
	}
	return s.Get(ctx, id)
}

func (s *stubStore) Search(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error) {
	if s.searchFunc != nil {
		return s.searchFunc(ctx, filter)
	}
	return nil, errors.New("not implemented")
}

func (s *stubStore) SearchStages(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error) {
	if s.searchStagesFunc != nil {
		return s.searchStagesFunc(ctx, filter)
	}
	return nil, errors.New("not implemented")
}

func (s *stubStore) Update(ctx context.Context, ticket *model.Ticket, fields ...string) error {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, ticket, fields...)
	}
	return nil
}

func (s *stubStore) DB() *gorm.DB { return nil }

func (s *stubEventStore) Append(ctx context.Context, event *model.TicketEvent) error {
	if s == nil {
		return errors.New("nil event store")
	}
	if s.appendFunc != nil {
		return s.appendFunc(ctx, event)
	}
	return nil
}

func (s *stubEventStore) AppendBatch(ctx context.Context, ticketID model.ID, events []*model.TicketEvent) error {
	if s == nil {
		return errors.New("nil event store")
	}
	if s.appendBatchFunc != nil {
		return s.appendBatchFunc(ctx, ticketID, events)
	}
	return nil
}

func (s *stubEventStore) ListByTicket(ctx context.Context, ticketID model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error) {
	if s == nil {
		return nil, errors.New("nil event store")
	}
	if s.listFunc != nil {
		return s.listFunc(ctx, ticketID, filter)
	}
	return nil, errors.New("not implemented")
}

func (s *stubEventStore) MarkReset(ctx context.Context, ticketID model.ID, reset *model.TicketReset, eventIDs []model.TicketEventID) error {
	if s == nil {
		return errors.New("nil event store")
	}
	if s.markResetFunc != nil {
		return s.markResetFunc(ctx, ticketID, reset, eventIDs)
	}
	return nil
}

func (s *stubEventStore) LatestReset(ctx context.Context, ticketID model.ID) (*model.TicketReset, error) {
	if s == nil {
		return nil, errors.New("nil event store")
	}
	if s.latestResetFunc != nil {
		return s.latestResetFunc(ctx, ticketID)
	}
	return nil, nil
}

func (s *stubProjects) CreateProject(ctx context.Context, input project.CreateInput) (*project.Project, error) {
	if s.createFunc != nil {
		return s.createFunc(ctx, input)
	}
	return nil, errors.New("not implemented")
}

func (s *stubProjects) GetProject(ctx context.Context, id project.ID) (*project.Project, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (s *stubProjects) ListProjects(ctx context.Context, filter project.SearchFilter) (project.Iterator[*project.Project], error) {
	if s.listFunc != nil {
		return s.listFunc(ctx, filter)
	}
	return nil, errors.New("not implemented")
}

func (s *stubProjects) UpdateProject(ctx context.Context, id project.ID, patch project.UpdateInput) (*project.Project, error) {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, id, patch)
	}
	return nil, errors.New("not implemented")
}

func (s *stubProjects) DeleteProject(ctx context.Context, id project.ID) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}
	return nil
}

func (s *stubCells) CreateCell(ctx context.Context, input cell.CreateInput) (*cell.Cell, error) {
	if s.createFunc != nil {
		return s.createFunc(ctx, input)
	}
	return nil, errors.New("not implemented")
}

func (s *stubCells) GetCell(ctx context.Context, id cell.ID) (*cell.Cell, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (s *stubCells) ListCells(ctx context.Context, filter cell.SearchFilter) (cell.Iterator[*cell.Cell], error) {
	if s.listFunc != nil {
		return s.listFunc(ctx, filter)
	}
	return nil, errors.New("not implemented")
}

func (s *stubCells) UpdateCell(ctx context.Context, id cell.ID, patch cell.UpdateInput) (*cell.Cell, error) {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, id, patch)
	}
	return nil, errors.New("not implemented")
}

func (s *stubCells) MarkDeleted(ctx context.Context, id cell.ID) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}
	return nil
}

func (s *stubCells) ReplaceDependencies(ctx context.Context, id cell.ID, deps []cell.ID) error {
	if s.replaceFunc != nil {
		return s.replaceFunc(ctx, id, deps)
	}
	return nil
}

func (s *stubCells) SyncFromPopulator(ctx context.Context, projectID project.ID, pop cell.Populator, opts cell.SyncOptions) (*cell.SyncResult, error) {
	if s.syncFunc != nil {
		return s.syncFunc(ctx, projectID, pop, opts)
	}
	return nil, errors.New("not implemented")
}

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type stubIDGen struct {
	id  string
	err error
}

func (s stubIDGen) NewID() (string, error) { return s.id, s.err }

func version(v int64) optimisticlock.Version {
	return optimisticlock.Version{Int64: v, Valid: true}
}

const testProjectID = project.ID("prj123456789012345678901234")
const testCellID = cell.ID("cel123456789012345678901234")

var okProjects = &stubProjects{
	getFunc: func(ctx context.Context, id project.ID) (*project.Project, error) {
		return &project.Project{ID: id}, nil
	},
}

var okCells = &stubCells{
	listFunc: func(ctx context.Context, filter cell.SearchFilter) (cell.Iterator[*cell.Cell], error) {
		name := "cell-a"
		if len(filter.Names) > 0 {
			name = filter.Names[0]
		}
		projectID := testProjectID
		if len(filter.ProjectIDs) > 0 {
			projectID = filter.ProjectIDs[0]
		}
		return &sliceIterator[*cell.Cell]{
			items: []*cell.Cell{
				{ID: testCellID, ProjectID: projectID, Name: name, WorkingPath: "/tmp/" + name},
			},
			doneErr: cell.ErrIteratorDone,
		}, nil
	},
}

func TestCreateTicketCompletedStageSetsTimestamp(t *testing.T) {
	ticketCaptured := make(chan *model.Ticket, 1)
	st := &stubStore{
		createFunc: func(ctx context.Context, ticket *model.Ticket) error {
			ticketCaptured <- ticket
			return nil
		},
	}
	now := time.Date(2024, 9, 10, 12, 0, 0, 0, time.UTC)
	svc, err := New(ServiceConfig{
		Store:      st,
		EventStore: &stubEventStore{},
		Projects:   okProjects,
		Cells:      okCells,
		Clock:      fixedClock{now: now},
		IDGen:      stubIDGen{id: "abc"},
	})
	require.NoError(t, err)

	actor := NewUserActor("user@example.com")
	created, err := svc.CreateTicket(context.Background(), CreateInput{
		Cell:      "cell-a",
		ProjectID: testProjectID,
		Title:     "Demo",
		Stage:     model.CompletedStage,
		State:     model.StateWorking,
		Actor:     actor,
	})
	require.NoError(t, err)
	require.NotNil(t, created.CompletedAt)
	require.WithinDuration(t, now, created.CompletedAt.UTC(), time.Millisecond)

	captured := <-ticketCaptured
	require.NotNil(t, captured.CompletedAt)
	require.WithinDuration(t, now, captured.CompletedAt.UTC(), time.Millisecond)
}

func TestCreateTicketInvalidState(t *testing.T) {
	svc, err := New(ServiceConfig{
		Store:      &stubStore{},
		EventStore: &stubEventStore{},
		Projects:   okProjects,
		Cells:      okCells,
		Clock:      fixedClock{now: time.Now()},
		IDGen:      stubIDGen{id: "abc"},
	})
	require.NoError(t, err)

	_, err = svc.CreateTicket(context.Background(), CreateInput{
		Cell:      "cell-a",
		ProjectID: testProjectID,
		Title:     "Demo",
		Stage:     "triage",
		State:     model.State("bad"),
		Actor:     NewUserActor("user@example.com"),
	})
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestUpdateTicketVersionConflict(t *testing.T) {
	now := time.Date(2024, 9, 10, 13, 0, 0, 0, time.UTC)
	svc, err := New(ServiceConfig{
		Store: &stubStore{
			getFunc: func(ctx context.Context, id model.ID) (*model.Ticket, error) {
				return &model.Ticket{
					ID:      id,
					Version: version(1),
					Stage:   "triage",
					State:   model.StateWorking,
					Creator: NewUserActor("user@example.com"),
				}, nil
			},
		},
		EventStore: &stubEventStore{},
		Projects:   okProjects,
		Cells:      okCells,
		Clock:      fixedClock{now: now},
		IDGen:      stubIDGen{id: "abc"},
	})
	require.NoError(t, err)

	_, err = svc.UpdateTicket(context.Background(), model.ID("abc"), UpdateInput{
		ExpectedVersion: version(2),
		Stage:           StagePtr("review"),
	})
	require.ErrorIs(t, err, ErrVersionConflict)
}

func TestUpdateTicketCompletedStageSetsTimestamp(t *testing.T) {
	now := time.Date(2024, 9, 10, 14, 0, 0, 0, time.UTC)
	stored := &model.Ticket{
		ID:        "abc",
		Version:   version(2),
		Stage:     "triage",
		State:     model.StateWorking,
		ProjectID: testProjectID,
		Creator:   NewUserActor("user@example.com"),
		UpdatedAt: now.Add(-time.Hour),
	}
	st := &stubStore{
		getFunc: func(ctx context.Context, id model.ID) (*model.Ticket, error) {
			return stored, nil
		},
		updateFunc: func(ctx context.Context, ticket *model.Ticket, fields ...string) error {
			stored = ticket
			return nil
		},
	}
	svc, err := New(ServiceConfig{
		Store:      st,
		EventStore: &stubEventStore{},
		Projects: &stubProjects{
			getFunc: func(ctx context.Context, id project.ID) (*project.Project, error) {
				return &project.Project{ID: id}, nil
			},
		},
		Cells: okCells,
		Clock: fixedClock{now: now},
		IDGen: stubIDGen{id: "abc"},
	})
	require.NoError(t, err)

	updated, err := svc.UpdateTicket(context.Background(), model.ID("abc"), UpdateInput{
		ExpectedVersion: version(2),
		Stage:           StagePtr(model.CompletedStage),
	})
	require.NoError(t, err)
	require.NotNil(t, updated.CompletedAt)
	require.WithinDuration(t, now, updated.CompletedAt.UTC(), time.Millisecond)
}

func TestUpdateTicketDescriptionChange(t *testing.T) {
	now := time.Now().UTC()
	original := &model.Ticket{
		ID:          "ticket-desc",
		Version:     version(3),
		Stage:       "triage",
		State:       model.StateWorking,
		ProjectID:   testProjectID,
		Description: "initial",
		Creator:     NewUserActor("user@example.com"),
		UpdatedAt:   now.Add(-time.Hour),
	}
	st := &stubStore{
		getFunc: func(ctx context.Context, id model.ID) (*model.Ticket, error) {
			return original, nil
		},
		updateFunc: func(ctx context.Context, ticket *model.Ticket, fields ...string) error {
			return nil
		},
		createFunc: func(ctx context.Context, ticket *model.Ticket) error {
			return nil
		},
	}
	var appended *model.TicketEvent
	events := &stubEventStore{
		appendFunc: func(ctx context.Context, event *model.TicketEvent) error {
			appended = event
			return nil
		},
	}

	svc, err := New(ServiceConfig{
		Store:      st,
		EventStore: events,
		Projects:   okProjects,
		Cells:      okCells,
		Clock:      fixedClock{now: now},
	})
	require.NoError(t, err)

	newDesc := "  refreshed summary  "
	updated, err := svc.UpdateTicket(context.Background(), model.ID("ticket-desc"), UpdateInput{
		ExpectedVersion: version(3),
		Description:     &newDesc,
	})
	require.NoError(t, err)
	require.Equal(t, "refreshed summary", updated.Description)
	require.NotNil(t, appended)
	require.Equal(t, model.TicketEventKindTicket, appended.Kind)
	require.NotNil(t, appended.Payload.Ticket)
	changes := appended.Payload.Ticket.Changes
	require.Len(t, changes, 1)
	require.Equal(t, model.TicketFieldName("description"), changes[0].Field)
	require.Equal(t, "initial", changes[0].From)
	require.Equal(t, "refreshed summary", changes[0].To)
}
func TestAppendWorkflowEventStoresPayload(t *testing.T) {
	now := time.Date(2024, 9, 12, 9, 0, 0, 0, time.UTC)
	ticketID := model.ID("ticket-1234567890123456789012")

	st := &stubStore{
		getFunc: func(ctx context.Context, id model.ID) (*model.Ticket, error) {
			return &model.Ticket{ID: id, ProjectID: testProjectID}, nil
		},
	}

	appended := make(chan *model.TicketEvent, 1)
	evtStore := &stubEventStore{
		appendFunc: func(ctx context.Context, event *model.TicketEvent) error {
			appended <- event
			return nil
		},
	}

	svc, err := New(ServiceConfig{
		Store:      st,
		EventStore: evtStore,
		Projects:   okProjects,
		Cells:      okCells,
		Clock:      fixedClock{now: now},
		IDGen:      stubIDGen{id: string(ticketID)},
		EventIDGen: stubIDGen{id: "event-123456789012345678901"},
	})
	require.NoError(t, err)

	actor := NewUserActor("owner@example.com")

	event, err := svc.AppendWorkflowEvent(context.Background(), ticketID, WorkflowEventInput{
		Actor: actor,
		Payload: model.WorkflowEventPayload{
			Type:       model.WorkflowEventRunning,
			WorkflowID: model.WorkflowID("wf-1"),
			RunID:      model.WorkflowRunID("run-1"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, model.TicketEventID("event-123456789012345678901"), event.ID)
	require.Equal(t, ticketID, event.TicketID)
	require.Equal(t, model.TicketEventPayloadTypeWorkflow, event.PayloadType)
	require.NotNil(t, event.Payload.Workflow)
	require.Equal(t, model.WorkflowEventRunning, event.Payload.Workflow.Type)
	require.Equal(t, model.WorkflowID("wf-1"), event.Payload.Workflow.WorkflowID)
	require.Equal(t, model.WorkflowRunID("run-1"), event.Payload.Workflow.RunID)

	select {
	case captured := <-appended:
		require.Equal(t, model.TicketEventKindWorkflow, captured.Kind)
		require.Equal(t, model.TicketEventPayloadTypeWorkflow, captured.PayloadType)
		require.NotNil(t, captured.Payload.Workflow)
		require.Equal(t, model.WorkflowEventRunning, captured.Payload.Workflow.Type)
		require.Equal(t, model.WorkflowID("wf-1"), captured.Payload.Workflow.WorkflowID)
		require.Equal(t, model.WorkflowRunID("run-1"), captured.Payload.Workflow.RunID)
	default:
		t.Fatal("expected event to be appended")
	}
}

func TestAppendTicketEventStoresNotes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 9, 12, 10, 30, 0, 0, time.UTC)
	ticketID := model.ID("ticket-note-1")

	store := &stubStore{
		getFunc: func(context.Context, model.ID) (*model.Ticket, error) {
			return &model.Ticket{ID: ticketID, ProjectID: testProjectID, ValidUntil: infinity()}, nil
		},
	}

	var appended *model.TicketEvent
	events := &stubEventStore{
		appendFunc: func(_ context.Context, evt *model.TicketEvent) error {
			appended = evt
			return nil
		},
		latestResetFunc: func(context.Context, model.ID) (*model.TicketReset, error) {
			return nil, nil
		},
	}

	svc, err := New(ServiceConfig{
		Store:      store,
		EventStore: events,
		Projects:   okProjects,
		Cells:      okCells,
		Clock:      fixedClock{now: now},
	})
	require.NoError(t, err)

	result, err := svc.AppendTicketEvent(ctx, ticketID, TicketEventInput{
		Actor:     NewUserActor("notes@example.com"),
		Notes:     "  captured details  ",
		EventTime: now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, appended)
	require.Equal(t, model.TicketEventKindTicket, result.Kind)
	require.Equal(t, "captured details", result.Payload.Ticket.Notes)
	require.Equal(t, result.ID, appended.ID)
	require.Equal(t, "captured details", appended.Payload.Ticket.Notes)
	require.WithinDuration(t, now.Add(2*time.Minute), result.EventTime, time.Microsecond)
}

func TestAppendTicketEventEmptyNotes(t *testing.T) {
	svc, err := New(ServiceConfig{
		Store: &stubStore{
			getFunc: func(context.Context, model.ID) (*model.Ticket, error) {
				return &model.Ticket{ID: model.ID("ticket-note-1"), ProjectID: testProjectID, ValidUntil: infinity()}, nil
			},
		},
		EventStore: &stubEventStore{
			latestResetFunc: func(context.Context, model.ID) (*model.TicketReset, error) {
				return nil, nil
			},
		},
		Projects: okProjects,
		Cells:    okCells,
	})
	require.NoError(t, err)

	_, err = svc.AppendTicketEvent(context.Background(), model.ID("ticket-note-1"), TicketEventInput{
		Actor: NewUserActor("notes@example.com"),
		Notes: " ",
	})
	require.ErrorIs(t, err, ErrInvalidEventPayload)
}

func TestResetTicketMissingAnchor(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	ticketStore, err := store.New(pg.DB)
	require.NoError(t, err)

	evtStore, err := eventstore.New(pg.DB)
	require.NoError(t, err)

	projStore, err := project.NewStore(pg.DB)
	require.NoError(t, err)
	projSvc, err := project.NewService(project.ServiceConfig{Store: projStore})
	require.NoError(t, err)
	proj, err := projSvc.CreateProject(context.Background(), project.CreateInput{
		Name:        "reset-project",
		GitRepoPath: "git@example.com/reset.git",
	})
	require.NoError(t, err)

	cellStore, err := cell.NewStore(pg.DB)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projSvc})
	require.NoError(t, err)
	_, err = cellSvc.CreateCell(context.Background(), cell.CreateInput{
		ProjectID:   proj.ID,
		Name:        "cell-reset",
		WorkingPath: "/repo/reset",
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	svc, err := New(ServiceConfig{
		Store:      ticketStore,
		EventStore: evtStore,
		Projects:   projSvc,
		Cells:      cellSvc,
		Clock:      fixedClock{now: now},
	})
	require.NoError(t, err)

	ctx := context.Background()
	created, err := svc.CreateTicket(ctx, CreateInput{
		Cell:      "cell-reset",
		ProjectID: proj.ID,
		Title:     "Rollback",
		Stage:     model.Stage("triage"),
		State:     model.StateWorking,
		Actor:     NewUserActor("owner@example.com"),
	})
	require.NoError(t, err)

	missing := model.TicketEventID("evt-missing")
	_, err = svc.ResetTicket(ctx, created.ID, TicketResetInput{
		Actor:          NewUserActor("owner@example.com"),
		Reason:         "cleanup",
		LastValidEvent: &missing,
	})
	require.ErrorIs(t, err, ErrEventNotFound)
}
