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

type stepClock struct{ current time.Time }

func (s *stepClock) Now() time.Time {
	now := s.current
	s.current = s.current.Add(time.Second)
	return now
}

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

	changeEvent, err := svc.AppendMarkdownEvent(ctx, created.ID, ticket.MarkdownEventInput{
		Actor: creator,
		Payload: ticket.MarkdownDocEventPayload{
			Type: ticket.MarkdownDocAttached,
			Name: "design",
			Path: "docs/design.md",
		},
		EventTime: now,
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
	require.Equal(t, ticket.TicketEventPayloadTypeMarkdownDoc, listed[0].PayloadType)
	require.NotNil(t, listed[0].Payload.MarkdownDoc)
	require.Equal(t, "design", listed[0].Payload.MarkdownDoc.Name)

	workflowEvent, err := svc.AppendWorkflowEvent(ctx, created.ID, ticket.WorkflowEventInput{
		Actor:     ticket.NewAgentActor("cell-a", "recipe", "exec-1", "hash-1"),
		EventTime: now.Add(time.Minute),
		Payload: ticket.WorkflowEventPayload{
			Type:       ticket.WorkflowEventRunning,
			WorkflowID: ticket.WorkflowID("wf-123"),
			RunID:      ticket.WorkflowRunID("run-1"),
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
	require.Equal(t, ticket.TicketEventPayloadTypeMarkdownDoc, beforeReset[0].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeWorkflow, beforeReset[1].PayloadType)
	require.NotNil(t, beforeReset[0].Payload.MarkdownDoc)
	require.NotNil(t, beforeReset[1].Payload.Workflow)

	reset, err := svc.ResetTicket(ctx, created.ID, ticket.TicketResetInput{
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
	require.Len(t, active, 2)
	require.Equal(t, changeEvent.ID, active[0].ID)
	require.Nil(t, active[0].ResetID)
	require.Equal(t, ticket.TicketEventPayloadTypeMarkdownDoc, active[0].PayloadType)
	require.NotNil(t, active[0].Payload.MarkdownDoc)
	require.Equal(t, ticket.TicketEventPayloadTypeReset, active[1].PayloadType)
	require.NotNil(t, active[1].Payload.Reset)
	require.Equal(t, reset.ID, active[1].Payload.Reset.ResetID)
	require.NotNil(t, active[1].Payload.Reset.AnchorEventID)
	require.Equal(t, changeEvent.ID, *active[1].Payload.Reset.AnchorEventID)
	require.Equal(t, "rollback", active[1].Payload.Reset.Reason)

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
	require.Len(t, all, 3)
	require.Equal(t, ticket.TicketEventPayloadTypeMarkdownDoc, all[0].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeReset, all[1].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeWorkflow, all[2].PayloadType)
	require.NotNil(t, all[0].Payload.MarkdownDoc)
	require.NotNil(t, all[1].Payload.Reset)
	require.NotNil(t, all[2].Payload.Workflow)
	require.NotNil(t, all[2].ResetID)
	require.Equal(t, reset.ID, *all[2].ResetID)

	at := now.Add(30 * time.Second)
	snapshotIter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{At: &at})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, snapshotIter)

	var snapshot []*ticket.TicketEvent
	for {
		evt, err := snapshotIter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		snapshot = append(snapshot, evt)
	}
	require.Len(t, snapshot, 2)
	require.Equal(t, ticket.TicketEventPayloadTypeMarkdownDoc, snapshot[0].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeReset, snapshot[1].PayloadType)
}

func TestServiceIntegration_ResetTicketRestoresSlice(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 12, 8, 0, 0, 0, time.UTC)
	clock := &stepClock{current: now}
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Clock:      clock,
		IDGen:      ticket.NewBase58Generator(ticket.DefaultIDLength),
		EventIDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
	})
	require.NoError(t, err)

	ctx := context.Background()
	creator := ticket.NewUserActor("stage@example.com")

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:  "cell-stage",
		Title: "Stage reset",
		Stage: ticket.Stage("triage"),
		State: ticket.StateWorking,
		Actor: creator,
	})
	require.NoError(t, err)

	firstUpdate, err := svc.UpdateTicket(ctx, created.ID, ticket.UpdateInput{
		ExpectedVersion: created.Version,
		Stage:           ticket.StagePtr(ticket.CompletedStage),
	})
	require.NoError(t, err)
	require.Equal(t, ticket.CompletedStage, firstUpdate.Stage)

	iter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iter)

	var initialEvents []*ticket.TicketEvent
	for {
		evt, err := iter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		initialEvents = append(initialEvents, evt)
	}
	require.Len(t, initialEvents, 1)
	anchorEvent := initialEvents[0]

	reviewStage := ticket.Stage("review")
	secondUpdate, err := svc.UpdateTicket(ctx, created.ID, ticket.UpdateInput{
		ExpectedVersion: firstUpdate.Version,
		Stage:           &reviewStage,
	})
	require.NoError(t, err)
	require.Equal(t, reviewStage, secondUpdate.Stage)

	iterBeforeReset, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{IncludeReset: true})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iterBeforeReset)

	var preResetEvents []*ticket.TicketEvent
	for {
		evt, err := iterBeforeReset.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		preResetEvents = append(preResetEvents, evt)
	}
	require.Len(t, preResetEvents, 2)
	require.Equal(t, anchorEvent.ID, preResetEvents[0].ID)
	stageReviewEvent := preResetEvents[1]
	require.Equal(t, ticket.TicketEventPayloadTypeTicket, stageReviewEvent.PayloadType)

	reset, err := svc.ResetTicket(ctx, created.ID, ticket.TicketResetInput{
		Actor:          creator,
		Reason:         "rewind stage",
		LastValidEvent: &anchorEvent.ID,
	})
	require.NoError(t, err)
	require.NotZero(t, reset.ID)

	postIter, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, postIter)

	var postEvents []*ticket.TicketEvent
	for {
		evt, err := postIter.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		postEvents = append(postEvents, evt)
	}
	require.Len(t, postEvents, 2)
	require.Equal(t, anchorEvent.ID, postEvents[0].ID)
	resetEvent := postEvents[1]
	require.Equal(t, ticket.TicketEventPayloadTypeReset, resetEvent.PayloadType)
	require.NotNil(t, resetEvent.Payload.Reset)

	iterWithReset, err := svc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{IncludeReset: true})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iterWithReset)

	var (
		withReset   []*ticket.TicketEvent
		stageReset  bool
		resetLogged bool
	)
	for {
		evt, err := iterWithReset.Next(ctx)
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		withReset = append(withReset, evt)
		switch evt.PayloadType {
		case ticket.TicketEventPayloadTypeTicket:
			if evt.ID != anchorEvent.ID {
				require.NotNil(t, evt.ResetID)
				require.Equal(t, reset.ID, *evt.ResetID)
				stageReset = true
			}
		case ticket.TicketEventPayloadTypeReset:
			resetLogged = true
		}
	}
	require.Len(t, withReset, 3)
	require.True(t, stageReset)
	require.True(t, resetLogged)

	beforeResetTime := stageReviewEvent.EventTime
	stateBeforeReset, err := svc.GetTicketAt(ctx, created.ID, beforeResetTime)
	require.NoError(t, err)
	require.Equal(t, reviewStage, stateBeforeReset.Stage)

	afterResetTime := resetEvent.EventTime.Add(2 * time.Microsecond)
	stateAfterReset, err := svc.GetTicketAt(ctx, created.ID, afterResetTime)
	require.NoError(t, err)
	require.Equal(t, ticket.CompletedStage, stateAfterReset.Stage)
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
	now := time.Now().UTC()
	infinity := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

	newTicket := &ticket.Ticket{
		ID:         ticket.ID(id),
		CellName:   "cell-a",
		Title:      "Draft",
		Stage:      ticket.Stage("triage"),
		State:      ticket.StateWorking,
		Creator:    ticket.NewUserActor("author@example.com"),
		CreatedAt:  now,
		UpdatedAt:  now,
		ValidFrom:  now,
		ValidUntil: infinity,
	}

	err = store.WithTx(ctx, func(ctx context.Context, txStore ticket.Store) error {
		return txStore.Create(ctx, newTicket)
	})
	require.NoError(t, err)

	persisted, err := store.Get(ctx, newTicket.ID)
	require.NoError(t, err)
	require.Equal(t, newTicket.ID, persisted.ID)
}
