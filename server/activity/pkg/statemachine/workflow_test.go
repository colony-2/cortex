package statemachine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// TestStateMachineWorkflowSuite tests state machine using Temporal's WorkflowTestSuite
func TestStateMachineWorkflowSuite(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create executor and activity
	executor := NewDefaultExecutor()
	
	// Register mock activities in executor
	executor.RegisterActivity("critique_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		score := 85
		return map[string]interface{}{
			"score":    score,
			"feedback": "Good document",
		}, nil
	})
	
	executor.RegisterActivity("improve_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"improved": true,
			"content":  "Improved document",
		}, nil
	})
	
	// Create state machine activity
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	// Register the workflow and activity
	env.RegisterWorkflow(StateMachineTestWorkflow)
	env.RegisterActivity(activity.Execute)
	
	// Mock the activity execution
	env.OnActivity(activity.Execute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
			// Execute the actual state machine logic
			return activity.Execute(ctx, config, inputs)
		},
	)
	
	// Define state machine configuration
	config := StateMachineConfig{
		InitialState: "reviewing",
		States: map[string]StateDefinition{
			"reviewing": {
				Activity: "critique_activity",
				Transitions: []TransitionSpec{
					{To: "approved", When: ".Outputs.score >= 80"},
					{To: "rejected", When: ".Outputs.score < 80"},
				},
			},
			"approved": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"status": "approved",
				},
			},
			"rejected": {
				Terminal: true,
				Error: "Document rejected",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"document": "test document",
	}
	
	// Execute workflow
	env.ExecuteWorkflow(StateMachineTestWorkflow, config, inputs)
	
	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	// Get and verify the result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	assert.Equal(t, "approved", result["status"])
}

// StateMachineTestWorkflow is a simple workflow that executes a state machine
func StateMachineTestWorkflow(ctx workflow.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Set up activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	
	// Create executor and activity
	executor := NewDefaultExecutor()
	activity, err := NewStateMachineActivity(executor)
	if err != nil {
		return nil, err
	}
	
	// Execute the state machine activity
	var result map[string]interface{}
	err = workflow.ExecuteActivity(ctx, activity.Execute, config, inputs).Get(ctx, &result)
	return result, err
}

// TestStateMachineRetryWorkflow tests retry behavior in Temporal
func TestStateMachineRetryWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	attempts := 0
	
	// Create executor and activity
	executor := NewDefaultExecutor()
	executor.RegisterActivity("flaky_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		attempts++
		if attempts < 2 {
			return map[string]interface{}{"score": 60}, nil
		}
		return map[string]interface{}{"score": 85}, nil
	})
	
	executor.RegisterActivity("improve_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"improved": true}, nil
	})
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	// Register workflow and activity
	env.RegisterWorkflow(StateMachineTestWorkflow)
	env.RegisterActivity(activity.Execute)
	
	// Mock the activity
	env.OnActivity(activity.Execute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
			return activity.Execute(ctx, config, inputs)
		},
	)
	
	config := StateMachineConfig{
		InitialState: "reviewing",
		States: map[string]StateDefinition{
			"reviewing": {
				Activity: "flaky_activity",
				Transitions: []TransitionSpec{
					{To: "approved", When: ".Outputs.score >= 80"},
					{To: "improving", When: ".Outputs.score < 80 && .State.Attempts < 3"},
					{To: "rejected", When: ".State.Attempts >= 3"},
				},
			},
			"improving": {
				Activity: "improve_activity",
				Transitions: []TransitionSpec{
					{To: "reviewing", When: ".Outputs.improved == true"},
				},
			},
			"approved": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"status": "approved",
				},
			},
			"rejected": {
				Terminal: true,
				Error: "Review failed",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"document": "test document",
	}
	
	env.ExecuteWorkflow(StateMachineTestWorkflow, config, inputs)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "approved", result["status"])
}

// TestNestedStateMachineWorkflow tests nested state machines
func TestNestedStateMachineWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Create executors and activities for both levels
	outerExecutor := NewDefaultExecutor()
	innerExecutor := NewDefaultExecutor()
	
	// Inner state machine mock
	innerExecutor.RegisterActivity("validate_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"valid": true}, nil
	})
	
	innerActivity, err := NewStateMachineActivity(innerExecutor)
	require.NoError(t, err)
	
	// Register inner state machine as an activity in outer executor
	outerExecutor.RegisterActivity("inner_state_machine", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		innerConfig := StateMachineConfig{
			InitialState: "validate",
			States: map[string]StateDefinition{
				"validate": {
					Activity: "validate_activity",
					Transitions: []TransitionSpec{
						{To: "valid", When: ".Outputs.valid == true"},
						{To: "invalid", When: ".Outputs.valid == false"},
					},
				},
				"valid": {
					Terminal: true,
					Outputs: map[string]interface{}{
						"validation_status": "passed",
					},
				},
				"invalid": {
					Terminal: true,
					Outputs: map[string]interface{}{
						"validation_status": "failed",
					},
				},
			},
		}
		return innerActivity.Execute(ctx, innerConfig, inputs)
	})
	
	outerActivity, err := NewStateMachineActivity(outerExecutor)
	require.NoError(t, err)
	
	// Register workflow and activity
	env.RegisterWorkflow(StateMachineTestWorkflow)
	env.RegisterActivity(outerActivity.Execute)
	
	env.OnActivity(outerActivity.Execute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
			return outerActivity.Execute(ctx, config, inputs)
		},
	)
	
	// Outer state machine config
	config := StateMachineConfig{
		InitialState: "phase1",
		States: map[string]StateDefinition{
			"phase1": {
				Activity: "inner_state_machine",
				Transitions: []TransitionSpec{
					{To: "complete", When: ".Outputs.validation_status == \"passed\""},
					{To: "failed", When: ".Outputs.validation_status == \"failed\""},
				},
			},
			"complete": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"result": "success",
				},
			},
			"failed": {
				Terminal: true,
				Error: "Validation failed",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"data": "test data",
	}
	
	env.ExecuteWorkflow(StateMachineTestWorkflow, config, inputs)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "success", result["result"])
}