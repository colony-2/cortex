package workflows

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// WorkflowExecutor interface for executing workflow logic
type WorkflowExecutor interface {
	ExecuteWorkflow(ctx workflow.Context, workflowDef *yamlpkg.WorkflowDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
}

// ActivityInvoker interface for invoking activities from workflows
type ActivityInvoker interface {
	InvokeActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error)
}

// CreateDynamicWorkflow creates a Temporal workflow function from a YAML workflow definition
func CreateDynamicWorkflow(workflowDef *yamlpkg.WorkflowDefinition, project *yamlpkg.Project, executor WorkflowExecutor) interface{} {
	return func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		if executor != nil {
			return executor.ExecuteWorkflow(ctx, workflowDef, inputs)
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
			"workflow": workflowDef.Name,
		}, nil
	}
}

// CreateDynamicActivity creates a Temporal activity function from a YAML activity definition
func CreateDynamicActivity(activityDef *recipe.ActivityDefinition) interface{} {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Get activity info for logging
		info := activity.GetInfo(ctx)
		
		// For now, return a placeholder result
		// Full implementation would execute the activity
		return map[string]interface{}{
			"status": "completed",
			"activity": activityDef.Name,
			"activityID": info.ActivityID,
		}, nil
	}
}