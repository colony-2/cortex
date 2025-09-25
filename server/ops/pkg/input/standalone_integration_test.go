package input_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	exportops "github.com/divisive-ai/vibethis/server/ops/pkg/export"
	inputpkg "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	recipepkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// TestFullInputActivityLifecycle tests the input op end-to-end with a real signal
func TestFullInputActivityLifecycle(t *testing.T) {
	// Register ops into recipe-core ops registry
	recipeops.Clear()
	recipeops.Register(exportops.GetAll()...)

	// Build a simple recipe that invokes the input op directly at root
	r := recipepkg.Recipe{
		RecipeImpl: &recipepkg.RecipeOp{
			RecipeMetadata: recipepkg.RecipeMetadata{
				Version:      "1.0.0",
				NodeMetadata: recipepkg.NodeMetadata{ID: "root"},
			},
			OpData: recipepkg.OpData{Op: "input"},
		},
	}

	// Create the registry and a Temporal test environment
	reg, err := workerops.NewActivityRegistry()
	require.NoError(t, err)

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	reg.EnableActivitiesInWorker(env)

	// Register workflow that runs the recipe via the compiler
	wf := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, reg, r, inputs)
	}
	env.RegisterWorkflowWithOptions(wf, workflow.RegisterOptions{Name: r.GetMetdata().ID})

	// Send a real user-response signal shortly after start
	recipeMeta := r.GetMetdata()
	inv := recipeops.Invocation{
		RecipeID:   recipeMeta.ID,
		NodePath:   recipeMeta.ID,
		InvokeSeq:  0,
		BoxID:      "test-cell",
		ActivityID: "approval-activity",
	}
	signalName := fmt.Sprintf("user-response:%s", inv.Hash())

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(signalName, inputpkg.UserResponseSignal{
			Fields: map[string]interface{}{"response": "approve"},
			UserID: "test-user",
		})
	}, 500*time.Millisecond)

	// Execute with inputs mapping to the input op struct fields
	inputs := map[string]interface{}{
		"box_id":      "test-cell",
		"activity_id": "approval-activity",
		"config": map[string]interface{}{
			"question": "Do you approve this deployment?",
			"type":     "multiple_choice",
			"options":  []map[string]interface{}{{"value": "approve"}, {"value": "reject"}},
			"timeout":  60,
		},
	}
	env.ExecuteWorkflow(r.GetMetdata().ID, inputs)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify the result from the input op
	var outputs map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&outputs))
	assert.Equal(t, "test-user", outputs["user_id"])

	// The inline input op returns responses under the "fields" map
	fields, ok := outputs["fields"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "approve", fields["response"])
}
