package recipe

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/testsuite"
    "go.temporal.io/sdk/workflow"
)

// TestRecipeOpMetadata verifies the op metadata returned by GetOp()
func TestRecipeOpMetadata(t *testing.T) {
    op := GetOp()
    md := op.GetMetadata()
    assert.Equal(t, "recipe", md.Type)
    assert.NotEmpty(t, md.Description)
    assert.Equal(t, "1.0.0", md.Version)
    assert.Equal(t, 35*time.Minute, md.DefaultTimeout)
}

// child workflow used by tests
func testChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
    return map[string]interface{}{
        "ok": true,
        "echo": input["inputs"],
    }, nil
}

// parent workflow that invokes the inline op execute
func parentInvokeRecipe(ctx workflow.Context) (map[string]interface{}, error) {
    // Call the inline execute with a retry policy and timeout
    retry := &temporal.RetryPolicy{MaximumAttempts: 2}
    out, err := execute(ctx, 2*time.Minute, retry, RecipeInput{
        Name:   "testChildWorkflow",
        Inputs: map[string]interface{}{"foo": "bar"},
    })
    if err != nil {
        return nil, err
    }
    return out.Outputs, nil
}

func TestRecipeOpExecuteChildWorkflow(t *testing.T) {
    ts := &testsuite.WorkflowTestSuite{}
    env := ts.NewTestWorkflowEnvironment()

    env.RegisterWorkflow(testChildWorkflow)
    env.RegisterWorkflow(parentInvokeRecipe)

    env.ExecuteWorkflow(parentInvokeRecipe)
    require.True(t, env.IsWorkflowCompleted())
    require.NoError(t, env.GetWorkflowError())

    var result map[string]interface{}
    require.NoError(t, env.GetWorkflowResult(&result))
    assert.Equal(t, true, result["ok"])
    // Ensure inputs propagated
    if m, ok := result["echo"].(map[string]interface{}); ok {
        assert.Equal(t, "bar", m["foo"])
    }
}
