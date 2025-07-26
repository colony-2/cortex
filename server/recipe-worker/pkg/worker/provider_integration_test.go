package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap/zaptest"
	
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	recipeworker "github.com/divisive-ai/vibethis/server/recipe-worker"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	recipeworkflows "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/workflows"
)

// ProviderIntegrationTestSuite tests the integration of custom providers with workflows
type ProviderIntegrationTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func TestProviderIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderIntegrationTestSuite))
}

func (s *ProviderIntegrationTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *ProviderIntegrationTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// CustomMathProvider implements a custom math activity provider
type CustomMathProvider struct{}

func (p *CustomMathProvider) GetType() string {
	return "math"
}

func (p *CustomMathProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("expected 2 arguments")
	}
	
	config := args[0].(map[string]interface{})
	inputs := args[1].(map[string]interface{})
	
	operation := config["operation"].(string)
	
	// Convert inputs to float64, handling both float64 and string types
	var a, b float64
	
	switch v := inputs["a"].(type) {
	case float64:
		a = v
	case string:
		n, err := fmt.Sscanf(v, "%f", &a)
		if err != nil || n != 1 {
			return nil, fmt.Errorf("invalid numeric value for 'a': %v", v)
		}
	case int:
		a = float64(v)
	default:
		return nil, fmt.Errorf("invalid type for 'a': %T", v)
	}
	
	switch v := inputs["b"].(type) {
	case float64:
		b = v
	case string:
		n, err := fmt.Sscanf(v, "%f", &b)
		if err != nil || n != 1 {
			return nil, fmt.Errorf("invalid numeric value for 'b': %v", v)
		}
	case int:
		b = float64(v)
	default:
		return nil, fmt.Errorf("invalid type for 'b': %T", v)
	}
	
	var result float64
	switch operation {
	case "add":
		result = a + b
	case "multiply":
		result = a * b
	case "subtract":
		result = a - b
	case "divide":
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		result = a / b
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
	
	return map[string]interface{}{
		"result": result,
	}, nil
}

// Implement schema interface for automatic registration
func (p *CustomMathProvider) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
	configSchema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type": "string",
				"enum": []string{"add", "subtract", "multiply", "divide"},
			},
		},
		"required": []string{"operation"},
	}
	
	inputSchema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"a": map[string]interface{}{"type": "number"},
			"b": map[string]interface{}{"type": "number"},
		},
		"required": []string{"a", "b"},
	}
	
	outputSchema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"result": map[string]interface{}{"type": "number"},
		},
		"required": []string{"result"},
	}
	
	return
}

func (p *CustomMathProvider) GetDescription() string {
	return "Performs mathematical operations"
}

func (p *CustomMathProvider) GetSchemaOptions() recipeworker.SchemaOptions {
	return recipeworker.SchemaOptions{
		RequiredConfig: true,
	}
}

func (s *ProviderIntegrationTestSuite) TestCustomProviderWorkflow() {
	// Create worker manager
	logger := zaptest.NewLogger(s.T())
	workerManager := worker.NewWorkerManager(logger, nil) // nil client is OK for testing
	
	// Register custom math provider
	mathProvider := &CustomMathProvider{}
	err := workerManager.RegisterProvider(mathProvider)
	s.NoError(err)
	
	// Verify provider was registered in both registries
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	_, exists := activityTypeRegistry.GetActivityType("math")
	s.True(exists)
	
	// Create a recipe that uses the custom math activity
	testRecipe := &recipe.Recipe{
		Name:    "math-workflow",
		Version: "1.0.0",
		Activities: []yamlpkg.ActivityDefinition{
			{
				Name:        "calculate-sum",
				Description: "Add two numbers",
				Timeout:     10 * time.Second,
				Implementation: yamlpkg.ActivityImplementation{
					Type: "math",
					Config: map[string]interface{}{
						"operation": "add",
					},
				},
			},
			{
				Name:        "calculate-product",
				Description: "Multiply two numbers",
				Timeout:     10 * time.Second,
				Implementation: yamlpkg.ActivityImplementation{
					Type: "math",
					Config: map[string]interface{}{
						"operation": "multiply",
					},
				},
			},
		},
	}
	
	// Create workflow definition
	project := &yamlpkg.Project{
		Activities: testRecipe.Activities,
		Workflow: &yamlpkg.WorkflowDefinition{
			Name: "math-calculation",
			Workflow: yamlpkg.WorkflowSpec{
				Type: "sequential",
				Steps: []yamlpkg.Step{
					{
						ID:       "step1",
						Activity: "calculate-sum",
						Inputs: map[string]interface{}{
							"a": "{{ .Inputs.x }}",
							"b": "{{ .Inputs.y }}",
						},
					},
					{
						ID:       "step2",
						Activity: "calculate-product",
						Inputs: map[string]interface{}{
							"a": "{{ .Steps.step1.outputs.result }}",
							"b": "{{ .Inputs.z }}",
						},
					},
				},
				Outputs: map[string]string{
					"sum":          "{{ .Steps.step1.outputs.result }}",
					"final_result": "{{ .Steps.step2.outputs.result }}",
				},
			},
		},
	}
	
	// Register activities with the test environment
	activityRegistry := compiler.NewActivityRegistry()
	for i := range testRecipe.Activities {
		activityRegistry.RegisterActivity(&testRecipe.Activities[i])
	}
	
	// Create activity implementations using the provider
	for _, actDef := range testRecipe.Activities {
		actDef := actDef // capture
		activityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
					result, err := mathProvider.Execute(ctx, actDef.Implementation.Config, inputs)
			if err != nil {
				return nil, err
			}
			return result.(map[string]interface{}), nil
		}
		// Register with activity name
		s.env.RegisterActivityWithOptions(activityFunc, activity.RegisterOptions{
			Name: actDef.Name,
		})
	}
	
	// Create and register workflow
	comp := compiler.NewCompiler(activityRegistry)
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(project.Workflow, project, comp)
	s.env.RegisterWorkflow(workflowFunc)
	
	// Execute workflow with inputs
	inputs := map[string]interface{}{
		"x": 10.0,
		"y": 5.0,
		"z": 3.0,
	}
	
	s.env.ExecuteWorkflow(workflowFunc, inputs)
	
	// Verify results
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	
	// x=10, y=5, sum=15
	// sum=15, z=3, product=45
	s.Equal("15", result["sum"])
	s.Equal("45", result["final_result"])
}

func (s *ProviderIntegrationTestSuite) TestTypedProviderWorkflow() {
	// Test using typed provider
	type StringConfig struct {
		Prefix string `json:"prefix"`
		Suffix string `json:"suffix"`
	}
	
	type StringInput struct {
		Text string `json:"text"`
	}
	
	type StringOutput struct {
		Result string `json:"result"`
	}
	
	// Create typed provider with schema
	stringProvider := recipeworker.NewTypedProviderWithSchema(
		"string_transform",
		"Transforms strings with prefix and suffix",
		func(ctx context.Context, config StringConfig, input StringInput) (StringOutput, error) {
			return StringOutput{
				Result: config.Prefix + input.Text + config.Suffix,
			}, nil
		},
		recipeworker.WithConfigSchema[StringConfig, StringInput, StringOutput](map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prefix": map[string]interface{}{"type": "string"},
				"suffix": map[string]interface{}{"type": "string"},
			},
			"required": []string{"prefix", "suffix"},
		}),
		recipeworker.WithInputSchema[StringConfig, StringInput, StringOutput](map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{"type": "string"},
			},
			"required": []string{"text"},
		}),
		recipeworker.WithOutputSchema[StringConfig, StringInput, StringOutput](map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"result": map[string]interface{}{"type": "string"},
			},
			"required": []string{"result"},
		}),
	)
	
	// Create worker manager and register provider
	logger := zaptest.NewLogger(s.T())
	workerManager := worker.NewWorkerManager(logger, nil)
	
	err := workerManager.RegisterProvider(stringProvider)
	s.NoError(err)
	
	// Verify type was registered
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	_, exists := activityTypeRegistry.GetActivityType("string_transform")
	s.True(exists)
	
	// Create simple workflow
	activityDef := &yamlpkg.ActivityDefinition{
		Name: "transform-text",
		Timeout: 10 * time.Second,
		Implementation: yamlpkg.ActivityImplementation{
			Type: "string_transform",
			Config: map[string]interface{}{
				"prefix": "Hello, ",
				"suffix": "!",
			},
		},
	}
	
	// Register activity with JSON marshaling wrapper
	activityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Convert maps to typed structs using JSON marshaling
		configBytes, err := json.Marshal(activityDef.Implementation.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		var config StringConfig
		if err := json.Unmarshal(configBytes, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		
		inputBytes, err := json.Marshal(inputs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input: %w", err)
		}
		var input StringInput
		if err := json.Unmarshal(inputBytes, &input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input: %w", err)
		}
		
		result, err := stringProvider.Execute(ctx, config, input)
		if err != nil {
			return nil, err
		}
		// Convert typed result to map
		output := result.(StringOutput)
		return map[string]interface{}{"result": output.Result}, nil
	}
	
	s.env.RegisterActivity(activityFunc)
	
	// Simple workflow that calls the activity
	simpleWorkflow := func(ctx workflow.Context, input string) (string, error) {
		// Set activity options
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Second,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		var result map[string]interface{}
		err := workflow.ExecuteActivity(ctx, activityFunc, map[string]interface{}{
			"text": input,
		}).Get(ctx, &result)
		if err != nil {
			return "", err
		}
		return result["result"].(string), nil
	}
	
	s.env.RegisterWorkflow(simpleWorkflow)
	s.env.ExecuteWorkflow(simpleWorkflow, "World")
	
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result string
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("Hello, World!", result)
}