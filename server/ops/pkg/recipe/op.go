package recipe

import (
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RecipeInput defines the input for recipe activities
type RecipeInput struct {
	Name   string                 `json:"name"`
	Inputs map[string]interface{} `json:"inputs"`
	Raw    map[string]interface{} `json:"-" mapstructure:",remain"`
}

// RecipeOutput defines the output from recipe activities
type RecipeOutput struct {
	Outputs        map[string]interface{} `json:"result"`            // Outputs from the executed recipe
	Context        map[string]interface{} `json:"context,omitempty"` // Propagated git context
	GitPersistHash string                 `json:"git_persist_hash,omitempty"`
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
	return ops.NewInlineOpV2[RecipeInput, RecipeOutput](ops.OpMetadata{
		Type:           "recipe",
		Description:    "Invokes another recipe as a child workflow with automatic context propagation",
		Version:        "1.0.0",
		DefaultTimeout: 35 * time.Minute, // Default timeout, can be overridden
	}, execute)
}

// Execute runs the activity with provided configuration and inputs
func execute(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input RecipeInput) (RecipeOutput, error) {
	baseInputs := make(map[string]interface{})
	for k, v := range input.Raw {
		baseInputs[k] = v
	}

	workspaceResult, err := gitstate.WithInlineWorkspace(ctx, inv, baseInputs, gitstate.InlineWorkspaceOptions{}, func(inner workflow.Context, childInputs map[string]interface{}) (map[string]interface{}, error) {
		for k, v := range input.Inputs {
			childInputs[k] = v
		}
		workflow.GetLogger(inner).Info("invoking child recipe", "inputs_keys", mapKeys(childInputs))
		cwo := workflow.ChildWorkflowOptions{
			RetryPolicy: retry,
		}
		if timeout > 0 {
			cwo.WorkflowRunTimeout = timeout
			cwo.WorkflowExecutionTimeout = timeout
		}
		inner = workflow.WithChildOptions(inner, cwo)
		workflowRun := workflow.ExecuteChildWorkflow(inner, input.Name, childInputs)
		var result map[string]interface{}
		if err := workflowRun.Get(inner, &result); err != nil {
			return nil, err
		}
		if result == nil {
			result = make(map[string]interface{})
		}
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		workflow.GetLogger(inner).Info("child recipe completed", "keys", keys, "result", result)
		return result, nil
	})
	if err != nil {
		return RecipeOutput{}, err
	}

	return RecipeOutput{
		Outputs:        workspaceResult.Result,
		Context:        workspaceResult.ContextMap,
		GitPersistHash: workspaceResult.GitContext.PersistHash,
	}, nil

}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
