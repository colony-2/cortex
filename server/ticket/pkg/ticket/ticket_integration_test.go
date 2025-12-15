package ticket_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/testutil"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type stepClock struct{ current time.Time }

func (s *stepClock) Now() time.Time {
	now := s.current
	s.current = s.current.Add(time.Second)
	return now
}

func createProject(t *testing.T, db *gorm.DB, name string) (project.Service, project.ID) {
	t.Helper()
	store, err := project.NewStore(db)
	require.NoError(t, err)
	svc, err := project.NewService(project.ServiceConfig{Store: store})
	require.NoError(t, err)
	ctx := context.Background()
	proj, err := svc.CreateProject(ctx, project.CreateInput{
		Name:        name,
		GitRepoPath: "git@example.com/" + name + ".git",
	})
	require.NoError(t, err)
	return svc, proj.ID
}

type blockingEventIDGen struct {
	ids      []string
	blockAt  int
	wait     chan struct{}
	release  chan struct{}
	calls    int
	mu       sync.Mutex
	waitOnce sync.Once
}

func (g *blockingEventIDGen) NewID() (string, error) {
	g.mu.Lock()
	g.calls++
	block := g.blockAt > 0 && g.calls == g.blockAt
	if len(g.ids) == 0 {
		g.mu.Unlock()
		return "", errors.New("ticket: exhausted ids")
	}
	id := g.ids[0]
	g.ids = g.ids[1:]
	g.mu.Unlock()

	if block && g.wait != nil {
		g.waitOnce.Do(func() { close(g.wait) })
		if g.release != nil {
			<-g.release
		}
	}

	return id, nil
}

func TestServiceIntegration_CreateSearchUpdate(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	projSvc, projectID := createProject(t, pg.DB, "svc-create")

	ctx := context.Background()

	cellStore, err := cell.NewStore(pg.DB)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projSvc})
	require.NoError(t, err)
	_, err = cellSvc.CreateCell(ctx, cell.CreateInput{
		ProjectID:   projectID,
		Name:        "cell-a",
		WorkingPath: "/repo/cell-a",
	})
	require.NoError(t, err)

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 11, 10, 30, 0, 0, time.UTC)
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Projects:   projSvc,
		Cells:      cellSvc,
		Clock:      fixedClock{now: now},
		IDGen:      ticket.NewBase58Generator(ticket.DefaultIDLength),
		EventIDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
	})
	require.NoError(t, err)

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:      "cell-a",
		ProjectID: projectID,
		Title:     "Proposal review",
		Stage:     ticket.Stage("triage"),
		State:     ticket.StateWorking,
		Actor:     ticket.NewUserActor("designer@example.com"),
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
	require.Nil(t, tickets[0].LastResetID)
	require.Nil(t, tickets[0].LastResetAt)

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

	projSvc, projectID := createProject(t, pg.DB, "svc-events")

	ctx := context.Background()

	cellStore, err := cell.NewStore(pg.DB)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projSvc})
	require.NoError(t, err)
	_, err = cellSvc.CreateCell(ctx, cell.CreateInput{
		ProjectID:   projectID,
		Name:        "cell-a",
		WorkingPath: "/repo/cell-a",
	})
	require.NoError(t, err)

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 11, 12, 0, 0, 0, time.UTC)
	idGen := ticket.NewBase58Generator(ticket.DefaultIDLength)
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Projects:   projSvc,
		Cells:      cellSvc,
		Clock:      fixedClock{now: now},
		IDGen:      idGen,
		EventIDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
	})
	require.NoError(t, err)

	creator := ticket.NewUserActor("user@example.com")

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:      "cell-a",
		ProjectID: projectID,
		Title:     "Lifecycle",
		Stage:     ticket.Stage("triage"),
		State:     ticket.StateWorking,
		Actor:     creator,
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

	currentTicket, err := svc.GetTicketAt(ctx, created.ID, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, currentTicket.LastResetID)
	require.NotNil(t, currentTicket.LastResetAt)

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

func TestServiceIntegration_AppendDuringResetTagged(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	projSvc, projectID := createProject(t, pg.DB, "svc-reset")

	ctx := context.Background()

	cellStore, err := cell.NewStore(pg.DB)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projSvc})
	require.NoError(t, err)
	_, err = cellSvc.CreateCell(ctx, cell.CreateInput{
		ProjectID:   projectID,
		Name:        "cell-reset",
		WorkingPath: "/repo/cell-reset",
	})
	require.NoError(t, err)

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	wait := make(chan struct{})
	release := make(chan struct{})
	gen := &blockingEventIDGen{
		ids: []string{
			"evtA23456789012345678901234",
			"evtB23456789012345678901234",
			"rstC23456789012345678901234",
			"evtD23456789012345678901234",
		},
		blockAt: 2,
		wait:    wait,
		release: release,
	}

	now := time.Date(2024, 9, 12, 9, 0, 0, 0, time.UTC)
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Projects:   projSvc,
		Cells:      cellSvc,
		Clock:      fixedClock{now: now},
		IDGen:      ticket.NewBase58Generator(ticket.DefaultIDLength),
		EventIDGen: gen,
	})
	require.NoError(t, err)

	actor := ticket.NewUserActor("owner@example.com")

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:      "cell-reset",
		ProjectID: projectID,
		Title:     "Concurrent",
		Stage:     ticket.Stage("triage"),
		State:     ticket.StateWorking,
		Actor:     actor,
	})
	require.NoError(t, err)
	require.Nil(t, created.LastResetID)
	require.Nil(t, created.LastResetAt)

	changeEvent, err := svc.AppendMarkdownEvent(ctx, created.ID, ticket.MarkdownEventInput{
		Actor:     actor,
		EventTime: now,
		Payload: ticket.MarkdownDocEventPayload{
			Type: ticket.MarkdownDocAttached,
			Name: "design",
			Path: "docs/design.md",
		},
	})
	require.NoError(t, err)

	appendResult := make(chan struct {
		event *ticket.TicketEvent
		err   error
	}, 1)

	go func() {
		evt, err := svc.AppendWorkflowEvent(ctx, created.ID, ticket.WorkflowEventInput{
			Actor:     actor,
			EventTime: now.Add(5 * time.Minute),
			Payload: ticket.WorkflowEventPayload{
				Type:       ticket.WorkflowEventRunning,
				WorkflowID: ticket.WorkflowID("wf-concurrent"),
				RunID:      ticket.WorkflowRunID("run-concurrent"),
			},
		})
		appendResult <- struct {
			event *ticket.TicketEvent
			err   error
		}{event: evt, err: err}
	}()

	<-wait

	reset, err := svc.ResetTicket(ctx, created.ID, ticket.TicketResetInput{
		Actor:  actor,
		Reason: "concurrent reset",
	})
	require.NoError(t, err)
	release <- struct{}{}

	res := <-appendResult
	require.NoError(t, res.err)
	workflowEvt := res.event
	require.NotNil(t, workflowEvt)
	require.NotNil(t, workflowEvt.ResetID)
	require.Equal(t, reset.ID, *workflowEvt.ResetID)
	require.True(t, workflowEvt.EventTime.After(reset.CreatedAt) || workflowEvt.EventTime.Equal(reset.CreatedAt))

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
	require.Equal(t, ticket.TicketEventPayloadTypeReset, active[0].PayloadType)

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
	require.Equal(t, changeEvent.ID, all[0].ID)
	require.Equal(t, ticket.TicketEventPayloadTypeMarkdownDoc, all[0].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeReset, all[1].PayloadType)
	require.Equal(t, ticket.TicketEventPayloadTypeWorkflow, all[2].PayloadType)
	require.NotNil(t, all[2].ResetID)
	require.Equal(t, reset.ID, *all[2].ResetID)
	require.NotNil(t, all[0].ResetID)
	require.Equal(t, reset.ID, *all[0].ResetID)

	current, err := svc.GetTicketAt(ctx, created.ID, now.Add(10*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, current.LastResetID)
	require.NotNil(t, current.LastResetAt)
}

func TestServiceIntegration_ResetTicketRestoresSlice(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	projSvc, projectID := createProject(t, pg.DB, "svc-reset-slice")

	ctx := context.Background()

	cellStore, err := cell.NewStore(pg.DB)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projSvc})
	require.NoError(t, err)
	_, err = cellSvc.CreateCell(ctx, cell.CreateInput{
		ProjectID:   projectID,
		Name:        "cell-stage",
		WorkingPath: "/repo/cell-stage",
	})
	require.NoError(t, err)

	store, err := ticket.NewStore(pg.DB)
	require.NoError(t, err)

	eventStore, err := ticket.NewEventStore(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 12, 8, 0, 0, 0, time.UTC)
	clock := &stepClock{current: now}
	svc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      store,
		EventStore: eventStore,
		Projects:   projSvc,
		Cells:      cellSvc,
		Clock:      clock,
		IDGen:      ticket.NewBase58Generator(ticket.DefaultIDLength),
		EventIDGen: ticket.NewBase58Generator(ticket.DefaultIDLength),
	})
	require.NoError(t, err)

	creator := ticket.NewUserActor("stage@example.com")

	created, err := svc.CreateTicket(ctx, ticket.CreateInput{
		Cell:      "cell-stage",
		ProjectID: projectID,
		Title:     "Stage reset",
		Stage:     ticket.Stage("triage"),
		State:     ticket.StateWorking,
		Actor:     creator,
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
	require.NotNil(t, stateBeforeReset.LastResetID)
	require.NotNil(t, stateBeforeReset.LastResetAt)

	afterResetTime := resetEvent.EventTime.Add(2 * time.Microsecond)
	stateAfterReset, err := svc.GetTicketAt(ctx, created.ID, afterResetTime)
	require.NoError(t, err)
	require.Equal(t, ticket.CompletedStage, stateAfterReset.Stage)
	require.NotNil(t, stateAfterReset.LastResetID)
	require.NotNil(t, stateAfterReset.LastResetAt)
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
