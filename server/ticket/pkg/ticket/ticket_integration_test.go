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

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 11, 10, 30, 0, 0, time.UTC)
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Clock:      fixedClock{now: now},
		IDGen:      ticket.NewBase58Generator(ticket.DefaultIDLength),
		EventIDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
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

func TestServiceIntegration_EventLifecycle(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 11, 12, 0, 0, 0, time.UTC)
	idGen := ticket.NewBase58Generator(ticket.DefaultIDLength)
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Clock:      fixedClock{now: now},
		IDGen:      idGen,
		EventIDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
	})
	require.NoError(t, err)

	ctx := context.Background()
	creator := ticket.NewUserActor("user@example.com")

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:  "cell-a",
		Title: "Lifecycle",
		Stage: ticket.Stage("triage"),
		State: ticket.StateWorking,
		Actor: creator,
	})
	require.NoError(t, err)

	changeEvent, err := svc.AppendEvent(ctx, created.ID, ticket.TicketEventInput{
		Kind:      ticket.TicketEventKindTicket,
		Actor:     creator,
		EventTime: now,
		Payload: ticket.TicketEventBody{
			Ticket: &ticket.TicketEventPayload{
				Changes: []ticket.TicketFieldChange{{
					Field: ticket.TicketFieldName("stage"),
					From:  "triage",
					To:    "analysis",
				}},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, changeEvent.TicketID)
	require.NotZero(t, changeEvent.ID)

	iter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iter)

	var listed []*ticket.TicketEvent
	for {
		evt, err := iter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		listed = append(listed, evt)
	}
	require.Len(t, listed, 1)
	require.Equal(t, changeEvent.ID, listed[0].ID)
	require.Equal(t, ticket.TicketEventPayloadTypeTicket, listed[0].PayloadType)
	require.NotNil(t, listed[0].Payload.Ticket)
	require.Len(t, listed[0].Payload.Ticket.Changes, 1)

	workflowEvent, err := svc.AppendEvent(ctx, created.ID, ticket.TicketEventInput{
		Kind:      ticket.TicketEventKindWorkflow,
		Actor:     ticket.NewAgentActor("cell-a", "recipe", "exec-1", "hash-1"),
		EventTime: now.Add(time.Minute),
		Payload: ticket.TicketEventBody{
			Workflow: &ticket.WorkflowEventPayload{
				Type:       ticket.WorkflowEventRunning,
				WorkflowID: ticket.WorkflowID("wf-123"),
				RunID:      ticket.WorkflowRunID("run-1"),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, workflowEvent.TicketID)

	preResetIter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{IncludeReset: true})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, preResetIter)

	var beforeReset []*ticket.TicketEvent
	for {
		evt, err := preResetIter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		beforeReset = append(beforeReset, evt)
	}
	require.Len(t, beforeReset, 2)
	require.Equal(t, ticket.TicketEventPayloadTypeTicket, beforeReset[0].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeWorkflow, beforeReset[1].PayloadType)
	require.NotNil(t, beforeReset[0].Payload.Ticket)
	require.NotNil(t, beforeReset[1].Payload.Workflow)

	reset, err := svc.ResetEvents(ctx, created.ID, ticket.TicketResetInput{
		Actor:          creator,
		Reason:         "rollback",
		LastValidEvent: &changeEvent.ID,
	})
	require.NoError(t, err)
	require.NotZero(t, reset.ID)
	require.Equal(t, created.ID, reset.TicketID)

	activeIter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, activeIter)

	var active []*ticket.TicketEvent
	for {
		evt, err := activeIter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		active = append(active, evt)
	}
	require.Len(t, active, 1)
	require.Equal(t, changeEvent.ID, active[0].ID)
	require.Nil(t, active[0].ResetID)
	require.Equal(t, ticket.TicketEventPayloadTypeTicket, active[0].PayloadType)
	require.NotNil(t, active[0].Payload.Ticket)

	allIter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{IncludeReset: true})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, allIter)

	var all []*ticket.TicketEvent
	for {
		evt, err := allIter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		all = append(all, evt)
	}
	require.Len(t, all, 2)
	require.NotNil(t, all[1].ResetID)
	require.Equal(t, reset.ID, *all[1].ResetID)
	require.Equal(t, ticket.TicketEventPayloadTypeTicket, all[0].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeWorkflow, all[1].PayloadType)
	require.NotNil(t, all[0].Payload.Ticket)
	require.NotNil(t, all[1].Payload.Workflow)
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
