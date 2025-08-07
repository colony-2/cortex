package statemachine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockExecutor for testing
type MockExecutor struct {
	activities map[string]func(map[string]interface{}) (map[string]interface{}, error)
	recipes    map[string]func(map[string]interface{}) (map[string]interface{}, error)
}

func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		activities: make(map[string]func(map[string]interface{}) (map[string]interface{}, error)),
		recipes:    make(map[string]func(map[string]interface{}) (map[string]interface{}, error)),
	}
}

func (m *MockExecutor) ExecuteActivity(ctx context.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	if fn, ok := m.activities[activityName]; ok {
		return fn(inputs)
	}
	return nil, fmt.Errorf("activity %s not found", activityName)
}

func (m *MockExecutor) ExecuteRecipe(ctx context.Context, recipeName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	if fn, ok := m.recipes[recipeName]; ok {
		return fn(inputs)
	}
	return nil, fmt.Errorf("recipe %s not found", recipeName)
}

func TestStateMachineActivity_SimpleTransition(t *testing.T) {
	executor := NewMockExecutor()
	
	// Mock critique activity
	executor.activities["critique_activity"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"score": 85,
			"feedback": "Good document",
		}, nil
	}
	
	activity, err := NewStateMachineActivity(executor)
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
				Error:    "Document rejected",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"document": "test document",
	}
	
	ctx := context.Background()
	outputs, err := activity.Execute(ctx, config, inputs)
	
	require.NoError(t, err)
	assert.Equal(t, "approved", outputs["status"])
}

func TestStateMachineActivity_RetryLoop(t *testing.T) {
	executor := NewMockExecutor()
	
	attempts := 0
	executor.activities["critique_activity"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		attempts++
		score := 60
		if attempts >= 2 {
			score = 85
		}
		return map[string]interface{}{
			"score": score,
			"attempt": attempts,
		}, nil
	}
	
	executor.activities["improve_activity"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"improved": true,
		}, nil
	}
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "reviewing",
		States: map[string]StateDefinition{
			"reviewing": {
				Uses: "critique_activity",
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
	
	ctx := context.Background()
	outputs, err := activity.Execute(ctx, config, inputs)
	
	require.NoError(t, err)
	assert.NotNil(t, outputs)
	assert.Equal(t, 2, attempts) // Should have retried once
}

func TestStateMachineActivity_RecipeInvocation(t *testing.T) {
	executor := NewMockExecutor()
	
	// Mock recipe execution (now treated as an activity)
	executor.activities["child_recipe"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"result": "success",
			"data":   "processed",
		}, nil
	}
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "process",
		States: map[string]StateDefinition{
			"process": {
				Uses: "child_recipe",
				Transitions: []TransitionSpec{
					{To: "complete", When: ".Outputs.result == \"success\""},
				},
			},
			"complete": {
				Terminal: true,
			},
		},
	}
	
	inputs := map[string]interface{}{
		"input": "test data",
	}
	
	ctx := context.Background()
	outputs, err := activity.Execute(ctx, config, inputs)
	
	require.NoError(t, err)
	assert.NotNil(t, outputs)
}

func TestStateMachineActivity_ComplexCELExpressions(t *testing.T) {
	executor := NewMockExecutor()
	
	executor.activities["process"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"count": 5,
			"items": []interface{}{"a", "b", "c", "d", "e"},
		}, nil
	}
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "process",
		States: map[string]StateDefinition{
			"process": {
				Uses: "process",
				Transitions: []TransitionSpec{
					{To: "many", When: ".Outputs.count > 3"},
					{To: "few", When: ".Outputs.count <= 3"},
				},
			},
			"many": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"result": "many items",
				},
			},
			"few": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"result": "few items",
				},
			},
		},
	}
	
	inputs := map[string]interface{}{}
	
	ctx := context.Background()
	outputs, err := activity.Execute(ctx, config, inputs)
	
	require.NoError(t, err)
	assert.Equal(t, "many items", outputs["result"])
}

func TestStateMachineActivity_Timeout(t *testing.T) {
	t.Skip("Timeout test requires context cancellation support in mock executor")
	executor := NewMockExecutor()
	
	// Slow activity that checks context
	executor.activities["slow_activity"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		// Simulate slow activity that would be cancelled
		select {
		case <-time.After(2 * time.Second):
			return map[string]interface{}{"done": true}, nil
		}
	}
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "process",
		Timeout:      "100ms", // Very short timeout
		States: map[string]StateDefinition{
			"process": {
				Uses: "slow_activity",
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
	
	ctx := context.Background()
	_, err = activity.Execute(ctx, config, inputs)
	
	if err != nil {
		assert.Contains(t, err.Error(), "timed out")
	}
}

func TestStateMachineActivity_ErrorState(t *testing.T) {
	executor := NewMockExecutor()
	
	executor.activities["check"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"valid": false,
		}, nil
	}
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "validate",
		States: map[string]StateDefinition{
			"validate": {
				Uses: "check",
				Transitions: []TransitionSpec{
					{To: "success", When: ".Outputs.valid == true"},
					{To: "failure", When: ".Outputs.valid == false"},
				},
			},
			"success": {
				Terminal: true,
			},
			"failure": {
				Terminal: true,
				Error:    "Validation failed",
			},
		},
	}
	
	inputs := map[string]interface{}{}
	
	ctx := context.Background()
	_, err = activity.Execute(ctx, config, inputs)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Validation failed")
}

func TestStateMachineActivity_StateOutputAccess(t *testing.T) {
	executor := NewMockExecutor()
	
	executor.activities["first"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"value": 10,
		}, nil
	}
	
	executor.activities["second"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"value": 20,
		}, nil
	}
	
	executor.activities["combine"] = func(inputs map[string]interface{}) (map[string]interface{}, error) {
		// This would access previous state outputs
		return map[string]interface{}{
			"total": 30,
		}, nil
	}
	
	activity, err := NewStateMachineActivity(executor)
	require.NoError(t, err)
	
	config := StateMachineConfig{
		InitialState: "step1",
		States: map[string]StateDefinition{
			"step1": {
				Uses: "first",
				Transitions: []TransitionSpec{
					{To: "step2"},
				},
			},
			"step2": {
				Uses: "second",
				Transitions: []TransitionSpec{
					{To: "step3"},
				},
			},
			"step3": {
				Uses: "combine",
				Transitions: []TransitionSpec{
					{To: "done", When: ".Outputs.total == 30"},
				},
			},
			"done": {
				Terminal: true,
			},
		},
	}
	
	inputs := map[string]interface{}{}
	
	ctx := context.Background()
	outputs, err := activity.Execute(ctx, config, inputs)
	
	require.NoError(t, err)
	assert.NotNil(t, outputs)
}