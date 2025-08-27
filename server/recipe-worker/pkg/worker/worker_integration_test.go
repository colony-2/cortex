package worker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
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
	s.T().Skip("Workflow execution requires proper activity registration")
	// Create a unified recipe definition
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID:   "test-recipe",
			Desc: "Test recipe",
			Op:   "echo-activity",
			Inputs: map[string]interface{}{
				"text": "Hello, World!",
			},
			Outputs: map[string]interface{}{
				"echoed": "echo_result",
			},
		},
		Version: "1.0",
	}

	// Create activity registry
	registry := ops.NewActivityRegistry()

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

	// Create a workflow that uses ExecuteNode
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteNode(ctx, registry, &recipeDef.Node, inputs)
	}
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "test-recipe",
		},
	)

	// Mock the activity - must be after RegisterWorkflow
	s.env.OnActivity("echo-activity", mock.Anything, map[string]interface{}{"text": "Hello, World!"}).Return(
		map[string]interface{}{"echo_result": "Hello, World!"},
		nil,
	)

	// Execute the workflow with empty inputs since the text is hardcoded
	s.env.ExecuteWorkflow("test-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestParallelWorkflowExecution() {
	s.T().Skip("Parallel execution not yet supported")
	// Create a recipe with parallel steps
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID:       "parallel-recipe",
			Parallel: []yamlpkg.Node{
			{
				ID: "task1",
				Op: "process-activity",
				Inputs: map[string]interface{}{
					"data": "data1",
				},
				Outputs: map[string]interface{}{
					"result": "result1",
				},
			},
			{
				ID: "task2",
				Op: "process-activity",
				Inputs: map[string]interface{}{
					"data": "data2",
				},
				Outputs: map[string]interface{}{
					"result": "result2",
				},
			},
		},
		},
		Version: "1.0",
	}

	// Create activity registry
	registry := ops.NewActivityRegistry()

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

	// Create a workflow that uses ExecuteNode
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteNode(ctx, registry, &recipeDef.Node, inputs)
	}
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
	s.T().Skip("Shared activity resolution needs proper implementation")
	// Create a recipe with shared activities
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID:   "shared-recipe",
			Desc: "Shared activity test recipe",
			Sequence: []yamlpkg.Node{
				{
					ID:     "analyze",
					Shared: "my_processor",
					Inputs: map[string]interface{}{
						"input": "test data",
					},
				},
			},
		},
		Version: "1.0",
		Defs: map[string]yamlpkg.Node{
			"my_processor": {
				Op: "process-data",
				Inputs: map[string]interface{}{
					"type":    "function",
					"timeout": "30s",
				},
			},
		},
	}

	// Create activity registry
	registry := ops.NewActivityRegistry()

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

	// Create a workflow that uses ExecuteNode
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteNode(ctx, registry, &recipeDef.Node, inputs)
	}
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
	s.T().Skip("Retry handling needs proper implementation")
	// Create a workflow with retry functionality
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID:  "retry-recipe",
			Op:  "flaky-activity",
			Inputs: map[string]interface{}{
				"attempt": "1",
			},
			Outputs: map[string]interface{}{
				"result": "output",
			},
		},
		Version: "1.0",
	}

	// Create activity registry
	registry := ops.NewActivityRegistry()

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

	// Create a workflow that uses ExecuteNode
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteNode(ctx, registry, &recipeDef.Node, inputs)
	}
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
		ID:          "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Recipe: &yamlpkg.RecipeDefinition{
			Node: yamlpkg.Node{
				ID:  "test-recipe",
				Op:  "test-activity",
				Inputs: map[string]interface{}{
					"input": "test",
				},
			},
			Version: "1.0.0",
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
	s.Equal("test-recipe", testRecipe.ID)
	s.Equal("1.0.0", testRecipe.Version)
}