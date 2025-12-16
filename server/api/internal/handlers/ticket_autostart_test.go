package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/api/internal/recipes"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	opsexport "github.com/colony-2/colony2/server/ops/pkg/export"
	workerexport "github.com/colony-2/colony2/server/recipe-worker/pkg/export"
	ticketop "github.com/colony-2/colony2/server/ticket/pkg/op"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateTicketAutoStartsRecipe(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// Domain services
	projectStore, err := project.NewStore(db)
	require.NoError(t, err)
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	require.NoError(t, err)

	cellStore, err := cell.NewStore(db)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	require.NoError(t, err)

	ticketStore, err := ticket.NewStore(db)
	require.NoError(t, err)
	eventStore, err := ticket.NewEventStore(db)
	require.NoError(t, err)

	// Register ops for the worker.
	ops.Clear()
	ops.Register(opsexport.GetAll()...)
	ops.Register(workerexport.GetAll()...)
	ops.Register(ticketop.GetOp())
	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	deps := ops.NewServiceDepsBuilder().WithDatabase(db).Build()
	registry.SetDependencies(deps)
	workset, err := compiler.NewRecipeWorker(deps, registry)
	require.NoError(t, err)
	engine := toy.NewToyEngine([]swf.WorkSet{*workset})

	// Embedded recipe provider
	provider, err := recipes.NewEmbeddedProvider()
	require.NoError(t, err)

	ticketSvc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      ticketStore,
		EventStore: eventStore,
		Projects:   projectSvc,
		Cells:      cellSvc,
		Engine:     engine,
		Recipes:    provider,
	})
	require.NoError(t, err)

	// Seed project and cell
	proj, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "proj",
		GitRepoPath: "/repo",
	})
	require.NoError(t, err)
	cellRecord, err := cellSvc.CreateCell(ctx, cell.CreateInput{
		Name:        "api",
		ProjectID:   proj.ID,
		WorkingPath: "/repo/api",
	})
	require.NoError(t, err)

	created, err := ticketSvc.CreateTicket(ctx, ticket.CreateInput{
		Cell:        core.CellName(cellRecord.Name),
		ProjectID:   proj.ID,
		Title:       "autostart",
		Stage:       ticket.Stage("triage"),
		State:       ticket.StateWaitingUser,
		Description: "ensure recipe auto-starts",
		Actor:       ticket.NewUserActor("tester@example.com"),
	})
	require.NoError(t, err)

	// Fetch workflow event to learn the job id.
	iter, err := ticketSvc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{})
	require.NoError(t, err)
	defer iter.Close(ctx)

	var jobID string
	for {
		ev, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, ticket.ErrIteratorDone) {
				break
			}
			require.NoError(t, err)
		}
		if ev != nil && ev.WorkflowData.WorkflowID != "" {
			jobID = string(ev.WorkflowData.WorkflowID)
			break
		}
	}
	require.NotEmpty(t, jobID, "expected workflow event with job id")

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var resultErr error
	for ctxTimeout.Err() == nil {
		status, _ := engine.CheckJobStatus(ctxTimeout, swf.JobId(jobID))
		if status == swf.JobStatusCompleted {
			_, resultErr = engine.GetJobResult(ctxTimeout, swf.JobId(jobID))
			break
		}
		if status == swf.JobStatusCancelled {
			resultErr = fmt.Errorf("job ended with status %s", status)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, resultErr)

	updated, err := ticketSvc.GetTicketAt(ctx, created.ID, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, ticket.CompletedStage, updated.Stage)
}
