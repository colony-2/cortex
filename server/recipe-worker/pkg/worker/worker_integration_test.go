package worker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
	"github.com/vibethis/server/recipe-worker/pkg/compiler"
	recipeworkflows "github.com/vibethis/server/recipe-worker/pkg/workflows"
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
	// Create a simple workflow definition
	workflowDef := &yamlpkg.WorkflowDefinition{
		Name:        "test-workflow",
		Description: "Test workflow",
		Version:     "1.0",
		Inputs: []yamlpkg.InputDefinition{
			{Name: "message", Type: "string", Required: true},
		},
		Outputs: []yamlpkg.OutputDefinition{
			{Name: "result", Type: "string"},
		},
		Workflow: yamlpkg.WorkflowSpec{
			Type: "sequential",
			Steps: []yamlpkg.Step{
				{
					ID:       "echo",
					Activity: "echo-activity",
					Inputs: map[string]interface{}{
						"text": "{{ .Inputs.message }}",
					},
					Outputs: map[string]string{
						"echoed": "echo_result",
					},
				},
			},
			Outputs: map[string]string{
				"result": "{{ .Steps.echo.outputs.echoed }}",
			},
		},
	}

	// Create activity definition
	activityDef := &recipe.ActivityDefinition{
		Name:        "echo-activity",
		Description: "Echo activity",
		Timeout:     time.Minute,
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity(activityDef)
	comp := compiler.NewCompiler(registry)

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		recipeworkflows.CreateDynamicActivity(activityDef),
		activity.RegisterOptions{
			Name: "echo-activity",
		},
	)

	// Create and register the workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(workflowDef, nil, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "test-workflow",
		},
	)

	// Mock the activity - must be after RegisterWorkflow
	s.env.OnActivity("echo-activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"echoed": "Hello, World!"},
		nil,
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("test-workflow", map[string]interface{}{
		"message": "Hello, World!",
	})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("Hello, World!", result["result"])
}

func (s *WorkerIntegrationTestSuite) TestParallelWorkflowExecution() {
	// Create a workflow with parallel steps
	workflowDef := &yamlpkg.WorkflowDefinition{
		Name:    "parallel-workflow",
		Version: "1.0",
		Workflow: yamlpkg.WorkflowSpec{
			Type: "sequential",
			Steps: []yamlpkg.Step{
				{
					ID: "parallel-tasks",
					Parallel: []yamlpkg.Step{
						{
							ID:       "task1",
							Activity: "process-activity",
							Inputs: map[string]interface{}{
								"data": "data1",
							},
							Outputs: map[string]string{
								"result": "result1",
							},
						},
						{
							ID:       "task2",
							Activity: "process-activity",
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
			Outputs: map[string]string{
				"task1_result": "{{ .Steps.task1.outputs.result }}",
				"task2_result": "{{ .Steps.task2.outputs.result }}",
			},
		},
	}

	// Create activity definition
	activityDef := &recipe.ActivityDefinition{
		Name:    "process-activity",
		Timeout: time.Minute,
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity(activityDef)
	comp := compiler.NewCompiler(registry)

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		recipeworkflows.CreateDynamicActivity(activityDef),
		activity.RegisterOptions{
			Name: "process-activity",
		},
	)

	// Create and register the workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(workflowDef, nil, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "parallel-workflow",
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
	s.env.ExecuteWorkflow("parallel-workflow", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("processed-data1", result["task1_result"])
	s.Equal("processed-data2", result["task2_result"])
}

func (s *WorkerIntegrationTestSuite) TestWorkflowWithRetry() {
	// Create a workflow with retry policy
	workflowDef := &yamlpkg.WorkflowDefinition{
		Name:    "retry-workflow",
		Version: "1.0",
		Workflow: yamlpkg.WorkflowSpec{
			Type: "sequential",
			RetryPolicy: yamlpkg.RetryPolicy{
				InitialInterval: time.Second,
				MaximumAttempts: 3,
			},
			Steps: []yamlpkg.Step{
				{
					ID:       "flaky",
					Activity: "flaky-activity",
					Inputs: map[string]interface{}{
						"attempt": "1",
					},
					Outputs: map[string]string{
						"result": "output",
					},
				},
			},
			Outputs: map[string]string{
				"result": "{{ .Steps.flaky.outputs.result }}",
			},
		},
	}

	// Create activity definition
	activityDef := &recipe.ActivityDefinition{
		Name:    "flaky-activity",
		Timeout: time.Minute,
	}

	// Create compiler and registry
	registry := compiler.NewActivityRegistry()
	registry.RegisterActivity(activityDef)
	comp := compiler.NewCompiler(registry)

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		recipeworkflows.CreateDynamicActivity(activityDef),
		activity.RegisterOptions{
			Name: "flaky-activity",
		},
	)

	// Create and register the workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(workflowDef, nil, comp)
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "retry-workflow",
		},
	)

	// Mock the activity to fail twice then succeed - must be after RegisterWorkflow
	attemptCount := 0
	s.env.OnActivity("flaky-activity", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			attemptCount++
			if attemptCount < 3 {
				return nil, temporal.NewApplicationError("temporary failure", "TEMPORARY")
			}
			return map[string]interface{}{"result": "success after retries"}, nil
		},
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("retry-workflow", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("success after retries", result["result"])
}

func (s *WorkerIntegrationTestSuite) TestWorkerManagerWithMockClient() {
	logger := zaptest.NewLogger(s.T())

	// Create a test recipe
	_ = &recipe.Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Workflow: &yamlpkg.WorkflowDefinition{
			Name: "test-workflow",
			Workflow: yamlpkg.WorkflowSpec{
				Type: "sequential",
				Steps: []yamlpkg.Step{
					{
						ID:       "step1",
						Activity: "test-activity",
						Inputs: map[string]interface{}{
							"input": "test",
						},
					},
				},
			},
		},
		Activities: []yamlpkg.ActivityDefinition{
			{
				Name:        "test-activity",
				Description: "Test activity",
				Timeout:     time.Minute,
			},
		},
	}

	// Create WorkerManager with nil client for this test
	// In a real test we would use a mock client
	manager := NewWorkerManager(logger, nil)

	// Verify task queue naming
	taskQueue := manager.GetTaskQueueForRecipe("test-recipe")
	s.Equal("ono-recipes-test-recipe", taskQueue)

	// Test worker status
	status := manager.GetWorkerStatus("test-recipe")
	s.Equal(recipe.WorkerStatusStopped, status)
}
