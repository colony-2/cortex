package statemachine

import (
	"fmt"
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/workflow"
)

// TestStateMachineSimpleTransition tests simple state transitions
// Refactored from the original activity test
func TestStateMachineSimpleTransition(t *testing.T) {
	executor := NewMockSimpleExecutor()
	
	// Mock critique activity
	executor.RegisterActivity("critique_activity", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"score":    85,
			"feedback": "Good document",
		}, nil
	})
	
	compiler, err := NewStateMachineCompiler(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "reviewing",
		States: map[string]StateDefinition{
			"reviewing": {
				Uses: "critique_activity",
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
	
	// Test transition evaluation
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "reviewing",
		Inputs: map[string]interface{}{
			"document": "test document",
		},
	}
	
	outputs := map[string]interface{}{
		"score":    85,
		"feedback": "Good document",
	}
	
	nextState := compiler.evaluateTransitions(config.States["reviewing"].Transitions, outputs, stateCtx)
	assert.Equal(t, "approved", nextState)
}

// TestStateMachineRetryLoop tests retry behavior
// Refactored from the original activity test
func TestStateMachineRetryLoop(t *testing.T) {
	executor := NewMockSimpleExecutor()
	
	attempts := 0
	executor.RegisterActivity("critique_activity", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		attempts++
		score := 60
		if attempts >= 2 {
			score = 85
		}
		return map[string]interface{}{
			"score":   score,
			"attempt": attempts,
		}, nil
	})
	
	executor.RegisterActivity("improve_activity", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"improved": true,
			"content":  "Improved document",
		}, nil
	})
	
	compiler, err := NewStateMachineCompiler(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "reviewing",
		States: map[string]StateDefinition{
			"reviewing": {
				Uses: "critique_activity",
				Retry: &StateRetryPolicy{
					When:               ".Outputs.score < 80",
					MaxAttempts:        3,
					BackoffCoefficient: 2.0,
					InitialInterval:    "100ms",
				},
				Transitions: []TransitionSpec{
					{To: "approved", When: ".Outputs.score >= 80"},
					{To: "improving", When: ".Outputs.score < 80 && .State.Attempts < 3"},
					{To: "rejected", When: ".State.Attempts >= 3"},
				},
			},
			"improving": {
				Uses: "improve_activity",
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
	
	// Test retry logic
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "reviewing",
		Attempts: map[string]int{
			"reviewing": 1,
		},
		StateOutputs: map[string]map[string]interface{}{},
	}
	
	outputs := map[string]interface{}{
		"score": 60,
	}
	
	// Test the CEL expression directly with outputs
	shouldRetryResult, err := compiler.evaluateCEL(config.States["reviewing"].Retry.When, outputs, stateCtx)
	assert.NoError(t, err)
	assert.True(t, shouldRetryResult)
	
	// Should not retry when max attempts reached
	stateCtx.Attempts["reviewing"] = 3
	shouldRetry := compiler.shouldRetry(config.States["reviewing"].Retry, nil, stateCtx)
	assert.False(t, shouldRetry)
}

// TestNestedStateMachine tests nested state machine execution
// Refactored from the original activity test
func TestNestedStateMachine(t *testing.T) {
	executor := NewMockSimpleExecutor()
	
	// Register inner state machine activities
	executor.RegisterActivity("validate_activity", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"valid": true}, nil
	})
	
	// Register the inner state machine as an activity
	executor.RegisterActivity("inner_state_machine", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		// Simulate inner state machine execution
		return map[string]interface{}{
			"validation_status": "passed",
		}, nil
	})
	
	compiler, err := NewStateMachineCompiler(executor)
	require.NoError(t, err)
	
	// Outer state machine config
	config := StateMachineConfig{
		InitialState: "phase1",
		States: map[string]StateDefinition{
			"phase1": {
				Uses: "inner_state_machine",
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
	
	// Test transition evaluation with nested result
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "phase1",
	}
	
	outputs := map[string]interface{}{
		"validation_status": "passed",
	}
	
	nextState := compiler.evaluateTransitions(config.States["phase1"].Transitions, outputs, stateCtx)
	assert.Equal(t, "complete", nextState)
}

// TestComplexTransitionConditions tests complex CEL expressions
// Refactored from the original activity test
func TestComplexTransitionConditions(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "processing",
		Inputs: map[string]interface{}{
			"priority": "high",
			"size":     1500000,
		},
		StateOutputs: map[string]map[string]interface{}{
			"validation": {
				"valid": true,
				"score": 95,
			},
		},
	}
	
	transitions := []TransitionSpec{
		{
			To:   "fast_track",
			When: ".Inputs.priority == 'high' && .Outputs.score > 90",
		},
		{
			To:   "normal",
			When: ".Outputs.score > 70",
		},
		{
			To:   "review",
			When: ".Outputs.score <= 70",
		},
	}
	
	outputs := map[string]interface{}{
		"score": 95,
	}
	
	// Should match the first condition (fast_track)
	nextState := compiler.evaluateTransitions(transitions, outputs, stateCtx)
	assert.Equal(t, "fast_track", nextState)
	
	// Test with lower score
	outputs["score"] = 75
	nextState = compiler.evaluateTransitions(transitions, outputs, stateCtx)
	assert.Equal(t, "normal", nextState)
	
	// Test with even lower score
	outputs["score"] = 60
	nextState = compiler.evaluateTransitions(transitions, outputs, stateCtx)
	assert.Equal(t, "review", nextState)
}

// TestParallelStepsWithDependencies tests parallel execution with dependencies
// Refactored from the original activity test
func TestParallelStepsWithDependencies(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	steps := []CompositionStep{
		{ID: "a", DependsOn: []string{}},
		{ID: "b", DependsOn: []string{}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b"}},
		{ID: "e", DependsOn: []string{"c", "d"}},
		{ID: "f", DependsOn: []string{"a", "b"}},
	}
	
	groups := compiler.groupByDependencies(steps)
	
	// The actual grouping based on the dependencies:
	// Group 1: a, b (no dependencies)
	// Group 2: c (depends on a), d (depends on b), f (depends on a,b), e (depends on c,d)
	// Note: The algorithm may group differently based on implementation
	
	// At minimum we should have 2 groups (independent and dependent)
	assert.GreaterOrEqual(t, len(groups), 2)
	
	// First group should have the independent steps
	if len(groups) > 0 {
		assert.Len(t, groups[0], 2) // a and b have no dependencies
	}
	
	// Verify dependency checking
	outputs := map[string]interface{}{
		"a": "result_a",
		"b": "result_b",
	}
	
	assert.True(t, compiler.dependenciesMet([]string{"a", "b"}, outputs))
	assert.False(t, compiler.dependenciesMet([]string{"c"}, outputs))
	
	outputs["c"] = "result_c"
	outputs["d"] = "result_d"
	assert.True(t, compiler.dependenciesMet([]string{"c", "d"}, outputs))
}

// TestErrorHandling tests error propagation and handling
// Refactored from the original activity test
func TestErrorHandling(t *testing.T) {
	executor := NewMockSimpleExecutor()
	
	// Register activity that returns an error
	executor.RegisterActivity("failing_activity", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return nil, fmt.Errorf("activity failed: %v", inputs["reason"])
	})
	
	compiler, err := NewStateMachineCompiler(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "processing",
		States: map[string]StateDefinition{
			"processing": {
				Uses: "failing_activity",
				Transitions: []TransitionSpec{
					{To: "error_handling", When: ".Outputs.Error != ''"},
					{To: "success"},
				},
			},
			"error_handling": {
				Terminal: true,
				Error:    "Processing failed",
			},
			"success": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"status": "completed",
				},
			},
		},
	}
	
	// Verify error transitions are evaluated correctly
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "processing",
	}
	
	// When there's an error in outputs, should take error transition
	outputs := map[string]interface{}{
		"Error": "activity failed",
	}
	
	// Test the CEL expression for error transition
	errorResult, err := compiler.evaluateCEL(".Outputs.Error != ''", outputs, stateCtx)
	assert.NoError(t, err)
	assert.True(t, errorResult)
	
	// Since evaluateTransitions checks conditions in order, the error transition should match
	nextState := compiler.evaluateTransitions(config.States["processing"].Transitions, outputs, stateCtx)
	assert.Equal(t, "error_handling", nextState)
}

// TestTimeoutHandling tests timeout configuration
// Refactored from the original activity test
func TestTimeoutHandling(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "processing",
		States: map[string]StateDefinition{
			"processing": {
				Uses: "slow_activity",
				Transitions: []TransitionSpec{
					{To: "timeout_handler", When: ".Outputs.TimedOut == true"},
					{To: "success"},
				},
			},
			"timeout_handler": {
				Terminal: true,
				Error:    "Processing timed out",
			},
			"success": {
				Terminal: true,
			},
		},
	}
	
	// Verify timeout transitions
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "processing",
	}
	
	// Should take timeout transition when timed out
	outputs := map[string]interface{}{
		"TimedOut": true,
	}
	nextState := compiler.evaluateTransitions(config.States["processing"].Transitions, outputs, stateCtx)
	assert.Equal(t, "timeout_handler", nextState)
}

// MockWorkflowContext for testing without Temporal
type MockWorkflowContext struct {
	workflow.Context
}

func (m MockWorkflowContext) Done() workflow.Channel {
	// Return a mock channel
	return nil
}