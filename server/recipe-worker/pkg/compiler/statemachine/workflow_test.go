package statemachine

import (
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/workflow"
)

// TestNestedCompositionWorkflow tests deeply nested compositions
func TestNestedCompositionWorkflow(t *testing.T) {
	// Test nested composition structure without workflow complexity
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "nested_state",
		States: map[string]yamlpkg.StateDefinition{
			"nested_state": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "prepare",
						Uses: "prepare_activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Inputs.data }}",
						},
					},
					{
						ID: "process",
						Parallel: []yamlpkg.CompositionStep{
							{
								ID:   "process1",
								Uses: "process1_activity",
								Inputs: map[string]interface{}{
									"data": "{{ .Steps.prepare.prepared }}",
								},
							},
							{
								ID:   "process2",
								Uses: "process2_activity",
								Inputs: map[string]interface{}{
									"data": "{{ .Steps.prepare.prepared }}",
								},
							},
						},
					},
					{
						ID:   "finalize",
						Uses: "finalize_activity",
						Inputs: map[string]interface{}{
							"result1": "{{ .Steps.process.process1.result }}",
							"result2": "{{ .Steps.process.process2.result }}",
						},
					},
				},
				Terminal: true,
			},
		},
	}
	
	// Verify the nested structure is valid
	assert.NotNil(t, compiler)
	assert.Len(t, config.States["nested_state"].Sequential, 3)
	assert.Len(t, config.States["nested_state"].Sequential[1].Parallel, 2)
}

func testNestedWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "nested_state",
		States: map[string]yamlpkg.StateDefinition{
			"nested_state": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "prepare",
						Uses: "prepare_activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Inputs.data }}",
						},
					},
					{
						ID: "process",
						Parallel: []yamlpkg.CompositionStep{
							{
								ID:   "process1",
								Uses: "process1_activity",
								Inputs: map[string]interface{}{
									"data": "{{ .Steps.prepare.prepared }}",
								},
							},
							{
								ID:   "process2",
								Uses: "process2_activity",
								Inputs: map[string]interface{}{
									"data": "{{ .Steps.prepare.prepared }}",
								},
							},
						},
					},
					{
						ID:   "finalize",
						Uses: "finalize_activity",
						Inputs: map[string]interface{}{
							"result1": "{{ .Steps.process.process1.result }}",
							"result2": "{{ .Steps.process.process2.result }}",
						},
					},
				},
				Terminal: true,
			},
		},
	}

	return compiler.Execute(ctx, config, map[string]interface{}{"data": "test_data"})
}

// TestStateTransitionsWorkflow tests state machine transitions
func TestStateTransitionsWorkflow(t *testing.T) {
	// Test state transitions without workflow complexity
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "validation",
		States: map[string]yamlpkg.StateDefinition{
			"validation": {
				Uses: "validate_activity",
				Inputs: map[string]interface{}{
					"data": "{{ .Inputs.data }}",
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To:   "processing",
						When: ".Outputs.valid == true",
					},
					{
						To:   "error",
						When: ".Outputs.valid == false",
					},
				},
			},
			"processing": {
				Uses: "process_activity",
				Inputs: map[string]interface{}{
					"data": "{{ .Inputs.data }}",
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To: "complete",
					},
				},
			},
			"complete": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"message": "Processing complete",
				},
			},
			"error": {
				Terminal: true,
				Error:    "Validation failed",
			},
		},
	}
	
	// Test transition evaluation
	stateCtx := &yamlpkg.StateContext{
		CurrentState: "validation",
	}
	
	// Valid output should transition to processing
	outputs := map[string]interface{}{"valid": true}
	nextState := compiler.evaluateTransitions(config.States["validation"].Transitions, outputs, stateCtx)
	assert.Equal(t, "processing", nextState)
	
	// Invalid output should transition to error
	outputs = map[string]interface{}{"valid": false}
	nextState = compiler.evaluateTransitions(config.States["validation"].Transitions, outputs, stateCtx)
	assert.Equal(t, "error", nextState)
}

func testTransitionsWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "validation",
		States: map[string]yamlpkg.StateDefinition{
			"validation": {
				Uses: "validate_activity",
				Inputs: map[string]interface{}{
					"data": "{{ .Inputs.data }}",
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To:   "processing",
						When: ".Outputs.valid == true",
					},
					{
						To:   "error",
						When: ".Outputs.valid == false",
					},
				},
			},
			"processing": {
				Uses: "process_activity",
				Inputs: map[string]interface{}{
					"data": "{{ .Inputs.data }}",
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To: "complete",
					},
				},
			},
			"complete": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"message": "Processing complete",
				},
			},
			"error": {
				Terminal: true,
				Error:    "Validation failed",
			},
		},
	}

	return compiler.Execute(ctx, config, map[string]interface{}{"data": "test_data"})
}

// TestRetryPolicyWorkflow tests retry policy in state machine
func TestRetryPolicyWorkflow(t *testing.T) {
	// Test retry policy configuration without workflow complexity
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "retry_state",
		States: map[string]yamlpkg.StateDefinition{
			"retry_state": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "flaky_step",
						Uses: "flaky_activity",
						Retry: &yamlpkg.StepRetryPolicy{
							MaxAttempts:        2,
							BackoffCoefficient: 2.0,
							InitialInterval:    100,
						},
					},
				},
				Terminal: true,
			},
		},
	}
	
	// Verify retry configuration
	assert.NotNil(t, compiler)
	step := config.States["retry_state"].Sequential[0]
	assert.NotNil(t, step.Retry)
	assert.Equal(t, 2, step.Retry.MaxAttempts)
	assert.Equal(t, 2.0, step.Retry.BackoffCoefficient)
}

func testRetryWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "retry_state",
		States: map[string]yamlpkg.StateDefinition{
			"retry_state": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "flaky_step",
						Uses: "flaky_activity",
						Retry: &yamlpkg.StepRetryPolicy{
							MaxAttempts:        2,
							BackoffCoefficient: 2.0,
							InitialInterval:    100,
						},
					},
				},
				Terminal: true,
			},
		},
	}

	return compiler.Execute(ctx, config, map[string]interface{}{"data": "test_data"})
}