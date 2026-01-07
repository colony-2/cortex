package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
)

type jobIDGen struct {
	count int
	max   int
}

func (g *jobIDGen) Generate(tenantID string) (swf.JobKey, error) {
	g.count++
	if g.count > g.max {
		return swf.JobKey{}, fmt.Errorf("too many jobs")
	}
	return swf.JobKey{TenantId: tenantID, JobId: fmt.Sprintf("job-%d", g.count)}, nil
}

func TestCodexOpToyEngine(t *testing.T) {
	originalOps := coreops.List()
	t.Cleanup(func() {
		coreops.Clear()
		if len(originalOps) > 0 {
			coreops.Register(originalOps...)
		}
	})
	coreops.Clear()
	coreops.Register(GetOp())

	worktree := t.TempDir()
	cellRel := filepath.Join("cells", "alpha")

	tempDir := t.TempDir()
	stdoutPath := filepath.Join(tempDir, "stdout.jsonl")
	stderrPath := filepath.Join(tempDir, "stderr.txt")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("{\"hello\":\"world\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte(""), 0o644))

	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		return Result{
			Status:              StatusCompleted,
			SessionID:           "sess-123",
			AssistantSummary:    "all good",
			PendingDependencies: []Dependency{},
		}, stdoutPath, stderrPath, tempDir, nil
	}
	t.Cleanup(func() { executeLibrary = Execute })

	recipeYaml := fmt.Sprintf(`
---
id: codex-op-toy
op: codex.exec
inputs:
  prompt: "do something"
  worktree_path: %q
  cell_relative_path: %q
`, worktree, cellRel)
	testRecipe, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(t, err)

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	g := jobIDGen{max: 1}
	engine := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))

	wf := workflow.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().WithWorkflowControl(&wf).Build()
	workSet, err := compiler.NewRecipeWorker(deps, registry)
	require.NoError(t, err)
	workSet.JobWorker = &artifactValidatingJobWorker{inner: workSet.JobWorker, t: t}
	require.NoError(t, engine.RegisterWorkers(workSet))

	jobCtx, gitCtx := compiler.GenerateTestContext()
	jobCtx.Environment.WorktreePath = worktree
	jobCtx.Workflow.CellName = cellRel
	start := workflowctl.StartJob{
		TenantId:   "test-tenant",
		RecipeName: testRecipe.GetMetadata().ID,
		Inputs:     map[string]interface{}{},
		JobContext: jobCtx,
		GitRef:     gitCtx.ParentRef,
	}

	key, err := starter.StartRecipeJob(context.Background(), start, engine, *testRecipe)
	require.NoError(t, err)

	res, err := engine.GetJobResult(context.Background(), key)
	require.NoError(t, err)

	raw, err := res.GetData()
	require.NoError(t, err)

	var output ExecOpOutput
	require.NoError(t, json.Unmarshal(raw, &output))
	require.Equal(t, string(StatusCompleted), output.Status)
	require.Equal(t, "sess-123", output.SessionID)
	require.Equal(t, "all good", output.AssistantSummary)
}

func TestCodexOpToyEngineCLIOutput(t *testing.T) {
	ensureCodexAvailable(t)
	t.Setenv("VIBETHIS_CODEX_USE_DIRECT", "1")

	originalOps := coreops.List()
	t.Cleanup(func() {
		coreops.Clear()
		if len(originalOps) > 0 {
			coreops.Register(originalOps...)
		}
	})
	coreops.Clear()
	coreops.Register(GetOp())

	worktreeRoot := t.TempDir()
	worktree := filepath.Join(worktreeRoot, "worktree")
	cellRel := filepath.Join("cells", "alpha")

	recipeYaml := fmt.Sprintf(`
---
id: codex-op-toy-cli
op: codex.exec
inputs:
  prompt: "Respond strictly with JSON matching the schema: status 'completed', assistantSummary 'All good', incompleteReason '', incompleteCategory '', errorMessage '', pendingDependencies []."
  worktree_path: %q
  cell_relative_path: %q
`, worktree, cellRel)
	testRecipe, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(t, err)

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	g := jobIDGen{max: 1}
	engine := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))

	wf := workflow.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().WithWorkflowControl(&wf).Build()
	workSet, err := compiler.NewRecipeWorker(deps, registry)
	require.NoError(t, err)
	require.NoError(t, engine.RegisterWorkers(workSet))

	jobCtx, gitCtx := compiler.GenerateTestContext()
	jobCtx.Environment.WorktreePath = worktree
	jobCtx.Workflow.CellName = cellRel
	start := workflowctl.StartJob{
		TenantId:   "test-tenant",
		RecipeName: testRecipe.GetMetadata().ID,
		Inputs:     map[string]interface{}{},
		JobContext: jobCtx,
		GitRef:     gitCtx.ParentRef,
	}

	key, err := starter.StartRecipeJob(context.Background(), start, engine, *testRecipe)
	require.NoError(t, err)

	res, err := engine.GetJobResult(context.Background(), key)
	require.NoError(t, err)

	raw, err := res.GetData()
	require.NoError(t, err)

	var output ExecOpOutput
	require.NoError(t, json.Unmarshal(raw, &output))
	if output.Status != string(StatusCompleted) {
		t.Fatalf("codex error: raw=%s", string(raw))
	}
	require.Equal(t, "All good", output.AssistantSummary)
}
