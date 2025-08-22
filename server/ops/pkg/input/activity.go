package input

import (
	"context"
	"fmt"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Config represents the configuration for the input activity
type Config struct {
	// Single question format
	Question string       `json:"question,omitempty" jsonschema:"description=Question to ask the user"`
	Type     FieldType    `json:"type,omitempty" jsonschema:"enum=short_answer|paragraph_text|multiple_choice|checkboxes|dropdown|linear_scale|date|time,description=Input field type"`
	Options  []Option     `json:"options,omitempty" jsonschema:"description=Options for choice fields"`
	Scale    *LinearScale `json:"scale,omitempty" jsonschema:"description=Configuration for linear scale fields"`

	// Multi-field format
	Title   string      `json:"title,omitempty" jsonschema:"description=Form title"`
	Fields  []FormField `json:"fields,omitempty" jsonschema:"description=Form fields"`
	Context FormContext `json:"context,omitempty" jsonschema:"description=Form context and artifacts"`
	Timeout int         `json:"timeout,omitempty" jsonschema:"default=300,minimum=1,maximum=3600,description=Timeout in seconds"`

	// Default value on timeout
	DefaultOnTimeout interface{} `json:"default_on_timeout,omitempty" jsonschema:"description=Default value to return if input times out"`
}

// Input represents the inputs passed to the input activity
type Input struct {
	BoxID      string                 `json:"box_id" jsonschema:"required,description=Box identifier"`
	ActivityID string                 `json:"activity_id" jsonschema:"required,description=Activity identifier"`
	Context    map[string]interface{} `json:"context,omitempty" jsonschema:"description=Additional context data"`
}

// Output represents the output from the input activity
type Output struct {
	Response interface{}            `json:"response,omitempty" jsonschema:"description=User response for single question"`
	Fields   map[string]interface{} `json:"fields,omitempty" jsonschema:"description=User responses for multi-field form"`
	UserID   string                 `json:"user_id,omitempty" jsonschema:"description=ID of user who responded"`
	Metadata map[string]interface{} `json:"metadata,omitempty" jsonschema:"description=Additional metadata"`
}

// InputActivity is a RegisterableOp that collects user input via forms
type InputActivity struct {
	// This will be injected by the framework when running in a workflow context
	temporalContext workflow.Context
	// Management service for HTTP endpoints
	managementService ManagementService
}

// NewInputActivity creates a new input activity instance
func NewInputActivity() *InputActivity {
	return &InputActivity{
		managementService: NewInputManagementService(),
	}
}

// GetMetadata returns activity metadata for registration
func (a *InputActivity) GetMetadata() types.OpMetadata {
	return types.OpMetadata{
		Type:           "input",
		Name:           "User Input Collection",
		Description:    "Collects user input through interactive forms with support for various field types",
		Version:        "1.0.0",
		DefaultTimeout: 5 * time.Minute,
		RetryPolicy: &types.RetryPolicy{
			MaximumAttempts:    1, // Don't retry user inputs
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			NonRetryableErrorTypes: []string{
				"InputTimeout",
				"UserCancelled",
			},
		},
	}
}

// Execute runs the input activity
func (a *InputActivity) Execute(ctx context.Context, config Config, input Input) (Output, error) {
	// Build the form from config
	form := a.buildForm(config, input)

	// Set default timeout if not specified
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// If we have a temporal context (running in workflow), use child workflow
	// Otherwise, we're in standalone mode for testing
	if a.temporalContext != nil {
		return a.executeWithWorkflow(form, timeout, config, input)
	}

	// Standalone mode - just return a mock response for testing
	return a.executeMockResponse(config)
}

// SetTemporalContext sets the workflow context for the activity
// This is called by the framework when the activity is registered
func (a *InputActivity) SetTemporalContext(ctx workflow.Context) {
	a.temporalContext = ctx
}

// GetManagementService returns the management service for HTTP endpoints
// This implements the ManagementServiceProvider interface
func (a *InputActivity) GetManagementService() ManagementService {
	return a.managementService
}

// buildForm constructs the InputForm from config and input
func (a *InputActivity) buildForm(config Config, input Input) InputForm {
	form := InputForm{
		Timeout: time.Duration(config.Timeout) * time.Second,
	}

	// Check if it's a single question or multi-field form
	if config.Question != "" {
		// Single question format
		form.Question = config.Question
		form.Type = config.Type
		form.Options = config.Options
		form.Scale = config.Scale
	} else if len(config.Fields) > 0 {
		// Multi-field format
		form.Title = config.Title
		form.Fields = config.Fields
	}

	// Add context
	form.Context = config.Context

	// Process artifacts from input context if needed
	if form.Context.ArtifactsFromOutput != "" {
		// This would resolve artifacts from previous activity outputs
		// For now, we'll leave this as a placeholder
	}

	return form
}

// executeWithWorkflow executes the input collection using a child workflow
func (a *InputActivity) executeWithWorkflow(form InputForm, timeout time.Duration, config Config, input Input) (Output, error) {
	// Generate unique workflow ID
	childID := fmt.Sprintf("input-%s-%d", workflow.GetInfo(a.temporalContext).WorkflowExecution.ID, time.Now().Unix())

	// Configure child workflow options
	childOptions := workflow.ChildWorkflowOptions{
		WorkflowID: childID,
		TaskQueue:  "input-handlers",
		SearchAttributes: map[string]interface{}{
			"InputWorkflowType": "user-input",
			"ParentWorkflowID":  workflow.GetInfo(a.temporalContext).WorkflowExecution.ID,
			"InputStatus":       "pending",
		},
	}

	wfCtx := workflow.WithChildOptions(a.temporalContext, childOptions)

	// Prepare workflow parameters
	inputParams := InputWorkflowParams{
		Form:       form,
		Timeout:    timeout,
		BoxID:      input.BoxID,
		ActivityID: input.ActivityID,
	}

	// Execute child workflow and wait for result
	var result InputWorkflowResult
	err := workflow.ExecuteChildWorkflow(wfCtx, "InputCollectionWorkflow", inputParams).Get(wfCtx, &result)

	if err != nil {
		// Handle timeout with default value if configured
		if config.DefaultOnTimeout != nil && temporal.IsTimeoutError(err) {
			if config.Question != "" {
				// Single question format
				return Output{Response: config.DefaultOnTimeout}, nil
			}
			// Multi-field format - return default as fields
			if fields, ok := config.DefaultOnTimeout.(map[string]interface{}); ok {
				return Output{Fields: fields}, nil
			}
		}
		return Output{}, err
	}

	// Convert workflow result to activity output
	output := Output{
		UserID:   result.UserID,
		Metadata: result.Metadata,
	}

	// Determine if it's single question or multi-field response
	if config.Question != "" {
		// Single question - extract single response value
		if val, ok := result.FormResponse["response"]; ok {
			output.Response = val
		}
	} else {
		// Multi-field - return all fields
		output.Fields = result.FormResponse
	}

	return output, nil
}

// executeMockResponse returns a mock response for testing
func (a *InputActivity) executeMockResponse(config Config) (Output, error) {
	output := Output{
		UserID: "test-user",
		Metadata: map[string]interface{}{
			"test": true,
		},
	}

	if config.Question != "" {
		// Single question mock response
		switch config.Type {
		case FieldTypeMultipleChoice:
			if len(config.Options) > 0 {
				output.Response = config.Options[0].Value
			}
		case FieldTypeLinearScale:
			if config.Scale != nil {
				output.Response = (config.Scale.Min + config.Scale.Max) / 2
			}
		default:
			output.Response = "test response"
		}
	} else if len(config.Fields) > 0 {
		// Multi-field mock response
		output.Fields = make(map[string]interface{})
		for _, field := range config.Fields {
			switch field.Type {
			case FieldTypeMultipleChoice:
				if len(field.Options) > 0 {
					output.Fields[field.ID] = field.Options[0].Value
				}
			case FieldTypeLinearScale:
				if field.Scale != nil {
					output.Fields[field.ID] = (field.Scale.Min + field.Scale.Max) / 2
				}
			default:
				output.Fields[field.ID] = "test response"
			}
		}
	}

	return output, nil
}
