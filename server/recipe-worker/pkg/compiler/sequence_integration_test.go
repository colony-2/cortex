package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"gopkg.in/yaml.v3"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
)

func TestSequenceOutputIntegration(t *testing.T) {
	// Test the full execution path from YAML to ExecuteRecipe
	recipeYAML := `
id: test-sequence-integration
desc: Integration test for sequence output references
version: '1.0'
inputs:
  base_value:
    type: number
    default: 10
sequence:
- id: step1
  op: echo_activity
  inputs:
    message: "Starting process"
- id: step2
  op: echo_activity
  inputs:
    message: "Processing data"
- id: step3
  op: echo_activity
  inputs:
    message: '{{ "Previous: " + string(sequence.step2.outputs.output) }}'
outputs:
  step1_output: '{{ sequence.step1.outputs.output }}'
  step2_result: '{{ sequence.step2.outputs.output }}'
  step3_output: '{{ sequence.step3.outputs.output }}'
  combined: '{{ string(sequence.step1.outputs.output) + " -> " + string(sequence.step2.outputs.output) }}'
`

	// Parse the recipe
	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(recipeYAML), &r)
	require.NoError(t, err)

	// Create test activity registry - it comes pre-loaded with test activities
	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	// The registry already has echo_activity registered
	// echo_activity returns: {result: message, output: message}
	// test_activity is not pre-registered, so we'll use echo_activity instead

	// Set up Temporal test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	// Execute the workflow
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		inputs := map[string]interface{}{
			"base_value": 10,
		}
		return ExecuteRecipe(ctx, registry, r, withRequiredGitInputs(inputs))
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err = env.GetWorkflowResult(&result)
	require.NoError(t, err)

	// Verify the outputs
	t.Logf("Recipe execution result: %+v", result)

	// Check if outputs were properly resolved
	assert.Equal(t, "Starting process", result["step1_output"])
	assert.Equal(t, "Processing data", result["step2_result"])
	assert.Equal(t, "Previous: Processing data", result["step3_output"])
	assert.Equal(t, "Starting process -> Processing data", result["combined"])
}

func TestSequenceWithComplexTemplates(t *testing.T) {
	// Test with more complex template references using echo_activity only
	recipeYAML := `
id: test-complex-templates
desc: Test complex template references in sequences
version: '1.0'
sequence:
- id: fetch_data
  op: echo_activity
  inputs:
    message: "100"
- id: process_data
  op: echo_activity
  inputs:
    message: '{{ "Processed: " + string(sequence.fetch_data.outputs.output) }}'
- id: validate
  op: echo_activity
  inputs:
    message: '{{ string(sequence.process_data.outputs.output).contains("Processed") ? "Valid" : "Invalid" }}'
outputs:
  is_valid: '{{ string(sequence.validate.outputs.output) == "Valid" }}'
  fetch_output: '{{ sequence.fetch_data.outputs.output }}'
  process_output: '{{ sequence.process_data.outputs.output }}'
  validation_output: '{{ sequence.validate.outputs.output }}'
`

	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(recipeYAML), &r)
	require.NoError(t, err)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	// Execute in test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, r, withRequiredGitInputs(map[string]interface{}{}))
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err = env.GetWorkflowResult(&result)
	require.NoError(t, err)

	t.Logf("Complex template result: %+v", result)

	// Verify complex outputs
	assert.Equal(t, true, result["is_valid"])
	assert.Equal(t, "100", result["fetch_output"])
	assert.Equal(t, "Processed: 100", result["process_output"])
	assert.Equal(t, "Valid", result["validation_output"])
}

func TestSequenceOutputErrorHandling(t *testing.T) {
	// Test error cases in sequence output references
	recipeYAML := `
id: test-error-handling
desc: Test error handling in sequence output references
version: '1.0'
sequence:
- id: step1
  op: echo_activity
  inputs:
    message: "test"
outputs:
  # This should fail - referencing non-existent node
  missing_node: '{{ sequence.nonexistent.outputs.field }}'
`

	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(recipeYAML), &r)
	require.NoError(t, err)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)
	// echo_activity is already registered in the registry

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, r, withRequiredGitInputs(map[string]interface{}{}))
	})

	require.True(t, env.IsWorkflowCompleted())

	// Should have an error due to missing node reference
	err = env.GetWorkflowError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestSequenceActualRecipeFile(t *testing.T) {
	// Test with the actual test-sequence-node.yaml content
	recipeYAML := `
id: test-sequence-node
desc: Test recipe for sequence node with multiple operations
version: '1.0'
sequence:
- id: step1
  op: echo_activity
  inputs:
    message: "first step"
- id: step2
  op: echo_activity
  inputs:
    message: "second step"
- id: step3
  op: echo_activity
  inputs:
    message: "Step 3 complete"
outputs:
  command_output: '{{ sequence.step1.outputs.output }}'
  step2_result: '{{ sequence.step2.outputs.output }}'
  final_output: '{{ sequence.step3.outputs.output }}'
`

	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(recipeYAML), &r)
	require.NoError(t, err)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)
	// echo_activity is already registered in the registry

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, r, withRequiredGitInputs(map[string]interface{}{}))
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err = env.GetWorkflowResult(&result)
	require.NoError(t, err)

	t.Logf("Actual recipe file result: %+v", result)

	// Verify outputs match expected values
	assert.Equal(t, "first step", result["command_output"])
	assert.Equal(t, "second step", result["step2_result"])
	assert.Equal(t, "Step 3 complete", result["final_output"])
}
