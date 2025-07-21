package worker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
	"github.com/vibethis/server/recipe-worker/pkg/compiler"
	recipeworkflows "github.com/vibethis/server/recipe-worker/pkg/workflows"
)

// WorkflowExecutionTestSuite tests actual workflow execution
type WorkflowExecutionTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *WorkflowExecutionTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *WorkflowExecutionTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func TestWorkflowExecutionTestSuite(t *testing.T) {
	suite.Run(t, new(WorkflowExecutionTestSuite))
}

func (s *WorkflowExecutionTestSuite) TestSimpleWorkflowExecution() {
	// Create compiler
	activityRegistry := compiler.NewActivityRegistry()
	comp := compiler.NewCompiler(activityRegistry)
	
	// Create a proper Project structure
	project := &yamlpkg.Project{
		Workflow: &yamlpkg.WorkflowDefinition{
			Name:    "test-workflow",
			Version: "1.0.0",
			Workflow: yamlpkg.WorkflowSpec{
				Type: "sequential",
				Steps: []yamlpkg.Step{
					{
						ID:       "step1",
						Activity: "process-data",
						Inputs: map[string]interface{}{
							"data": "test-input",
						},
						Outputs: map[string]string{
							"result": "processed",
						},
					},
				},
				Outputs: map[string]string{
					"final_result": "{{ .Steps.step1.outputs.processed }}",
				},
			},
		},
		Activities: []yamlpkg.ActivityDefinition{
			{
				Name:    "process-data",
				Timeout: 30 * time.Second,
			},
		},
	}
	
	// Register activities with the compiler registry
	for i := range project.Activities {
		activityRegistry.RegisterActivity(&project.Activities[i])
	}
	
	// Create dynamic workflow
	dynamicWorkflow := recipeworkflows.CreateDynamicWorkflow(project.Workflow, project, comp)
	
	// Register workflow with test environment
	s.env.RegisterWorkflow(dynamicWorkflow)
	
	// Register activities - the test framework needs them registered even when mocking
	for _, actDef := range project.Activities {
		actDef := actDef // capture loop variable
		s.env.RegisterActivityWithOptions(
			recipeworkflows.CreateDynamicActivity(&actDef),
			activity.RegisterOptions{
				Name: actDef.Name,
			},
		)
	}
	
	// Mock the activity
	s.env.OnActivity("process-data", mock.Anything, map[string]interface{}{
		"data": "test-input",
	}).Return(map[string]interface{}{
		"processed": "processed-test-input",  // This gets mapped to "result" via outputs mapping
	}, nil)
	
	// Execute workflow
	s.env.ExecuteWorkflow(dynamicWorkflow, map[string]interface{}{})
	
	// Verify results
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("processed-test-input", result["final_result"])
}

func (s *WorkflowExecutionTestSuite) TestParallelWorkflowExecution() {
	// Create compiler
	activityRegistry := compiler.NewActivityRegistry()
	comp := compiler.NewCompiler(activityRegistry)
	
	// Create a workflow with parallel steps
	project := &yamlpkg.Project{
		Workflow: &yamlpkg.WorkflowDefinition{
			Name:    "parallel-workflow",
			Version: "1.0",
			Workflow: yamlpkg.WorkflowSpec{
				Type: "sequential",
				Steps: []yamlpkg.Step{
					{
						ID: "parallel-group",
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
					"task1_result": "{{ .Steps.task1.outputs.result1 }}",
					"task2_result": "{{ .Steps.task2.outputs.result2 }}",
				},
			},
		},
		Activities: []yamlpkg.ActivityDefinition{
			{
				Name:    "process-activity",
				Timeout: time.Minute,
			},
		},
	}
	
	// Register activities with the compiler registry
	for i := range project.Activities {
		activityRegistry.RegisterActivity(&project.Activities[i])
	}
	
	// Create dynamic workflow
	dynamicWorkflow := recipeworkflows.CreateDynamicWorkflow(project.Workflow, project, comp)
	
	// Register workflow
	s.env.RegisterWorkflow(dynamicWorkflow)
	
	// Register activities - the test framework needs them registered even when mocking
	for _, actDef := range project.Activities {
		actDef := actDef // capture loop variable
		s.env.RegisterActivityWithOptions(
			recipeworkflows.CreateDynamicActivity(&actDef),
			activity.RegisterOptions{
				Name: actDef.Name,
			},
		)
	}
	
	// Mock the activity calls
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{"data": "data1"}).Return(
		map[string]interface{}{"result1": "processed-data1"},
		nil,
	)
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{"data": "data2"}).Return(
		map[string]interface{}{"result2": "processed-data2"},
		nil,
	)
	
	// Execute workflow
	s.env.ExecuteWorkflow(dynamicWorkflow, map[string]interface{}{})
	
	// Verify results
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("processed-data1", result["task1_result"])
	s.Equal("processed-data2", result["task2_result"])
}

func (s *WorkflowExecutionTestSuite) TestWorkflowWithTemplateInputs() {
	// Create compiler
	activityRegistry := compiler.NewActivityRegistry()
	comp := compiler.NewCompiler(activityRegistry)
	
	// Create workflow with template resolution
	project := &yamlpkg.Project{
		Workflow: &yamlpkg.WorkflowDefinition{
			Name:    "template-workflow",
			Version: "1.0",
			Inputs: []yamlpkg.InputDefinition{
				{Name: "message", Type: "string", Required: true},
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
					{
						ID:       "process",
						Activity: "process-activity",
						Inputs: map[string]interface{}{
							"input": "{{ .Steps.echo.outputs.echo_result }}",
						},
						Outputs: map[string]string{
							"processed": "final_result",
						},
					},
				},
				Outputs: map[string]string{
					"result": "{{ .Steps.process.outputs.final_result }}",
				},
			},
		},
		Activities: []yamlpkg.ActivityDefinition{
			{Name: "echo-activity", Timeout: time.Minute},
			{Name: "process-activity", Timeout: time.Minute},
		},
	}
	
	// Register activities with the compiler registry
	for i := range project.Activities {
		activityRegistry.RegisterActivity(&project.Activities[i])
	}
	
	// Create dynamic workflow
	dynamicWorkflow := recipeworkflows.CreateDynamicWorkflow(project.Workflow, project, comp)
	
	// Register workflow
	s.env.RegisterWorkflow(dynamicWorkflow)
	
	// Register activities - the test framework needs them registered even when mocking
	for _, actDef := range project.Activities {
		actDef := actDef // capture loop variable
		s.env.RegisterActivityWithOptions(
			recipeworkflows.CreateDynamicActivity(&actDef),
			activity.RegisterOptions{
				Name: actDef.Name,
			},
		)
	}
	
	// Mock activities
	s.env.OnActivity("echo-activity", mock.Anything, map[string]interface{}{
		"text": "Hello, World!",
	}).Return(map[string]interface{}{
		"echo_result": "Echo: Hello, World!",
	}, nil)
	
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{
		"input": "Echo: Hello, World!",
	}).Return(map[string]interface{}{
		"final_result": "Processed: Echo: Hello, World!",
	}, nil)
	
	// Execute workflow with inputs
	s.env.ExecuteWorkflow(dynamicWorkflow, map[string]interface{}{
		"message": "Hello, World!",
	})
	
	// Verify results
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("Processed: Echo: Hello, World!", result["result"])
}

// TestCompleteIntegration tests the full flow from recipe to workflow execution
func (s *WorkflowExecutionTestSuite) TestCompleteIntegration() {
	// Create a complete recipe definition
	recipe := &yamlpkg.Project{
		Workflow: &yamlpkg.WorkflowDefinition{
			Name:        "integration-workflow",
			Description: "Complete integration test",
			Version:     "1.0",
			Inputs: []yamlpkg.InputDefinition{
				{
					Name:        "data",
					Type:        "string",
					Required:    true,
					Description: "Input data to process",
				},
			},
			Outputs: []yamlpkg.OutputDefinition{
				{
					Name:        "result",
					Type:        "string",
					Description: "Processed result",
				},
			},
			Workflow: yamlpkg.WorkflowSpec{
				Type: "sequential",
				RetryPolicy: yamlpkg.RetryPolicy{
					InitialInterval:  time.Second,
					MaximumAttempts:  3,
					BackoffCoefficient: 2.0,
				},
				Steps: []yamlpkg.Step{
					{
						ID:       "validate",
						Activity: "validation-activity",
						Inputs: map[string]interface{}{
							"data": "{{ .Inputs.data }}",
						},
						Outputs: map[string]string{
							"validated": "validated_data",
						},
					},
					{
						ID:       "transform",
						Activity: "transformation-activity",
						Inputs: map[string]interface{}{
							"input": "{{ .Steps.validate.outputs.validated_data }}",
						},
						Outputs: map[string]string{
							"transformed": "transformed_data",
						},
					},
				},
				Outputs: map[string]string{
					"result": "{{ .Steps.transform.outputs.transformed_data }}",
				},
			},
		},
		Activities: []yamlpkg.ActivityDefinition{
			{
				Name:        "validation-activity",
				Description: "Validates input data",
				Timeout:     30 * time.Second,
			},
			{
				Name:        "transformation-activity",
				Description: "Transforms validated data",
				Timeout:     60 * time.Second,
			},
		},
	}
	
	// Create compiler
	activityRegistry := compiler.NewActivityRegistry()
	comp := compiler.NewCompiler(activityRegistry)
	
	// Register activities with the compiler registry
	for i := range recipe.Activities {
		activityRegistry.RegisterActivity(&recipe.Activities[i])
	}
	
	// Create dynamic workflow
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipe.Workflow, recipe, comp)
	
	// Register workflow with options
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: recipe.Workflow.Name,
		},
	)
	
	// Register activities - the test framework needs them registered even when mocking
	for _, actDef := range recipe.Activities {
		actDef := actDef // capture loop variable
		s.env.RegisterActivityWithOptions(
			recipeworkflows.CreateDynamicActivity(&actDef),
			activity.RegisterOptions{
				Name: actDef.Name,
			},
		)
	}
	
	// Mock activities
	s.env.OnActivity("validation-activity", mock.Anything, map[string]interface{}{
		"data": "test-data",
	}).Return(map[string]interface{}{
		"validated_data": "valid:test-data",
	}, nil)
	
	s.env.OnActivity("transformation-activity", mock.Anything, map[string]interface{}{
		"input": "valid:test-data",
	}).Return(map[string]interface{}{
		"transformed_data": "TRANSFORMED[valid:test-data]",
	}, nil)
	
	// Execute workflow by name
	s.env.ExecuteWorkflow("integration-workflow", map[string]interface{}{
		"data": "test-data",
	})
	
	// Verify completion
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())
	
	// Check result
	var result map[string]interface{}
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	assert.Equal(s.T(), "TRANSFORMED[valid:test-data]", result["result"])
}