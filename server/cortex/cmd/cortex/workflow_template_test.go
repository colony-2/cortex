package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
	export3 "github.com/divisive-ai/vibethis/server/git/pkg/export"
	"github.com/divisive-ai/vibethis/server/ops/pkg/export"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	export2 "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/export"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	ops2 "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	recipeworkflows "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/workflows"
)

type svcContext struct {
}

func (s svcContext) Get(name string) (interface{}, error) {
	return nil, fmt.Errorf("service not found: %s", name)
}

func TestWorkflowTemplateExpansion(t *testing.T) {
	// Create test suite
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	_, _, err := shared.SetupOps(svcContext{})
	require.NoError(t, err)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	// Track what inputs the activity receives
	var capturedInputs map[string]interface{}

	// Register the real activity executor that also captures inputs
	env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			capturedInputs = inputs

			// Execute the real activity
			logger := createTestLogger()
			activityExecutor, err := executor.NewStandaloneExecutor(registry, logger)
			require.NoError(t, err)
			executorFunc := activityExecutor.CreateTemporalActivity("command_execution")
			return executorFunc.(func(context.Context, map[string]interface{}) (map[string]interface{}, error))(ctx, inputs)
		},
		activity.RegisterOptions{
			Name: "command_execution",
		},
	)

	// Create a recipe with templates
	recipe := &yamlpkg.Recipe{
		Name: "test-workflow-templates",
		Sequence: []yamlpkg.Node{
			{
				ID: "step1",
				Op: "command_execution",
				Inputs: map[string]interface{}{
					"run": "echo {{ .Inputs.message }}",
				},
			},
		},
	}

	// Create compiler
	compilerRegistry := compiler.NewActivityRegistry()
	compilerRegistry.RegisterActivity("command_execution")
	comp := compiler.NewCompiler(compilerRegistry)

	// Create workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipe, comp)

	// Register workflow
	env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: recipe.Name,
		},
	)

	// Execute workflow with inputs
	inputs := map[string]interface{}{
		"message": "Hello from test",
	}
	env.ExecuteWorkflow(recipe.Name, inputs)

	// Verify workflow completed
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify that the activity received the expanded template
	require.NotNil(t, capturedInputs, "Activity should have been called")

	// The key assertion: the template should have been expanded
	runCmd, ok := capturedInputs["run"].(string)
	require.True(t, ok, "run should be a string")
	assert.Equal(t, "echo Hello from test", runCmd, "Template should be expanded")
	assert.NotContains(t, runCmd, "{{", "Template markers should be removed")
}

func TestWorkflowWithMultipleTemplates(t *testing.T) {
	// Create test suite
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create activity registry and register real activities
	registry := ops.NewActivityRegistry()
	activities := opsactivity.GetAll()
	for _, act := range activities {
		err := registry.RegisterGeneric(act)
		require.NoError(t, err)
	}

	// Track all activity calls in order
	var activityCalls []map[string]interface{}

	// Register real activities with input capture
	env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			// Capture inputs for verification
			activityCalls = append(activityCalls, inputs)

			// Execute the real activity
			logger := createTestLogger()
			activityExecutor := executor.NewActivityExecutor(registry, logger)
			executorFunc := activityExecutor.CreateTemporalActivity("command_execution")
			return executorFunc.(func(context.Context, map[string]interface{}) (map[string]interface{}, error))(ctx, inputs)
		},
		activity.RegisterOptions{
			Name: "command_execution",
		},
	)

	// Create a recipe with multiple templates including step references
	recipe := &yamlpkg.RecipeDefinition{
		Name: "test-complex-templates",
		Sequence: []yamlpkg.Node{
			{
				ID: "step1",
				Op: "command_execution",
				Inputs: map[string]interface{}{
					"run": "echo {{ .Inputs.prefix }}: {{ .Inputs.message }}",
				},
				Outputs: map[string]interface{}{
					"stdout": "step1_output",
				},
			},
			{
				ID: "step2",
				Op: "command_execution",
				Inputs: map[string]interface{}{
					"run": "echo Received from step1",
				},
			},
		},
	}

	// Create compiler
	compilerRegistry := compiler.NewActivityRegistry()
	compilerRegistry.RegisterActivity("command_execution")
	comp := compiler.NewCompiler(compilerRegistry)

	// Create and register workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipe, comp)
	env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: recipe.Name,
		},
	)

	// Execute workflow
	inputs := map[string]interface{}{
		"prefix":  "INFO",
		"message": "Template test",
	}
	env.ExecuteWorkflow(recipe.Name, inputs)

	// Verify workflow completed
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify both activities were called with expanded templates
	require.Len(t, activityCalls, 2, "Should have called two activities")

	// First activity should have expanded input template
	step1Inputs := activityCalls[0]
	require.NotNil(t, step1Inputs)
	runCmd1, ok := step1Inputs["run"].(string)
	require.True(t, ok)
	assert.Equal(t, "echo INFO: Template test", runCmd1, "First template should be expanded")

	// Second activity should get its input
	step2Inputs := activityCalls[1]
	require.NotNil(t, step2Inputs)
	runCmd2, ok := step2Inputs["run"].(string)
	require.True(t, ok)
	assert.Equal(t, "echo Received from step1", runCmd2, "Second step should have its command")
}
