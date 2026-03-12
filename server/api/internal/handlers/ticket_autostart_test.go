package handlers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	serverdeps "github.com/colony-2/colony2/server/api/pkg/serverdeps"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	opsexport "github.com/colony-2/colony2/server/ops/pkg/export"
	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerexport "github.com/colony-2/colony2/server/recipe-worker/pkg/export"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	ticketop "github.com/colony-2/colony2/server/ticket/pkg/op"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/impl"
	directruntime "github.com/colony-2/swf-go/pkg/swf/runtime/direct"
	"github.com/stretchr/testify/require"
	"log/slog"
)

func TestCreateTicketAutoStartsRecipe(t *testing.T) {
	ctx := context.Background()
	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)
	db := pg.DB
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
	workset, err := compiler.NewRecipeWorker(deps, registry, nil)
	require.NoError(t, err)
	if _, ok := workset.TaskWorkers["ticket.manage:ticket.manage"]; !ok {
		t.Fatalf("workset missing ticket.manage:ticket.manage task, keys=%v", worksetTaskKeys(workset.TaskWorkers))
	}

	// Install PGWF schema for workflow engine
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, impl.InstallPGWF(ctx, sqlDB))

	// Start embedded Strata daemon
	strata, err := impl.StartEmbeddedStrata()
	require.NoError(t, err)
	defer strata.Shutdown()

	// Build embedded workflow engine
	taskWorkers := make([]swf.TaskWorker, 0, len(workset.TaskWorkers))
	for _, tw := range workset.TaskWorkers {
		taskWorkers = append(taskWorkers, tw)
	}
	swfRuntime, err := directruntime.NewFromConfig(pg.DSN(), strata.BaseURL, strata.APIKey)
	require.NoError(t, err)
	engine, err := swf.NewEngineBuilder().
		WithRuntime(swfRuntime).
		WithAwaitRecycleThreshold(5*time.Second).
		WithLogger(slog.Default()).
		WithMaxActive(100).
		PlusWorkers(workset.JobWorker, taskWorkers...).
		BuildEngine()
	require.NoError(t, err)

	// Start worker loops
	go engine.Run(ctx)

	// Embedded recipe provider
	provider, err := serverdeps.NewEmbeddedProvider()
	require.NoError(t, err)

	// Create a RecipeProjectProvider that uses the embedded provider
	recipeProvider := func(projectId string, recipeRef string) (*recipe.Recipe, error) {
		return provider.GetRecipe(recipeRef)
	}

	ticketSvc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      ticketStore,
		EventStore: eventStore,
		Projects:   projectSvc,
		Cells:      cellSvc,
		Engine:     engine,
		Recipes:    recipeProvider,
	})
	require.NoError(t, err)

	// Install testing stub to prevent the ticket.manage operation from creating
	// a new service instance (which would run migrations and deadlock)
	stub := ticketop.TestingStub{Service: ticketSvc}
	defer stub.Install()()

	// Seed project and cell
	proj, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "proj",
		GitRepoPath: repoRoot,
	})
	require.NoError(t, err)
	cellRecord, err := cellSvc.CreateCell(ctx, cell.CreateInput{
		Name:        "api",
		ProjectID:   proj.ID,
		WorkingPath: "api",
	})
	require.NoError(t, err)

	created, jobID, err := ticketSvc.CreateTicket(ctx, ticket.CreateInput{
		Cell:        core.CellName(cellRecord.Name),
		ProjectID:   proj.ID,
		Title:       "autostart",
		Stage:       ticket.Stage("open"),
		State:       ticket.StateWaitingUser,
		Description: "ensure recipe auto-starts",
		Actor:       ticket.NewUserActor("tester@example.com"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, jobID)

	// Fetch workflow event to learn the job id.
	iter, err := ticketSvc.ListEvents(ctx, created.ID, ticket.TicketEventFilter{})
	require.NoError(t, err)
	defer iter.Close(ctx)

	var workflowJobID string
	for {
		ev, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, ticket.ErrIteratorDone) {
				break
			}
			require.NoError(t, err)
		}
		if ev != nil && ev.WorkflowData.WorkflowID != "" {
			workflowJobID = string(ev.WorkflowData.WorkflowID)
			break
		}
	}
	require.NotEmpty(t, workflowJobID, "expected workflow event with job id")
	require.Equal(t, jobID, workflowJobID)

	// Wait for job completion
	require.NoError(t, swf.WaitForJobToComplete(ctx, 30*time.Second, swf.JobKey{TenantId: string(proj.ID), JobId: jobID}, engine))

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
