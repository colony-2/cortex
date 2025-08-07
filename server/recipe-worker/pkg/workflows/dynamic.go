package workflows

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// WorkflowExecutor interface for executing workflow logic
type WorkflowExecutor interface {
	ExecuteWorkflow(ctx workflow.Context, recipeDef *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
}

// ActivityInvoker interface for invoking activities from workflows
type ActivityInvoker interface {
	InvokeActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error)
}

// CreateDynamicWorkflow creates a Temporal workflow function from a unified recipe definition
func CreateDynamicWorkflow(recipeDef *yamlpkg.RecipeDefinition, executor WorkflowExecutor) interface{} {
	return func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		if executor != nil {
			return executor.ExecuteWorkflow(ctx, recipeDef, inputs)
		}
		
		// Default implementation - placeholder
		// Set activity options
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Minute,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		// Return placeholder result
		return map[string]interface{}{
			"status": "completed",
			"recipe": recipeDef.Name,
		}, nil
	}
}

// CreateDynamicActivity creates a Temporal activity function from a step definition
func CreateDynamicActivity(step *yamlpkg.Step) interface{} {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Get activity info for logging
		info := activity.GetInfo(ctx)
		
		// For now, return a placeholder result
		// Full implementation would execute the activity
		return map[string]interface{}{
			"status": "completed",
			"step": step.ID,
			"uses": step.Uses,
			"activityID": info.ActivityID,
		}, nil
	}
}