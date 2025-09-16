package recipe

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"time"
)

// RecipeInput defines the input for recipe activities
type RecipeInput struct {
	Name   string                 `json:"name"`
	Inputs map[string]interface{} `json:"inputs"`
}

// RecipeOutput defines the output from recipe activities
type RecipeOutput struct {
	Outputs map[string]interface{} `json:"result"` // Outputs from the executed recipe
}

// RecipeMetadata contains execution metadata
type RecipeMetadata struct {
	StartTime    string `json:"start_time"`    // RFC3339 formatted time
	EndTime      string `json:"end_time"`      // RFC3339 formatted time
	DurationMs   int64  `json:"duration_ms"`   // Duration in milliseconds
	AttemptCount int    `json:"attempt_count"` // Number of attempts
}

// RecipeActivityWrapper implements the RegisterableOp interface
type recipeInlineAdapter struct {
    isWorkflowContext bool // Indicates if running in workflow context
}

// NewRecipeActivity creates a new recipe activity that implements RegisterableOp
func GetOp() ops.RegisterableOp {
	return ops.NewInlineOp(ops.OpMetadata{
		Type:           "recipe",
		Description:    "Invokes another recipe as a child workflow with automatic context propagation",
		Version:        "1.0.0",
		DefaultTimeout: 35 * time.Minute, // Default timeout, can be overridden
	}, execute)
}

// Execute runs the activity with provided configuration and inputs
func execute(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input RecipeInput) (RecipeOutput, error) {
	//startTime := time.Now()

	cwo := workflow.ChildWorkflowOptions{
		RetryPolicy:              retry,
		WorkflowExecutionTimeout: timeout,
	}

	ctx = workflow.WithChildOptions(ctx, cwo)
	workflowRun := workflow.ExecuteChildWorkflow(
		ctx,
		input.Name,
		input.Inputs,
	)
	var result map[string]interface{}
	err := workflowRun.Get(ctx, &result)

	if err != nil {
		return RecipeOutput{}, err
	}

	//endTime := time.Now()
	//duration := endTime.Sub(startTime)

	// Build output
	output := &RecipeOutput{
		Outputs: result,
	}

	return *output, nil

}
