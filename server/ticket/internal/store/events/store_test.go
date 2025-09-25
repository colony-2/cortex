package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	"github.com/divisive-ai/vibethis/server/ticket/internal/store/events"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"github.com/divisive-ai/vibethis/server/ticket/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestStoreAppendListReset(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	evtStore, err := events.New(pg.DB)
	require.NoError(t, err)

	ctx := context.Background()

	ticketID := model.ID("12345678901234567890123456")
	now := time.Now().UTC()

	first := &model.TicketEvent{
		ID:        model.TicketEventID("evt12345678901234567890123"),
		TicketID:  ticketID,
		Actor:     model.Actor{Type: model.ActorTypeUser, User: &model.ActorUser{Email: "owner@example.com"}},
		EventTime: now,
		CreatedAt: now,
	}
	first.SetPayload(model.TicketEventKindTicket, model.TicketEventBody{
		Ticket: &model.TicketEventPayload{Notes: "initial"},
	})

	second := &model.TicketEvent{
		ID:        model.TicketEventID("evt22345678901234567890123"),
		TicketID:  ticketID,
		Actor:     model.Actor{Type: model.ActorTypeAgent, Agent: &model.ActorAgent{CellName: "cell", WorkflowName: "wf", ExecutionID: "exec", InvocationHash: "hash"}},
		EventTime: now.Add(time.Minute),
		CreatedAt: now.Add(time.Minute),
	}
	second.SetPayload(model.TicketEventKindWorkflow, model.TicketEventBody{
		Workflow: &model.WorkflowEventPayload{Type: model.WorkflowEventRunning, WorkflowID: model.WorkflowID("wf-1"), RunID: model.WorkflowRunID("run-1")},
	})

	require.NoError(t, evtStore.AppendBatch(ctx, ticketID, []*model.TicketEvent{first, second}))

	iter, err := evtStore.ListByTicket(ctx, ticketID, model.TicketEventFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iter)

	eventsByTicket := collectEvents(t, ctx, iter)
	require.Len(t, eventsByTicket, 2)
	require.Equal(t, first.ID, eventsByTicket[0].ID)
	require.Equal(t, second.ID, eventsByTicket[1].ID)
	require.Equal(t, ticketID, eventsByTicket[0].TicketID)
	require.Equal(t, ticketID, eventsByTicket[1].TicketID)
	require.Equal(t, model.TicketEventPayloadTypeTicket, eventsByTicket[0].PayloadType)
	require.Equal(t, model.TicketEventPayloadTypeWorkflow, eventsByTicket[1].PayloadType)
	require.NotNil(t, eventsByTicket[0].Payload.Ticket)
	require.Equal(t, "initial", eventsByTicket[0].Payload.Ticket.Notes)

	wfIter, err := evtStore.ListByTicket(ctx, ticketID, model.TicketEventFilter{
		Kinds:        []model.TicketEventKind{model.TicketEventKindWorkflow},
		PayloadTypes: []model.TicketEventPayloadType{model.TicketEventPayloadTypeWorkflow},
		Types:        []string{string(model.WorkflowEventRunning)},
	})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, wfIter)

	workflowEvents := collectEvents(t, ctx, wfIter)
	require.Len(t, workflowEvents, 1)
	require.Equal(t, second.ID, workflowEvents[0].ID)
	require.Equal(t, model.TicketEventPayloadTypeWorkflow, workflowEvents[0].PayloadType)
	require.NotNil(t, workflowEvents[0].Payload.Workflow)
	require.Equal(t, model.WorkflowEventRunning, workflowEvents[0].Payload.Workflow.Type)

	reset := &model.TicketReset{
		ID:        model.TicketResetID("rst12345678901234567890123"),
		TicketID:  ticketID,
		Actor:     model.Actor{Type: model.ActorTypeUser, User: &model.ActorUser{Email: "owner@example.com"}},
		Reason:    "cleanup",
		CreatedAt: now.Add(2 * time.Minute),
	}
	require.NoError(t, evtStore.MarkReset(ctx, ticketID, reset, []model.TicketEventID{second.ID}))

	activeIter, err := evtStore.ListByTicket(ctx, ticketID, model.TicketEventFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, activeIter)
	active := collectEvents(t, ctx, activeIter)
	require.Len(t, active, 1)
	require.Equal(t, first.ID, active[0].ID)

	withResetIter, err := evtStore.ListByTicket(ctx, ticketID, model.TicketEventFilter{IncludeReset: true})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, withResetIter)
	withReset := collectEvents(t, ctx, withResetIter)
	require.Len(t, withReset, 2)
	require.NotNil(t, withReset[1].ResetID)
	require.Equal(t, reset.ID, *withReset[1].ResetID)

	atBefore := now.Add(90 * time.Second)
	iterAtBefore, err := evtStore.ListByTicket(ctx, ticketID, model.TicketEventFilter{At: &atBefore})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iterAtBefore)
	snapshotBefore := collectEvents(t, ctx, iterAtBefore)
	require.Len(t, snapshotBefore, 2)
	require.Equal(t, second.ID, snapshotBefore[1].ID)

	atAfter := now.Add(3 * time.Minute)
	iterAtAfter, err := evtStore.ListByTicket(ctx, ticketID, model.TicketEventFilter{At: &atAfter})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iterAtAfter)
	snapshotAfter := collectEvents(t, ctx, iterAtAfter)
	require.Len(t, snapshotAfter, 1)
	require.Equal(t, first.ID, snapshotAfter[0].ID)
}

func collectEvents(t testing.TB, ctx context.Context, iter store.Iterator[*model.TicketEvent]) []*model.TicketEvent {
	t.Helper()
	var events []*model.TicketEvent
	for {
		evt, err := iter.Next(ctx)
		if errors.Is(err, store.ErrIteratorDone) {
			break
		}
		require.NoError(t, err)
		events = append(events, evt)
	}
	return events
}
