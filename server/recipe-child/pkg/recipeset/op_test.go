package recipeset

import (
	"os"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestMain(m *testing.M) {
	recipe.RegisterGitStateAdapter(testGitStateAdapter{})
	code := m.Run()
	recipe.ResetGitStateAdapter()
	os.Exit(code)
}

func TestRecipeSetOpMetadata(t *testing.T) {
	op := GetOp()
	md := op.GetMetadata()
	assert.Equal(t, "recipe_set", md.Type)
	assert.NotEmpty(t, md.Description)
	assert.Equal(t, "1.0.0", md.Version)
	assert.Equal(t, 35*time.Minute, md.DefaultTimeout)
}

func TestRecipeSetExecutesChildrenAndReturnsNil(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	var childACalled, childBCalled bool

	env.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		childACalled = true
		return map[string]interface{}{"status": "ok", "input": input}, nil
	}, workflow.RegisterOptions{Name: "childA"})

	env.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		childBCalled = true
		return map[string]interface{}{"status": "ok", "input": input}, nil
	}, workflow.RegisterOptions{Name: "childB"})

	base := makeBaseInputs()

	parentWorkflow := func(ctx workflow.Context) (Output, error) {
		in := Input{
			Recipes: []map[string]interface{}{
				{
					"recipe": "childA",
					"inputs": map[string]interface{}{"value": "alpha"},
				},
				{
					"recipe": "childB",
					"inputs": map[string]interface{}{"value": "beta"},
				},
			},
			Raw: cloneMapDeep(base),
		}
		return execute(ops.Invocation{}, ctx, 0, nil, in)
	}

	env.RegisterWorkflow(parentWorkflow)
	env.ExecuteWorkflow(parentWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var out Output
	require.NoError(t, env.GetWorkflowResult(&out))
	assert.True(t, childACalled)
	assert.True(t, childBCalled)
}

func TestRecipeSetAggregatesFailures(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	env.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"status": "ok"}, nil
	}, workflow.RegisterOptions{Name: "willSucceed"})

	env.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return nil, temporal.NewNonRetryableApplicationError("boom", "BOOM", nil, map[string]interface{}{"payload": input})
	}, workflow.RegisterOptions{Name: "willFail"})

	base := makeBaseInputs()

	parentWorkflow := func(ctx workflow.Context) (Output, error) {
		in := Input{
			Recipes: []map[string]interface{}{
				{"recipe": "willSucceed", "inputs": map[string]interface{}{"value": "ok"}},
				{"recipe": "willFail", "inputs": map[string]interface{}{"value": "nope"}},
			},
			Raw: cloneMapDeep(base),
		}
		return execute(ops.Invocation{}, ctx, 0, nil, in)
	}

	env.RegisterWorkflow(parentWorkflow)
	env.ExecuteWorkflow(parentWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	err := env.GetWorkflowError()
	require.Error(t, err)

	var appErr *temporal.ApplicationError
	require.True(t, temporal.IsApplicationError(err))
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "RECIPE_SET_FAILURE", appErr.Type())

	var details []map[string]interface{}
	require.NoError(t, appErr.Details(&details))
	require.Len(t, details, 2)

	assert.Equal(t, float64(0), details[0]["index"])
	assert.Equal(t, "willSucceed", details[0]["recipe"])
	assert.Equal(t, "succeeded", details[0]["status"])

	assert.Equal(t, float64(1), details[1]["index"])
	assert.Equal(t, "willFail", details[1]["recipe"])
	assert.Equal(t, "failed", details[1]["status"])
	errMap, ok := details[1]["error"].(map[string]interface{})
	require.True(t, ok)
	message, ok := errMap["message"].(string)
	require.True(t, ok)
	assert.Contains(t, message, "boom")
}

func TestRecipeSetValidatesRecipeList(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	base := makeBaseInputs()

	parentWorkflow := func(ctx workflow.Context) (Output, error) {
		return execute(ops.Invocation{}, ctx, 0, nil, Input{Raw: cloneMapDeep(base)})
	}

	env.RegisterWorkflow(parentWorkflow)
	env.ExecuteWorkflow(parentWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	err := env.GetWorkflowError()
	require.Error(t, err)
	assert.True(t, temporal.IsApplicationError(err))
}

// --- test git adapter --------------------------------------------------------

type testGitStateAdapter struct{}

func (testGitStateAdapter) WithInlineWorkspace(ctx workflow.Context, inv ops.Invocation, inputs map[string]interface{}, opts recipe.InlineWorkspaceOptions, fn recipe.InlineWorkspaceFunc) (*recipe.InlineWorkspaceResult, error) {
	childCtx, childInputs, err := testGitStateAdapter{}.PlanDetachedWorkspace(inv, inputs, recipe.DetachedWorkspaceOptions{})
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, temporal.NewNonRetryableApplicationError("inline workspace function required", "TEST_ADAPTER", nil)
	}
	result, err := fn(ctx, childInputs)
	if err != nil {
		return nil, err
	}
	ctxMap, _ := childInputs["context"].(map[string]interface{})
	return &recipe.InlineWorkspaceResult{
		Result:     result,
		ContextMap: cloneMapDeep(ctxMap),
		GitContext: childCtx,
	}, nil
}

func (testGitStateAdapter) PlanDetachedWorkspace(inv ops.Invocation, inputs map[string]interface{}, _ recipe.DetachedWorkspaceOptions) (recipe.GitStateContext, map[string]interface{}, error) {
	clone := cloneMapDeep(inputs)
	if clone == nil {
		clone = make(map[string]interface{})
	}
	contextMap := ensureContextMap(clone)
	gitMap := ensureGitMap(contextMap)

	persist := stringValue(clone["git_persist_hash"])
	if persist == "" {
		persist = stringValue(gitMap["persist_hash"])
	}
	if persist == "" {
		persist = "test-persist"
	}
	gitMap["persist_hash"] = persist
	clone["git_persist_hash"] = persist

	contextMap["git"] = gitMap
	clone["context"] = contextMap

	ctx := &testGitContext{
		persistHash: persist,
		worktree:    stringValue(contextMap["worktree"]),
		blobstore:   stringValue(contextMap["blobstore"]),
		ticketID:    stringValue(contextMap["ticketid"]),
		cellName:    stringValue(contextMap["cellname"]),
		gitMap:      cloneMapDeep(gitMap),
	}
	return ctx, clone, nil
}

type testGitContext struct {
	persistHash string
	worktree    string
	blobstore   string
	ticketID    string
	cellName    string
	gitMap      map[string]interface{}
}

func (c *testGitContext) ToMap() map[string]interface{} { return cloneMapDeep(c.gitMap) }
func (c *testGitContext) GetPersistHash() string        { return c.persistHash }
func (c *testGitContext) GetWorktreePath() string       { return c.worktree }
func (c *testGitContext) GetBlobStoreURI() string       { return c.blobstore }
func (c *testGitContext) GetTicketID() string           { return c.ticketID }
func (c *testGitContext) GetCellName() string           { return c.cellName }

// --- helpers -----------------------------------------------------------------

func makeBaseInputs() map[string]interface{} {
	return map[string]interface{}{
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":    "file:///tmp/repo",
				"base_hash":    "abcdef",
				"persist_hash": "abcdef",
			},
			"worktree":  "/tmp/repo",
			"blobstore": "file:///tmp/repo",
			"ticketid":  "TEST-1",
			"cellname":  "cell-a",
		},
		"git_persist_hash": "abcdef",
		"basegitrepo":      "file:///tmp/repo",
		"basegithash":      "abcdef",
		"ticketid":         "TEST-1",
		"cellname":         "cell-a",
	}
}

func ensureContextMap(inputs map[string]interface{}) map[string]interface{} {
	ctx, _ := inputs["context"].(map[string]interface{})
	if ctx == nil {
		ctx = make(map[string]interface{})
	} else {
		ctx = cloneMapDeep(ctx)
	}
	return ctx
}

func ensureGitMap(ctx map[string]interface{}) map[string]interface{} {
	git, _ := ctx["git"].(map[string]interface{})
	if git == nil {
		git = make(map[string]interface{})
	} else {
		git = cloneMapDeep(git)
	}
	ctx["git"] = git
	return git
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
