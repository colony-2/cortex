//go:build temporal
// +build temporal

package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// TestInvestigateStateOutputStorage investigates what's happening with state output storage
func TestInvestigateStateOutputStorage(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	// The registry already has echo_activity registered, enable it in the worker
	registry.EnableActivitiesInWorker(env)

	// Simple state machine with one terminal state
	stateMap := &recipe.StateMap{
		Initial: "simple_state",
		States: map[string]recipe.State{
			"simple_state": {
				Node: recipe.Node{
					NodeImpl: &recipe.NodeOp{
						NodeMetadata: recipe.NodeMetadata{
							ID: "simple_state",
							Inputs: map[string]interface{}{
								"message": "Hello test",
							},
						},
						OpData: recipe.OpData{
							Op: "echo_activity", // Use the test activity that exists
						},
					},
				},
				SingleStateMetadata: recipe.SingleStateMetadata{
					Transitions: []recipe.Transition{}, // Terminal state
				},
			},
		},
	}

	baseInputs := withRequiredGitInputs(nil)
	worktree := filepath.Join(os.TempDir(), "state-investigation-worktree")
	blobStore := "file://" + filepath.Join(os.TempDir(), "state-investigation-blobstore")
	inputs := map[string]interface{}{
		"test_input":  "test_value",
		"basegitrepo": baseInputs["basegitrepo"],
		"basegithash": baseInputs["basegithash"],
		"ticketid":    baseInputs["ticketid"],
		"cellname":    baseInputs["cellname"],
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":    baseInputs["basegitrepo"],
				"base_hash":    baseInputs["basegithash"],
				"persist_hash": baseInputs["basegithash"],
			},
			"worktree":  worktree,
			"blobstore": blobStore,
			"ticketid":  baseInputs["ticketid"],
			"cellname":  baseInputs["cellname"],
			"recipe": map[string]interface{}{
				"id":              "test-investigation",
				"workflow_id":     "default-test-workflow-id",
				"workflow_run_id": "default-test-run-id",
			},
		},
	}
	inputs["ticket_id"] = baseInputs["ticketid"]
	inputs["cell_name"] = baseInputs["cellname"]

	// Execute and capture what happens
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		// Create resolution context
		resCtx, err := NewResolutionContext("state_machine", "test-sm")
		if err != nil {
			return nil, err
		}
		resCtx.TemplateData.Inputs = inputs

		// Execute the state
		tracker := newInvocationTracker(recipe.RecipeMetadata{NodeMetadata: recipe.NodeMetadata{ID: "test-investigation"}}, nil)
		state := stateMap.States["simple_state"]

		// This should execute the activity and return outputs
		stateOutputs, err := executeStateNode(ctx, registry, tracker, &state.Node, resCtx, "simple_state")
		if err != nil {
			return nil, err
		}

		t.Logf("executeStateNode returned: %+v", stateOutputs)

		// Store outputs like the state machine does
		resCtx.AddStateOutput("simple_state", stateOutputs)

		// Check what's stored
		if storedState, ok := resCtx.TemplateData.States["simple_state"]; ok {
			t.Logf("Stored state outputs: %+v", storedState.Outputs)
		} else {
			t.Log("No state stored!")
		}

		// Try to retrieve like the state machine does
		if lastState, ok := resCtx.TemplateData.States["simple_state"]; ok {
			t.Logf("Retrieved outputs: %+v", lastState.Outputs)
			if lastState.Outputs != nil && len(lastState.Outputs) > 0 {
				return lastState.Outputs, nil
			}
		}

		return map[string]interface{}{"error": "no outputs found"}, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err = env.GetWorkflowResult(&result)
	require.NoError(t, err)

	t.Logf("Final result: %+v", result)

	// We expect to see the echo activity outputs
	if _, hasError := result["error"]; hasError {
		t.Error("Failed to retrieve stored outputs!")
	}
}
