package statemachine

import (
	"testing"
	"time"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/workflow"
)

// workflowActivityExecutor for testing
type workflowActivityExecutor struct{}

func (e *workflowActivityExecutor) ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, activityName, inputs).Get(ctx, &outputs)
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// Test CEL expression evaluation
func TestCELExpressionEvaluation(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	stateCtx := &yamlpkg.StateContext{
		CurrentState: "test_state",
		Inputs: map[string]interface{}{
			"value": 100,
			"flag":  true,
		},
		StateOutputs: map[string]map[string]interface{}{
			"previous_state": {
				"result": "success",
			},
		},
		StepOutputs: map[string]interface{}{
			"step1": map[string]interface{}{
				"count": 5,
			},
		},
	}

	// Test simple comparison
	result, err := compiler.evaluateCEL(".Inputs.value > 50", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test boolean check
	result, err = compiler.evaluateCEL(".Inputs.flag == true", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test state output access
	result, err = compiler.evaluateCEL(".States.previous_state.result == 'success'", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test step output access
	result, err = compiler.evaluateCEL(".Steps.step1.count > 3", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)
}

// Test retry policy calculation
func TestRetryBackoffCalculation(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	policy := &yamlpkg.StepRetryPolicy{
		InitialInterval:    "100ms",
		BackoffCoefficient: 2.0,
	}

	// Test backoff calculation
	backoff1 := compiler.calculateBackoff(policy, 1)
	assert.Equal(t, 100*time.Millisecond, backoff1)

	backoff2 := compiler.calculateBackoff(policy, 2)
	assert.Equal(t, 200*time.Millisecond, backoff2)

	backoff3 := compiler.calculateBackoff(policy, 3)
	assert.Equal(t, 400*time.Millisecond, backoff3)
}

// Test sequential composition with template resolution
func TestSequentialComposition(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "sequential_state",
		States: map[string]yamlpkg.StateDefinition{
			"sequential_state": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "step1",
						Uses: "step1_activity",
						Inputs: map[string]interface{}{
							"data": "input1",
						},
					},
					{
						ID:   "step2",
						Uses: "step2_activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Steps.step1.result }}",
						},
					},
					{
						ID:   "step3",
						Uses: "step3_activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Steps.step2.result }}",
						},
					},
				},
				Terminal: true,
			},
		},
	}
	
	// Verify the compiler is set up correctly
	assert.NotNil(t, compiler)
	
	// Test that templates are resolved correctly
	resolver := NewTemplateResolver()
	stateCtx := &yamlpkg.StateContext{
		StepOutputs: map[string]interface{}{
			"step1": map[string]interface{}{"result": "output1"},
			"step2": map[string]interface{}{"result": "output2"},
		},
	}
	
	// Test template resolution for step2
	step2Inputs, err := resolver.ResolveInputs(
		config.States["sequential_state"].Sequential[1].Inputs,
		stateCtx)
	assert.NoError(t, err)
	assert.Equal(t, "output1", step2Inputs["data"])
	
	// Test template resolution for step3
	step3Inputs, err := resolver.ResolveInputs(
		config.States["sequential_state"].Sequential[2].Inputs,
		stateCtx)
	assert.NoError(t, err)
	assert.Equal(t, "output2", step3Inputs["data"])
}

func testSequentialWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "sequential_state",
		States: map[string]yamlpkg.StateDefinition{
			"sequential_state": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "step1",
						Uses: "step1_activity",
						Inputs: map[string]interface{}{
							"data": "input1",
						},
					},
					{
						ID:   "step2",
						Uses: "step2_activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Steps.step1.result }}",
						},
					},
					{
						ID:   "step3",
						Uses: "step3_activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Steps.step2.result }}",
						},
					},
				},
				Terminal: true,
			},
		},
	}

	inputs := map[string]interface{}{
		"initial_data": "test_data",
	}

	return compiler.Execute(ctx, config, inputs)
}

// Test parallel composition with dependency grouping
func TestParallelComposition(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "parallel_state",
		States: map[string]yamlpkg.StateDefinition{
			"parallel_state": {
				Parallel: []yamlpkg.CompositionStep{
					{
						ID:   "parallel1",
						Uses: "parallel1_activity",
						Inputs: map[string]interface{}{
							"data": "input1",
						},
					},
					{
						ID:   "parallel2",
						Uses: "parallel2_activity",
						Inputs: map[string]interface{}{
							"data": "input2",
						},
					},
					{
						ID:   "parallel3",
						Uses: "parallel3_activity",
						Inputs: map[string]interface{}{
							"data": "input3",
						},
					},
				},
				Terminal: true,
			},
		},
	}
	
	// Verify the compiler setup
	assert.NotNil(t, compiler)
	
	// Test dependency grouping for parallel steps
	steps := config.States["parallel_state"].Parallel
	groups := compiler.groupByDependencies(steps)
	
	// All parallel steps with no dependencies should be in the same group
	assert.Len(t, groups, 1)
	assert.Len(t, groups[0], 3)
}

func testParallelWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "parallel_state",
		States: map[string]yamlpkg.StateDefinition{
			"parallel_state": {
				Parallel: []yamlpkg.CompositionStep{
					{
						ID:   "parallel1",
						Uses: "parallel1_activity",
						Inputs: map[string]interface{}{
							"data": "input1",
						},
					},
					{
						ID:   "parallel2",
						Uses: "parallel2_activity",
						Inputs: map[string]interface{}{
							"data": "input2",
						},
					},
					{
						ID:   "parallel3",
						Uses: "parallel3_activity",
						Inputs: map[string]interface{}{
							"data": "input3",
						},
					},
				},
				Terminal: true,
			},
		},
	}

	return compiler.Execute(ctx, config, map[string]interface{}{})
}

// Test conditional composition with CEL evaluation
func TestConditionalComposition(t *testing.T) {
	t.Run("high priority branch", func(t *testing.T) {
		compiler, err := NewStateMachineCompiler(nil)
		assert.NoError(t, err)
		
		config := yamlpkg.StateMachineConfig{
			InitialState: "conditional_state",
			States: map[string]yamlpkg.StateDefinition{
				"conditional_state": {
					Conditional: []yamlpkg.ConditionalBranch{
						{
							When: ".Inputs.priority == 'high'",
							Uses: "high_priority_processor",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
						{
							When: ".Inputs.priority == 'medium'",
							Uses: "medium_priority_processor",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
						{
							Default: true,
							Uses:    "default_processor",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
					},
					Terminal: true,
				},
			},
		}
		
		// Test CEL evaluation for the condition
		stateCtx := &yamlpkg.StateContext{
			Inputs: map[string]interface{}{
				"priority": "high",
				"data":     "test_data",
			},
		}
		
		// Evaluate the condition
		result, err := compiler.evaluateCEL(config.States["conditional_state"].Conditional[0].When, nil, stateCtx)
		assert.NoError(t, err)
		assert.True(t, result)
	})
	
	t.Run("default branch", func(t *testing.T) {
		compiler, err := NewStateMachineCompiler(nil)
		assert.NoError(t, err)
		
		// Test CEL evaluation for conditions that don't match
		stateCtx := &yamlpkg.StateContext{
			Inputs: map[string]interface{}{
				"priority": "low",
				"data":     "test_data",
			},
		}
		
		// Verify none of the specific conditions match
		result, err := compiler.evaluateCEL(".Inputs.priority == 'high'", nil, stateCtx)
		assert.NoError(t, err)
		assert.False(t, result)
		
		result, err = compiler.evaluateCEL(".Inputs.priority == 'medium'", nil, stateCtx)
		assert.NoError(t, err)
		assert.False(t, result)
	})
}

func testConditionalWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "conditional_state",
		States: map[string]yamlpkg.StateDefinition{
			"conditional_state": {
				Conditional: []yamlpkg.ConditionalBranch{
					{
						When: ".Inputs.priority == 'high'",
						Uses: "high_priority_processor",
						Inputs: map[string]interface{}{
							"data": "{{ .Inputs.data }}",
						},
					},
					{
						When: ".Inputs.priority == 'medium'",
						Uses: "medium_priority_processor",
						Inputs: map[string]interface{}{
							"data": "{{ .Inputs.data }}",
						},
					},
					{
						Default: true,
						Uses:    "default_processor",
						Inputs: map[string]interface{}{
							"data": "{{ .Inputs.data }}",
						},
					},
				},
				Terminal: true,
			},
		},
	}

	inputs := map[string]interface{}{
		"priority": "high",
		"data":     "test_data",
	}

	return compiler.Execute(ctx, config, inputs)
}