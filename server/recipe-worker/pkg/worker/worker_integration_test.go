package worker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	recipeworkflows "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/workflows"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap/zaptest"
)

type WorkerIntegrationTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *WorkerIntegrationTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *WorkerIntegrationTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func TestWorkerIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerIntegrationTestSuite))
}

func (s *WorkerIntegrationTestSuite) TestSimpleWorkflowExecution() {
	// Create a unified recipe definition
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:        "test-recipe",
		Description: "Test recipe",
		Version:     "1.0",
		Steps: []yamlpkg.Step{
			{
				ID:   "echo",
				Uses: "echo-activity",
				Inputs: map[string]interface{}{
					"text": "{{ .Inputs.message }}",
				},
				Outputs: map[string]string{
					"echoed": "echo_result",
				},
			},
		},
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity("echo-activity")
	comp := compiler.NewCompiler(registry)

	// Create mock activity function
	echoActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"echo_result": inputs["text"],
		}, nil
	}

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		echoActivityFunc,
		activity.RegisterOptions{
			Name: "echo-activity",
		},
	)

	// Create and register the workflow using the compiler
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipeDef, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "test-recipe",
		},
	)

	// Mock the activity - must be after RegisterWorkflow
	s.env.OnActivity("echo-activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"echo_result": "Hello, World!"},
		nil,
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("test-recipe", map[string]interface{}{
		"message": "Hello, World!",
	})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestParallelWorkflowExecution() {
	// Create a recipe with parallel steps
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:    "parallel-recipe",
		Version: "1.0",
		Steps: []yamlpkg.Step{
			{
				ID: "parallel-tasks",
				Parallel: &yamlpkg.ParallelSpec{
					Steps: []yamlpkg.Step{
						{
							ID:   "task1",
							Uses: "process-activity",
							Inputs: map[string]interface{}{
								"data": "data1",
							},
							Outputs: map[string]string{
								"result": "result1",
							},
						},
						{
							ID:   "task2",
							Uses: "process-activity",
							Inputs: map[string]interface{}{
								"data": "data2",
							},
							Outputs: map[string]string{
								"result": "result2",
							},
						},
					},
				},
			},
		},
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity("process-activity")
	comp := compiler.NewCompiler(registry)

	// Create mock activity function
	processActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		data := inputs["data"].(string)
		return map[string]interface{}{
			"result": "processed-" + data,
		}, nil
	}

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		processActivityFunc,
		activity.RegisterOptions{
			Name: "process-activity",
		},
	)

	// Create and register the workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipeDef, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "parallel-recipe",
		},
	)

	// Mock the activity calls - must be after RegisterWorkflow
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{"data": "data1"}).Return(
		map[string]interface{}{"result": "processed-data1"},
		nil,
	)
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{"data": "data2"}).Return(
		map[string]interface{}{"result": "processed-data2"},
		nil,
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("parallel-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestSharedActivityWorkflow() {
	// Create a recipe with shared activities
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:        "shared-recipe",
		Description: "Shared activity test recipe",
		Version:     "1.0",
		Shared: map[string]yamlpkg.SharedActivity{
			"my_processor": {
				Uses: "process-data",
				Config: map[string]interface{}{
					"type":    "function",
					"timeout": "30s",
				},
			},
		},
		Steps: []yamlpkg.Step{
			{
				ID:   "analyze",
				Uses: "shared/my_processor",
				Inputs: map[string]interface{}{
					"input": "test data",
				},
			},
		},
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity("process-data")
	registry.RegisterActivity("shared/my_processor")
	comp := compiler.NewCompiler(registry)

	// Create mock activity function
	processActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"result": "processed: " + inputs["input"].(string),
		}, nil
	}

	// Register shared activity with name
	s.env.RegisterActivityWithOptions(
		processActivityFunc,
		activity.RegisterOptions{
			Name: "shared/my_processor",
		},
	)

	// Create and register the workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipeDef, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "shared-recipe",
		},
	)

	// Mock the activity call
	s.env.OnActivity("shared/my_processor", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "processed: test data"},
		nil,
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("shared-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestWorkflowWithRetry() {
	// Create a workflow with retry functionality
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:    "retry-recipe",
		Version: "1.0",
		Steps: []yamlpkg.Step{
			{
				ID:   "flaky",
				Uses: "flaky-activity",
				Inputs: map[string]interface{}{
					"attempt": "1",
				},
				Outputs: map[string]string{
					"result": "output",
				},
			},
		},
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity("flaky-activity")
	comp := compiler.NewCompiler(registry)

	// Create mock activity function that fails first time
	attemptCount := 0
	flakyActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		attemptCount++
		if attemptCount == 1 {
			return nil, temporal.NewApplicationError("temporary failure", "TEMPORARY")
		}
		return map[string]interface{}{"result": "success after retry"}, nil
	}

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		flakyActivityFunc,
		activity.RegisterOptions{
			Name: "flaky-activity",
		},
	)

	// Create and register the workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipeDef, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "retry-recipe",
		},
	)

	// Mock the activity to fail first then succeed
	callCount := 0
	s.env.OnActivity("flaky-activity", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			callCount++
			if callCount == 1 {
				return nil, temporal.NewApplicationError("temporary failure", "TEMPORARY")
			}
			return map[string]interface{}{"result": "success after retry"}, nil
		},
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("retry-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestWorkerManagerWithMockClient() {
	logger := zaptest.NewLogger(s.T())

	// Create a test recipe using unified format
	testRecipe := &recipe.Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Recipe: &yamlpkg.RecipeDefinition{
			Name:    "test-recipe",
			Version: "1.0.0",
			Steps: []yamlpkg.Step{
				{
					ID:   "step1",
					Uses: "test-activity",
					Inputs: map[string]interface{}{
						"input": "test",
					},
				},
			},
		},
	}

	// Create WorkerManager with nil client for this test
	// In a real test we would use a mock client
	manager := NewWorkerManager(logger, nil)

	// Verify task queue naming
	taskQueue := manager.GetTaskQueueForRecipe("test-recipe")
	s.Equal("ono-recipes-test-recipe", taskQueue)

	// Test worker status for non-existent worker
	status := manager.GetWorkerStatus("test-recipe")
	s.Equal(recipe.WorkerStatusStopped, status)

	// Test error case for stopping non-existent worker
	err := manager.StopWorker("non-existent")
	s.Error(err)
	s.Contains(err.Error(), "worker not found")

	// Verify the recipe was created correctly
	s.NotNil(testRecipe)
	s.NotNil(testRecipe.Recipe)
	s.Equal("test-recipe", testRecipe.Name)
	s.Equal("1.0.0", testRecipe.Version)
}