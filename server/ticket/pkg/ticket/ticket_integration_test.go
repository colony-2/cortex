package ticket_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/testutil"
	"github.com/divisive-ai/vibethis/server/ticket/pkg/ticket"
	"github.com/stretchr/testify/require"
)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

func TestServiceIntegration_CreateSearchUpdate(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 11, 10, 30, 0, 0, time.UTC)
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store: store,
		Clock: fixedClock{now: now},
		IDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
	})
	require.NoError(t, err)

	ctx := context.Background()

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:  "cell-a",
		Title: "Proposal review",
		Stage: ticket.Stage("triage"),
		State: ticket.StateWorking,
		Actor: ticket.NewUserActor("designer@example.com"),
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	iter, err := svc.SearchTickets(ctx, ticket.SearchFilter{StageAny: []ticket.Stage{ticket.Stage("triage")}})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iter)

	var tickets []*ticket.Ticket
	for {
		tkt, err := iter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		tickets = append(tickets, tkt)
	}
	require.Len(t, tickets, 1)
	require.Equal(t, created.ID, tickets[0].ID)

	updated, err := svc.UpdateTicket(ctx, created.ID, ticket.UpdateInput{
		ExpectedVersion: created.Version,
		Stage:           ticket.StagePtr(ticket.CompletedStage),
	})
	require.NoError(t, err)
	require.NotNil(t, updated.CompletedAt)
	require.WithinDuration(t, now, updated.CompletedAt.UTC(), time.Millisecond)

	stagesIter, err := svc.SearchStages(ctx, ticket.SearchFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, stagesIter)

	var stages []ticket.Stage
	for {
		stage, err := stagesIter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		stages = append(stages, stage)
	}
	require.ElementsMatch(t, []ticket.Stage{ticket.CompletedStage}, stages)
}

func TestStoreWithTransaction(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	ctx := context.Background()
	gen := ticket.NewBase58Generator(ticket.DefaultIDLength)
	id, err := gen.NewID()
	require.NoError(t, err)

	newTicket := &ticket.Ticket{
		ID:        ticket.ID(id),
		CellName:  "cell-a",
		Title:     "Draft",
		Stage:     ticket.Stage("triage"),
		State:     ticket.StateWorking,
		Creator:   ticket.NewUserActor("author@example.com"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	err = store.WithTx(ctx, func(ctx context.Context, txStore ticket.Store) error {
		return txStore.Create(ctx, newTicket)
	})
	require.NoError(t, err)

	persisted, err := store.Get(ctx, newTicket.ID)
	require.NoError(t, err)
	require.Equal(t, newTicket.ID, persisted.ID)
}
