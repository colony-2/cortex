package statemachine

import (
	"fmt"
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/workflow"
)

// MockSimpleExecutor for testing without Temporal complexity
type MockSimpleExecutor struct {
	activities map[string]func(map[string]interface{}) (map[string]interface{}, error)
}

func NewMockSimpleExecutor() *MockSimpleExecutor {
	return &MockSimpleExecutor{
		activities: make(map[string]func(map[string]interface{}) (map[string]interface{}, error)),
	}
}

func (m *MockSimpleExecutor) ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	if fn, ok := m.activities[activityName]; ok {
		return fn(inputs)
	}
	return nil, fmt.Errorf("activity %s not found", activityName)
}

func (m *MockSimpleExecutor) RegisterActivity(name string, fn func(map[string]interface{}) (map[string]interface{}, error)) {
	m.activities[name] = fn
}

// Test simple state transition logic
func TestSimpleStateTransition(t *testing.T) {
	executor := NewMockSimpleExecutor()
	
	// Mock critique activity
	executor.RegisterActivity("critique_activity", func(inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"score":    85,
			"feedback": "Good document",
		}, nil
	})
	
	compiler, err := NewStateMachineCompiler(nil) // Using nil executor for CEL testing
	require.NoError(t, err)
	
	// Test CEL transition evaluation
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "reviewing",
		Inputs: map[string]interface{}{
			"document": "test document",
		},
		StateOutputs: map[string]map[string]interface{}{
			"reviewing": {
				"score":    85,
				"feedback": "Good document",
			},
		},
	}
	
	// Test transition evaluation
	transitions := []yamlpkg.TransitionSpec{
		{To: "approved", When: ".Outputs.score >= 80"},
		{To: "rejected", When: ".Outputs.score < 80"},
	}
	
	outputs := map[string]interface{}{
		"score":    85,
		"feedback": "Good document",
	}
	
	nextState := compiler.evaluateTransitions(transitions, outputs, stateCtx)
	assert.Equal(t, "approved", nextState)
}

// Test retry loop logic
func TestRetryLoopLogic(t *testing.T) {
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
	
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	// Test retry policy evaluation
	policy := &yamlpkg.StateRetryPolicy{
		When:               ".Outputs.score < 80",
		MaxAttempts:        3,
		BackoffCoefficient: 1.0,
		InitialInterval:    "100ms",
	}
	
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "reviewing",
		Attempts: map[string]int{
			"reviewing": 1,
		},
		StateOutputs: map[string]map[string]interface{}{},
	}
	
	// For retry evaluation, the outputs are from the current state
	// We need to pass them directly to evaluateCEL
	outputs := map[string]interface{}{
		"score": 60,
	}
	
	// Test the CEL expression directly
	shouldRetryResult, err := compiler.evaluateCEL(policy.When, outputs, stateCtx)
	assert.NoError(t, err)
	assert.True(t, shouldRetryResult)
	
	// Should not retry when max attempts reached
	stateCtx.Attempts["reviewing"] = 3
	shouldRetry := compiler.shouldRetry(policy, nil, stateCtx)
	assert.False(t, shouldRetry)
}

// Test composition step dependency management
func TestStepDependencies(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	steps := []yamlpkg.CompositionStep{
		{ID: "a", DependsOn: []string{}},
		{ID: "b", DependsOn: []string{}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b"}},
		{ID: "e", DependsOn: []string{"c", "d"}},
	}
	
	groups := compiler.groupByDependencies(steps)
	
	// Should have 2 groups
	assert.Len(t, groups, 2)
	
	// First group should have steps with no dependencies
	assert.Len(t, groups[0], 2)
	
	// Check dependencies are met
	outputs := map[string]interface{}{
		"a": "result_a",
		"b": "result_b",
	}
	
	assert.True(t, compiler.dependenciesMet([]string{"a"}, outputs))
	assert.True(t, compiler.dependenciesMet([]string{"a", "b"}, outputs))
	assert.False(t, compiler.dependenciesMet([]string{"c"}, outputs))
}

// Test terminal state detection
func TestTerminalStateDetection(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	states := map[string]yamlpkg.StateDefinition{
		"active": {
			Uses: "some_activity",
		},
		"terminal": {
			Terminal: true,
		},
		"error": {
			Terminal: true,
			Error:    "An error occurred",
		},
	}
	
	assert.False(t, compiler.isTerminal("active", states))
	assert.True(t, compiler.isTerminal("terminal", states))
	assert.True(t, compiler.isTerminal("error", states))
	assert.True(t, compiler.isTerminal("nonexistent", states))
}

// Test template resolution in inputs
func TestInputTemplateResolution(t *testing.T) {
	resolver := NewTemplateResolver()
	
	stateCtx := &yamlpkg.StateContext{
		Inputs: map[string]interface{}{
			"document": "test document",
		},
		StepOutputs: map[string]interface{}{
			"step1": map[string]interface{}{
				"result": "processed",
			},
		},
		StateOutputs: map[string]map[string]interface{}{
			"previous": {
				"data": "previous_result",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"doc":      "{{ .Inputs.document }}",
		"step_res": "{{ .Steps.step1.result }}",
		"state_res": "{{ .States.previous.data }}",
		"static":   "no_template",
		"nested": map[string]interface{}{
			"inner": "{{ .Inputs.document }}",
		},
	}
	
	resolved, err := resolver.ResolveInputs(inputs, stateCtx)
	require.NoError(t, err)
	
	assert.Equal(t, "test document", resolved["doc"])
	assert.Equal(t, "processed", resolved["step_res"])
	assert.Equal(t, "previous_result", resolved["state_res"])
	assert.Equal(t, "no_template", resolved["static"])
	
	nested := resolved["nested"].(map[string]interface{})
	assert.Equal(t, "test document", nested["inner"])
}

// Test CEL expression with complex conditions
func TestComplexCELConditions(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)
	
	stateCtx := &yamlpkg.StateContext{
		Inputs: map[string]interface{}{
			"data_size": 1500000,
			"priority":  "high",
		},
		StateOutputs: map[string]map[string]interface{}{
			"validation": {
				"valid": true,
				"score": 95,
			},
		},
	}
	
	// Test complex AND condition
	result, err := compiler.evaluateCEL(
		".Inputs.data_size > 1000000 && .Inputs.priority == 'high'",
		nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)
	
	// Test OR condition
	result, err = compiler.evaluateCEL(
		".States.validation.score > 90 || .States.validation.valid == false",
		nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)
	
	// Test nested field access
	result, err = compiler.evaluateCEL(
		".States.validation.valid == true && .States.validation.score >= 95",
		nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)
}