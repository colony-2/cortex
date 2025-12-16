package handlers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/api/internal/recipes"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	opsexport "github.com/colony-2/colony2/server/ops/pkg/export"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerexport "github.com/colony-2/colony2/server/recipe-worker/pkg/export"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	ticketop "github.com/colony-2/colony2/server/ticket/pkg/op"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"os"
)

func TestCreateTicketAutoStartsRecipe(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA journal_mode=WAL").Error)
	require.NoError(t, db.Exec("PRAGMA busy_timeout=5000").Error)
	repoRoot := t.TempDir()
	worktree := repoRoot + "/api"
	require.NoError(t, os.MkdirAll(worktree, 0o755))
	runGit(t, "init", "-q", repoRoot)
	runGit(t, "-C", repoRoot, "config", "user.email", "test@example.com")
	runGit(t, "-C", repoRoot, "config", "user.name", "Test User")
	runGit(t, "-C", repoRoot, "checkout", "-q", "-b", "main")
	runGit(t, "-C", repoRoot, "commit", "--allow-empty", "-m", "init")

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
	all := registry.GetAll()
	if _, ok := all["ticket.manage:ticket.manage"]; !ok {
		t.Fatalf("expected ticket.manage:ticket.manage to be registered, got keys: %v", keys(all))
	}
	deps := ops.NewServiceDepsBuilder().WithDatabase(db).Build()
	registry.SetDependencies(deps)
	workset, err := compiler.NewRecipeWorker(deps, registry)
	require.NoError(t, err)
	if _, ok := workset.TaskWorkers["ticket.manage:ticket.manage"]; !ok {
		t.Fatalf("workset missing ticket.manage:ticket.manage task, keys=%v", worksetTaskKeys(workset.TaskWorkers))
	}
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
		GitRepoPath: repoRoot,
	})
	require.NoError(t, err)
	cellRecord, err := cellSvc.CreateCell(ctx, cell.CreateInput{
		Name:        "api",
		ProjectID:   proj.ID,
		WorkingPath: worktree,
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

func keys(m map[string]workerops.ActivityRegistration) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func worksetTaskKeys(m map[string]swf.TaskWorker) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v; output=%s", args, err, out)
	}
}
