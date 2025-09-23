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
		Store: st,
		Clock: fixedClock{now: now},
		IDGen: stubIDGen{id: "abc"},
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
		Store: &stubStore{},
		Clock: fixedClock{now: time.Now()},
		IDGen: stubIDGen{id: "abc"},
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
		Clock: fixedClock{now: now},
		IDGen: stubIDGen{id: "abc"},
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
		Store: st,
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
