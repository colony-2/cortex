package input

import (
	"context"
	"fmt"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
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
	Form Config `json:"form,omitempty" jsonschema:"description=Form formuration"`
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
	// Management service for HTTP endpoints
	managementService ops.ManagementService
}

// newInputActivity creates a new input activity instance
func newInputActivity() *InputActivity {
	return &InputActivity{
		managementService: nil, //newInputManagementService(),
	}
}

func GetOp() ops.RegisterableOp {
	a := newInputActivity()
	// Run inline within the workflow: wait on user-response signal
	// NewInlineOpWithManagementV2 builds on ops.NewInlineOpV2[Input, Output] to attach management endpoints.
	return ops.NewActivityMappedOpWithManagementV2[Input, Output](
		a.GetMetadata(),
		func(deps ops.OpDependencies, ctx context.Context, in Input) (Output, error) {
			return Output{}, fmt.Errorf("input activity execution is not supported in workflow, must be done via unheld op")
		},
		a.managementService,
	)
}

// GetMetadata returns activity metadata for registration
func (a *InputActivity) GetMetadata() ops.OpMetadata {
	return ops.OpMetadata{
		Type:           "input",
		Description:    "Collects user input through interactive forms with support for various field types",
		Version:        "1.0.0",
		DisallowAsTask: true,
	}
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

// executeWithWorkflow was intentionally removed: activities must not start workflows.
