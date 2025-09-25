package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"github.com/stretchr/testify/require"
	"gorm.io/plugin/optimisticlock"
)

type stubStore struct {
	createFunc       func(ctx context.Context, ticket *model.Ticket) error
	getFunc          func(ctx context.Context, id model.ID) (*model.Ticket, error)
	updateFunc       func(ctx context.Context, ticket *model.Ticket, fields ...string) error
	searchFunc       func(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error)
	searchStagesFunc func(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error)
}

type stubEventStore struct {
	appendFunc      func(ctx context.Context, event *model.TicketEvent) error
	appendBatchFunc func(ctx context.Context, ticketID model.ID, events []*model.TicketEvent) error
	listFunc        func(ctx context.Context, ticketID model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error)
	markResetFunc   func(ctx context.Context, ticketID model.ID, reset *model.TicketReset, eventIDs []model.TicketEventID) error
}

type sliceIterator[T any] struct {
	items []T
	idx   int
}

func (it *sliceIterator[T]) Next(ctx context.Context) (T, error) {
	var zero T
	if it.idx >= len(it.items) {
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
		Clock:      fixedClock{now: now},
		IDGen:      stubIDGen{id: "abc"},
	})
	require.NoError(t, err)

	actor := NewUserActor("user@example.com")
	created, err := svc.CreateTicket(context.Background(), CreateInput{
		Cell:  "cell-a",
		Title: "Demo",
		Stage: model.CompletedStage,
		State: model.StateWorking,
		Actor: actor,
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
		Clock:      fixedClock{now: time.Now()},
		IDGen:      stubIDGen{id: "abc"},
	})
	require.NoError(t, err)

	_, err = svc.CreateTicket(context.Background(), CreateInput{
		Cell:  "cell-a",
		Title: "Demo",
		Stage: "triage",
		State: model.State("bad"),
		Actor: NewUserActor("user@example.com"),
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
		Clock:      fixedClock{now: now},
		IDGen:      stubIDGen{id: "abc"},
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
func TestAppendEventStoresPayload(t *testing.T) {
	now := time.Date(2024, 9, 12, 9, 0, 0, 0, time.UTC)
	ticketID := model.ID("ticket-1234567890123456789012")

	st := &stubStore{
		getFunc: func(ctx context.Context, id model.ID) (*model.Ticket, error) {
			return &model.Ticket{ID: id}, nil
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
		Clock:      fixedClock{now: now},
		IDGen:      stubIDGen{id: string(ticketID)},
		EventIDGen: stubIDGen{id: "event-123456789012345678901"},
	})
	require.NoError(t, err)

	actor := NewUserActor("owner@example.com")
	change := model.TicketFieldChange{Field: model.TicketFieldName("stage"), From: "triage", To: "analysis"}

	event, err := svc.AppendEvent(context.Background(), ticketID, TicketEventInput{
		Kind:  model.TicketEventKindTicket,
		Actor: actor,
		Payload: model.TicketEventBody{
			Ticket: &model.TicketEventPayload{
				Changes: []model.TicketFieldChange{change},
				Notes:   "updated",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, model.TicketEventID("event-123456789012345678901"), event.ID)
	require.Equal(t, ticketID, event.TicketID)
	require.Equal(t, model.TicketEventPayloadTypeTicket, event.PayloadType)
	require.Len(t, event.Payload.Ticket.Changes, 1)
	require.Equal(t, "updated", event.Payload.Ticket.Notes)

	select {
	case captured := <-appended:
		require.Equal(t, model.TicketEventKindTicket, captured.Kind)
		require.Equal(t, model.TicketEventPayloadTypeTicket, captured.PayloadType)
		require.NotNil(t, captured.Payload.Ticket)
		require.Len(t, captured.Payload.Ticket.Changes, 1)
		require.Equal(t, change, captured.Payload.Ticket.Changes[0])
		require.Equal(t, "updated", captured.TicketData.Notes)
		require.Equal(t, []model.TicketFieldChange{change}, []model.TicketFieldChange(captured.TicketChanges))
	default:
		t.Fatal("expected event to be appended")
	}
}

func TestResetEventsMissingAnchor(t *testing.T) {
	ticketID := model.ID("ticket-evt-000000000000000001")
	anchorID := model.TicketEventID("evt-anchor")
	otherID := model.TicketEventID("evt-other")

	st := &stubStore{
		getFunc: func(ctx context.Context, id model.ID) (*model.Ticket, error) {
			return &model.Ticket{ID: id}, nil
		},
	}

	eventsToReturn := []*model.TicketEvent{
		{ID: otherID, TicketID: ticketID, Kind: model.TicketEventKindTicket},
	}

	evtStore := &stubEventStore{
		listFunc: func(ctx context.Context, ticketID model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error) {
			return &sliceIterator[*model.TicketEvent]{items: eventsToReturn}, nil
		},
	}

	svc, err := New(ServiceConfig{
		Store:      st,
		EventStore: evtStore,
		Clock:      fixedClock{now: time.Now().UTC()},
		IDGen:      stubIDGen{id: string(ticketID)},
		EventIDGen: stubIDGen{id: "reset-id"},
	})
	require.NoError(t, err)

	_, err = svc.ResetEvents(context.Background(), ticketID, TicketResetInput{
		Actor:          NewUserActor("owner@example.com"),
		Reason:         "cleanup",
		LastValidEvent: &anchorID,
	})
	require.ErrorIs(t, err, ErrEventNotFound)
}
