//go:build temporal
// +build temporal

package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"gopkg.in/yaml.v3"

	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
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
	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)

	// The registry already has echo_activity registered
	// echo_activity returns: {result: message, output: message}
	// test_activity is not pre-registered, so we'll use echo_activity instead

	// Set up Temporal test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	// Execute the workflow
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		inputs := map[string]interface{}{
			"base_value": 10,
		}
		expandedInputs, execCtx := withRequiredGitInputs(inputs)
		return ExecuteRecipe(ctx, registry, r, expandedInputs, execCtx)
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

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)

	// Execute in test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		inputs, execCtx := withRequiredGitInputs(map[string]interface{}{})
		return ExecuteRecipe(ctx, registry, r, inputs, execCtx)
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

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	// echo_activity is already registered in the registry

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		inputs, execCtx := withRequiredGitInputs(map[string]interface{}{})
		return ExecuteRecipe(ctx, registry, r, inputs, execCtx)
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

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	// echo_activity is already registered in the registry

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	// Register activities from the registry with the test environment
	registry.EnableActivitiesInWorker(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		inputs, execCtx := withRequiredGitInputs(map[string]interface{}{})
		return ExecuteRecipe(ctx, registry, r, inputs, execCtx)
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

func TestNestedRecipeGitContextPropagation(t *testing.T) {
	parentYAML := `
id: parent-nested
desc: Parent recipe invoking child recipe
version: '1.0'
sequence:
- id: init_parent
  op: test_write_file
  inputs:
    path: '{{ inputs.context.worktree }}/parent.txt'
    content: 'parent'
- id: nested_child
  op: recipe
  inputs:
    name: child-workflow
    inputs: {}
outputs:
  child_git_hash: '{{ sequence.nested_child.outputs.git_persist_hash }}'
  final_parent_hash: '{{ sequence.nested_child.outputs.context.git.persist_hash }}'
  child_result: '{{ sequence.nested_child.outputs }}'
`

	childYAML := `
id: child-nested
desc: Child recipe reading parent state and writing child file
version: '1.0'
sequence:
- id: read_parent
  op: test_read_file
  inputs:
    path: '{{ inputs.context.worktree }}/parent.txt'
- id: write_child
  op: test_write_file
  inputs:
    path: '{{ inputs.context.worktree }}/child.txt'
    content: 'child'
outputs:
  parent_stdout: '{{ sequence.read_parent.outputs["content"] }}'
  write_stdout: '{{ sequence.write_child.outputs["path"] }}'
`

	ensureNestedTestOpsRegistered()

	var parent recipe.Recipe
	require.NoError(t, yaml.Unmarshal([]byte(parentYAML), &parent))
	var child recipe.Recipe
	require.NoError(t, yaml.Unmarshal([]byte(childYAML), &child))

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	registry.EnableActivitiesInWorker(env)

	env.RegisterWorkflowWithOptions(func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		childInputs, childCtx := withRequiredGitInputs(inputs)
		result, err := ExecuteRecipe(ctx, registry, child, childInputs, childCtx)
		workflow.GetLogger(ctx).Info("child workflow finished", "result", result)
		return result, err
	}, workflow.RegisterOptions{Name: "child-workflow"})

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		parentInputs, parentCtx := withRequiredGitInputs(map[string]interface{}{})
		return ExecuteRecipe(ctx, registry, parent, parentInputs, parentCtx)
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	t.Logf("Parent outputs: %+v", result)

	childHash, _ := result["child_git_hash"].(string)
	parentHash, _ := result["final_parent_hash"].(string)
	require.NotEmpty(t, childHash)
	require.Equal(t, childHash, parentHash)
	nestedOutputs, ok := result["child_result"].(map[string]interface{})
	require.True(t, ok)
	childResult, ok := nestedOutputs["result"].(map[string]interface{})
	require.True(t, ok)
	require.NotEmpty(t, childResult)
}

var nestedOpsOnce sync.Once

type testWriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type testWriteFileOutput struct {
	Path string `json:"path"`
}

type testReadFileInput struct {
	Path string `json:"path"`
}

type testReadFileOutput struct {
	Content string `json:"content"`
}

func ensureNestedTestOpsRegistered() {
	nestedOpsOnce.Do(func() {
		writeOp := recipeops.NewActivityMappedOpV2[testWriteFileInput, testWriteFileOutput](
			recipeops.OpMetadata{
				Type:        "test_write_file",
				Description: "writes file into git workspace for testing",
				Version:     "1.0.0",
			},
			func(_ recipeops.Invocation, ctx context.Context, input testWriteFileInput) (testWriteFileOutput, error) {
				if input.Path == "" {
					return testWriteFileOutput{}, fmt.Errorf("path is required")
				}
				dir := filepath.Dir(input.Path)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return testWriteFileOutput{}, err
				}
				if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
					return testWriteFileOutput{}, err
				}
				return testWriteFileOutput{Path: input.Path}, nil
			},
		)

		readOp := recipeops.NewActivityMappedOpV2[testReadFileInput, testReadFileOutput](
			recipeops.OpMetadata{
				Type:        "test_read_file",
				Description: "reads file contents from git workspace for testing",
				Version:     "1.0.0",
			},
			func(_ recipeops.Invocation, ctx context.Context, input testReadFileInput) (testReadFileOutput, error) {
				if input.Path == "" {
					return testReadFileOutput{}, fmt.Errorf("path is required")
				}
				data, err := os.ReadFile(input.Path)
				if err != nil {
					return testReadFileOutput{}, err
				}
				return testReadFileOutput{Content: string(data)}, nil
			},
		)

		recipeops.Register(writeOp, readOp)
	})
}
