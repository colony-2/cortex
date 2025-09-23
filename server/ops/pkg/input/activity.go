package input

import (
	"context"
	"fmt"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
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
	Config     Config                 `json:"config,omitempty" jsonschema:"description=Form configuration"`
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
		managementService: newInputManagementService(),
	}
}

func GetOp() ops.RegisterableOp {
	a := newInputActivity()
	// Run inline within the workflow: wait on user-response signal
	// NewInlineOpWithManagementV2 builds on ops.NewInlineOpV2[Input, Output] to attach management endpoints.
	return ops.NewInlineOpWithManagementV2[Input, Output](
		a.GetMetadata(),
		func(_ ops.Invocation, ctx workflow.Context, timeout time.Duration, _ *temporal.RetryPolicy, in Input) (Output, error) {
			// Build form from config for validation and metadata
			form := a.buildForm(in.Config, in)

			// Record pending status and basic metadata
			_ = workflow.UpsertTypedSearchAttributes(ctx,
				temporal.NewSearchAttributeKeyKeyword("InputStatus").ValueSet("pending"),
				temporal.NewSearchAttributeKeyString("InputFormTitle").ValueSet(form.Title),
				temporal.NewSearchAttributeKeyString("InputBoxID").ValueSet(in.BoxID),
				temporal.NewSearchAttributeKeyTime("InputCreatedAt").ValueSet(workflow.Now(ctx)),
				temporal.NewSearchAttributeKeyTime("InputExpiresAt").ValueSet(workflow.Now(ctx).Add(form.Timeout)),
			)

			// Wait for signal or timeout
			responseChan := workflow.GetSignalChannel(ctx, "user-response")
			tctx, cancel := workflow.WithCancel(ctx)
			workflow.Go(tctx, func(c workflow.Context) {
				// Use provided timeout if non-zero; otherwise default
				to := form.Timeout
				if to == 0 {
					to = 5 * time.Minute
				}
				workflow.Sleep(c, to)
				cancel()
			})

			var sig UserResponseSignal
			responseChan.Receive(tctx, &sig)
			if tctx.Err() != nil {
				_ = workflow.UpsertTypedSearchAttributes(ctx,
					temporal.NewSearchAttributeKeyKeyword("InputStatus").ValueSet("timeout"),
				)
				return Output{}, temporal.NewApplicationError("input timeout", "TIMEOUT")
			}

			_ = workflow.UpsertTypedSearchAttributes(ctx,
				temporal.NewSearchAttributeKeyKeyword("InputStatus").ValueSet("completed"),
				temporal.NewSearchAttributeKeyString("InputRespondedBy").ValueSet(sig.UserID),
				temporal.NewSearchAttributeKeyTime("InputRespondedAt").ValueSet(sig.RespondedAt),
			)

			return Output{Fields: sig.Fields, UserID: sig.UserID, Metadata: sig.Metadata}, nil
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
		DefaultTimeout: 5 * time.Minute,
	}
}

// Execute runs the input activity using configuration provided within input
func (a *InputActivity) Execute(ctx context.Context, input Input) (Output, error) {
	// Build the form from the embedded config (validation only)
	_ = a.buildForm(input.Config, input)

	// Activities should not collect input directly. This operation must run inline
	// within a workflow that waits on the "user-response" signal, and the signal
	// should be sent via the management service using a typed WorkflowControl.
	return Output{}, fmt.Errorf("input activity execution is not supported outside workflows; run inline and signal via management service")
}

// (No Temporal context or workflow execution in activity code)

// GetManagementService returns the management service for HTTP endpoints
// This implements the ManagementServiceProvider interface
func (a *InputActivity) GetManagementService() ops.ManagementService {
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

// executeWithWorkflow was intentionally removed: activities must not start workflows.
