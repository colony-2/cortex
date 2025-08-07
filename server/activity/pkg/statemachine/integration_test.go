package statemachine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// StateMachineTestWorkflow for integration testing
func StateMachineTestWorkflow(ctx workflow.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
	// For testing, we'll execute the state machine as an activity
	var result map[string]interface{}
	
	// Create a local activity that runs the state machine
	activityFn := func(ctx context.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
		executor := NewDefaultExecutor()
		
		// Register mock activities in the executor
		executor.RegisterActivity("critique_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return MockCritiqueActivity(ctx, inputs)
		})
		executor.RegisterActivity("improve_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return MockImproveActivity(ctx, inputs)
		})
		
		// Create and execute state machine
		activity, err := NewStateMachineActivity(executor)
		if err != nil {
			return nil, err
		}
		
		return activity.Execute(ctx, config, inputs)
	}
	
	// Execute as a local activity with options
	localActivityOptions := workflow.LocalActivityOptions{
		ScheduleToCloseTimeout: 10 * time.Second,
	}
	ctx = workflow.WithLocalActivityOptions(ctx, localActivityOptions)
	err := workflow.ExecuteLocalActivity(ctx, activityFn, config, inputs).Get(ctx, &result)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

// Mock activities for testing
func MockCritiqueActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	score := 75
	if attempt, ok := inputs["attempt"].(int); ok && attempt > 1 {
		score = 85
	}
	return map[string]interface{}{
		"score":    score,
		"feedback": "Document reviewed",
	}, nil
}

func MockImproveActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"improved": true,
		"content":  "Improved document",
	}, nil
}

func TestStateMachineIntegration_SimpleWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Register activities
	env.RegisterActivity(MockCritiqueActivity)
	env.RegisterActivity(MockImproveActivity)
	
	config := StateMachineConfig{
		InitialState: "reviewing",
		States: map[string]StateDefinition{
			"reviewing": {
				Activity: "critique_activity",
				Transitions: []TransitionSpec{
					{To: "approved", When: ".Outputs.score >= 80"},
					{To: "improving", When: ".Outputs.score < 80"},
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

func TestStateMachineIntegration_NestedStateMachine(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Register activities
	env.RegisterActivity(MockCritiqueActivity)
	
	// Create nested state machine config
	innerConfig := StateMachineConfig{
		InitialState: "check",
		States: map[string]StateDefinition{
			"check": {
				Activity: "critique_activity",
				Transitions: []TransitionSpec{
					{To: "pass", When: ".Outputs.score >= 70"},
					{To: "fail", When: ".Outputs.score < 70"},
				},
			},
			"pass": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"passed": true,
				},
			},
			"fail": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"passed": false,
				},
			},
		},
	}
	
	// Register the inner state machine as an activity
	env.RegisterActivity(func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		executor := NewDefaultExecutor()
		executor.RegisterActivity("critique_activity", func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return MockCritiqueActivity(ctx, inputs)
		})
		
		activity, _ := NewStateMachineActivity(executor)
		return activity.Execute(ctx, innerConfig, inputs)
	})
	
	// Outer state machine config
	outerConfig := StateMachineConfig{
		InitialState: "phase1",
		States: map[string]StateDefinition{
			"phase1": {
				Activity: "inner_state_machine",
				Transitions: []TransitionSpec{
					{To: "complete", When: ".Outputs.passed == true"},
					{To: "failed", When: ".Outputs.passed == false"},
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
				Error: "Phase 1 failed",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"document": "test",
	}
	
	env.ExecuteWorkflow(StateMachineTestWorkflow, outerConfig, inputs)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "success", result["result"])
}

func TestStateMachineIntegration_RetryPolicy(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	attempts := 0
	env.RegisterActivity(func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, assert.AnError
		}
		return map[string]interface{}{"success": true}, nil
	})
	
	config := StateMachineConfig{
		InitialState: "process",
		States: map[string]StateDefinition{
			"process": {
				Activity: "retry_activity",
				Retry: &StateRetryPolicy{
					MaxAttempts:        3,
					BackoffCoefficient: 2.0,
					InitialInterval:    "100ms",
				},
				Transitions: []TransitionSpec{
					{To: "done", When: ".Outputs.success == true"},
				},
			},
			"done": {
				Terminal: true,
			},
		},
	}
	
	inputs := map[string]interface{}{}
	
	env.ExecuteWorkflow(StateMachineTestWorkflow, config, inputs)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	assert.Equal(t, 3, attempts)
}

func TestStateMachineIntegration_ComplexTransitions(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Register multiple activities
	env.RegisterActivity(func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"type":     "document",
			"priority": 5,
			"size":     1024,
		}, nil
	})
	
	config := StateMachineConfig{
		InitialState: "analyze",
		States: map[string]StateDefinition{
			"analyze": {
				Activity: "analyze_activity",
				Transitions: []TransitionSpec{
					{To: "urgent", When: ".Outputs.priority > 7"},
					{To: "normal", When: ".Outputs.priority >= 3 && .Outputs.priority <= 7"},
					{To: "low", When: ".Outputs.priority < 3"},
				},
			},
			"urgent": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"processing": "immediate",
				},
			},
			"normal": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"processing": "standard",
				},
			},
			"low": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"processing": "deferred",
				},
			},
		},
	}
	
	inputs := map[string]interface{}{
		"input": "test",
	}
	
	env.ExecuteWorkflow(StateMachineTestWorkflow, config, inputs)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "standard", result["processing"])
}

func TestStateMachineIntegration_TimeoutHandling(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Register slow activity
	env.RegisterActivity(func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		time.Sleep(5 * time.Second)
		return map[string]interface{}{"done": true}, nil
	})
	
	config := StateMachineConfig{
		InitialState: "process",
		Timeout:      "1s",
		States: map[string]StateDefinition{
			"process": {
				Activity: "slow_activity",
				Transitions: []TransitionSpec{
					{To: "done", When: ".Outputs.done == true"},
				},
			},
			"done": {
				Terminal: true,
			},
		},
	}
	
	inputs := map[string]interface{}{}
	
	env.ExecuteWorkflow(StateMachineTestWorkflow, config, inputs)
	
	require.True(t, env.IsWorkflowCompleted())
	err := env.GetWorkflowError()
	assert.Error(t, err)
}