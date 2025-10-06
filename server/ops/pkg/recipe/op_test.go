package recipe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestMain(m *testing.M) {
	RegisterGitStateAdapter(testGitStateAdapter{})
	code := m.Run()
	ResetGitStateAdapter()
	os.Exit(code)
}

func TestRecipeOpMetadata(t *testing.T) {
	op := GetOp()
	md := op.GetMetadata()
	assert.Equal(t, "recipe", md.Type)
	assert.NotEmpty(t, md.Description)
	assert.Equal(t, "1.0.0", md.Version)
	assert.Equal(t, 35*time.Minute, md.DefaultTimeout)
}

func TestRecipeOpExecuteChildWorkflow(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	env.RegisterWorkflow(testChildWorkflow)

	baseRaw := makeRecipeBaseInputs(t)

	parentWorkflow := func(ctx workflow.Context) (map[string]interface{}, error) {
		retry := &temporal.RetryPolicy{MaximumAttempts: 2}
		out, err := execute(ops.Invocation{}, ctx, 2*time.Minute, retry, RecipeInput{
			Name:   "testChildWorkflow",
			Inputs: map[string]interface{}{"foo": "bar"},
			Raw:    cloneMapDeep(baseRaw),
		})
		if err != nil {
			return nil, err
		}
		return out.Outputs, nil
	}

	env.RegisterWorkflow(parentWorkflow)
	env.ExecuteWorkflow(parentWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, true, result["ok"])
	if m, ok := result["echo"].(map[string]interface{}); ok {
		assert.Equal(t, "bar", m["foo"])
	}
}

func TestRecipeOpSharedAsyncReturnsHandle(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	env.RegisterWorkflow(testChildWorkflow)

	baseRaw := makeRecipeBaseInputs(t)

	workflowFn := func(ctx workflow.Context) (RecipeOutput, error) {
		return execute(ops.Invocation{}, ctx, 0, nil, RecipeInput{
			Name:     "testChildWorkflow",
			GitState: gitStateSharedName,
			RunMode:  runModeAsyncName,
			Raw:      cloneMapDeep(baseRaw),
		})
	}

	env.RegisterWorkflow(workflowFn)
	env.ExecuteWorkflow(workflowFn)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result RecipeOutput
	require.NoError(t, env.GetWorkflowResult(&result))

	handle, ok := result.Outputs["async_handle"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, gitStateSharedName, handle["git_state"])
	require.NotEmpty(t, handle["workflow_id"])
	require.NotEmpty(t, handle["run_id"])
	ctxVal, ok := handle["context"].(map[string]interface{})
	require.True(t, ok)
	require.NotNil(t, ctxVal["git"])
	require.Nil(t, result.Context)
	require.Empty(t, result.GitPersistHash)
}

func TestRecipeOpDiscreteSyncProvidesContext(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	env.RegisterWorkflow(discreteContextInspector)

	baseRaw := makeRecipeBaseInputs(t)

	workflowFn := func(ctx workflow.Context) (map[string]interface{}, error) {
		out, err := execute(ops.Invocation{}, ctx, 0, nil, RecipeInput{
			Name:     "discreteContextInspector",
			GitState: gitStateDiscreteName,
			RunMode:  runModeSyncName,
			Raw:      cloneMapDeep(baseRaw),
		})
		if err != nil {
			return nil, err
		}
		return out.Outputs, nil
	}

	env.RegisterWorkflow(workflowFn)
	env.ExecuteWorkflow(workflowFn)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var outputs map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&outputs))
	assert.Equal(t, true, outputs["context_present"])
}

func TestRecipeOpDiscreteAsyncReturnsHandle(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	env.RegisterWorkflow(testChildWorkflow)

	baseRaw := makeRecipeBaseInputs(t)

	workflowFn := func(ctx workflow.Context) (map[string]interface{}, error) {
		out, err := execute(ops.Invocation{}, ctx, 0, nil, RecipeInput{
			Name:     "testChildWorkflow",
			GitState: gitStateDiscreteName,
			RunMode:  runModeAsyncName,
			Raw:      cloneMapDeep(baseRaw),
		})
		if err != nil {
			return nil, err
		}
		return out.Outputs, nil
	}

	env.RegisterWorkflow(workflowFn)
	env.ExecuteWorkflow(workflowFn)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var outputs map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&outputs))
	handle, ok := outputs["async_handle"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "testChildWorkflow", handle["recipe"])
	require.Equal(t, gitStateDiscreteName, handle["git_state"])
	require.NotEmpty(t, handle["workflow_id"])
	require.NotEmpty(t, handle["run_id"])
	ctxVal, ok := handle["context"].(map[string]interface{})
	require.True(t, ok)
	require.NotNil(t, ctxVal["git"])
}

// --- Helpers -----------------------------------------------------------------

func testChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"ok":   true,
		"echo": input["inputs"],
	}, nil
}

func discreteContextInspector(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	ctxMap, ok := input["context"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing context")
	}
	gitMap, ok := ctxMap["git"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing git context")
	}
	_, hasBase := gitMap["base_hash"]
	_, hasPersist := gitMap["persist_hash"]
	return map[string]interface{}{"context_present": hasBase && hasPersist}, nil
}

func makeRecipeBaseInputs(t *testing.T) map[string]interface{} {
	repoPath, baseHash := initGitRepo(t)
	contextMap := map[string]interface{}{
		"git": map[string]interface{}{
			"base_repo":     repoPath,
			"base_hash":     baseHash,
			"persist_hash":  baseHash,
			"previous_hash": baseHash,
		},
		"worktree":  repoPath,
		"blobstore": "file://" + filepath.ToSlash(repoPath),
		"ticketid":  "TEST-TICKET",
		"cellname":  "test-cell",
	}
	return map[string]interface{}{
		"basegitrepo":      repoPath,
		"basegithash":      baseHash,
		"ticketid":         "TEST-TICKET",
		"cellname":         "test-cell",
		"ticket_id":        "TEST-TICKET",
		"cell_name":        "test-cell",
		"context":          contextMap,
		"git_persist_hash": baseHash,
	}
}

func initGitRepo(t *testing.T) (string, string) {
	dir := t.TempDir()
	runGit(t, dir, "git", "init")
	configureGitAuthor(t, dir, "Test User", "test@example.com")
	writeFile(t, dir, "README.md", "seed\n")
	runGit(t, dir, "git", "add", ".")
	runGit(t, dir, "git", "commit", "-m", "init")
	hash := strings.TrimSpace(runGit(t, dir, "git", "rev-parse", "HEAD"))
	return dir, hash
}

func runGit(t *testing.T, dir string, args ...string) string {
	out, err := runGitWithCtx(context.Background(), dir, args...)
	require.NoError(t, err)
	return out
}

func runGitWithCtx(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func configureGitAuthor(t *testing.T, dir, name, email string) {
	runGit(t, dir, "git", "config", "user.name", name)
	runGit(t, dir, "git", "config", "user.email", email)
}

func writeFile(t *testing.T, dir, name, content string) {
	full := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func cloneMapDeep(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[k] = cloneMapDeep(val)
		case []interface{}:
			dst[k] = cloneSliceDeep(val)
		default:
			dst[k] = val
		}
	}
	return dst
}

func cloneSliceDeep(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}
	dst := make([]interface{}, len(src))
	for i, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[i] = cloneMapDeep(val)
		case []interface{}:
			dst[i] = cloneSliceDeep(val)
		default:
			dst[i] = val
		}
	}
	return dst
}

// testGitStateAdapter provides a minimal git workspace implementation for unit tests.
type testGitStateAdapter struct{}

func (testGitStateAdapter) WithInlineWorkspace(ctx workflow.Context, inv ops.Invocation, inputs map[string]interface{}, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc) (*InlineWorkspaceResult, error) {
	childCtx, childInputs, err := testGitStateAdapter{}.PlanDetachedWorkspace(inv, inputs, DetachedWorkspaceOptions{ChildID: opts.ChildID})
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, fmt.Errorf("inline workspace function is required")
	}
	result, err := fn(ctx, childInputs)
	if err != nil {
		return nil, err
	}
	workspaceResult := &InlineWorkspaceResult{
		Result:     result,
		GitContext: childCtx,
	}
	if !opts.SkipFinalize {
		if ctxMap, ok := childInputs["context"].(map[string]interface{}); ok {
			workspaceResult.ContextMap = cloneMapDeep(ctxMap)
		}
	}
	return workspaceResult, nil
}

func (testGitStateAdapter) PlanDetachedWorkspace(inv ops.Invocation, inputs map[string]interface{}, _ DetachedWorkspaceOptions) (GitStateContext, map[string]interface{}, error) {
	childInputs := cloneMapDeep(inputs)
	if childInputs == nil {
		childInputs = make(map[string]interface{})
	}
	contextMap, _ := childInputs["context"].(map[string]interface{})
	if contextMap == nil {
		contextMap = make(map[string]interface{})
	} else {
		contextMap = cloneMapDeep(contextMap)
	}
	gitMap, _ := contextMap["git"].(map[string]interface{})
	if gitMap == nil {
		gitMap = make(map[string]interface{})
	} else {
		gitMap = cloneMapDeep(gitMap)
	}
	contextMap["git"] = gitMap
	childInputs["context"] = contextMap

	persistHash := stringValue(childInputs["git_persist_hash"])
	if persistHash == "" {
		persistHash = stringValue(gitMap["persist_hash"])
	}
	if persistHash == "" {
		persistHash = "test-persist-hash"
	}
	gitMap["persist_hash"] = persistHash

	if worktree := stringValue(contextMap["worktree"]); worktree != "" {
		gitMap["worktree_path"] = worktree
	}
	if blobstore := stringValue(contextMap["blobstore"]); blobstore != "" {
		gitMap["blob_store_uri"] = blobstore
	}
	ticket := stringValue(contextMap["ticketid"])
	cell := stringValue(contextMap["cellname"])

	childInputs["git_persist_hash"] = persistHash

	ctx := &testGitContext{
		persistHash: persistHash,
		worktree:    stringValue(gitMap["worktree_path"]),
		blobstore:   stringValue(gitMap["blob_store_uri"]),
		ticketID:    ticket,
		cellName:    cell,
		gitMap:      gitMap,
	}
	return ctx, childInputs, nil
}

type testGitContext struct {
	persistHash string
	worktree    string
	blobstore   string
	ticketID    string
	cellName    string
	gitMap      map[string]interface{}
}

func (c *testGitContext) ToMap() map[string]interface{} {
	return cloneMapDeep(c.gitMap)
}

func (c *testGitContext) GetPersistHash() string { return c.persistHash }

func (c *testGitContext) GetWorktreePath() string { return c.worktree }

func (c *testGitContext) GetBlobStoreURI() string { return c.blobstore }

func (c *testGitContext) GetTicketID() string { return c.ticketID }

func (c *testGitContext) GetCellName() string { return c.cellName }

func stringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}
