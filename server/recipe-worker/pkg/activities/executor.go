package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
	"github.com/vibethis/server/recipe-core/pkg/yaml"
)

// ExecutorImplementation defines the interface for activity execution implementations
type ExecutorImplementation interface {
	ExecuteHTTPActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteGRPCActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteScriptActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteFunctionActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteAIPromptActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
}

// Executor handles execution of activities based on their implementation type
type Executor struct {
	implementation ExecutorImplementation
}

// NewExecutor creates a new activity executor
func NewExecutor(impl ExecutorImplementation) *Executor {
	return &Executor{
		implementation: impl,
	}
}

// ExecuteActivity executes an activity based on its definition
func (e *Executor) ExecuteActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Check if this is an activity context before using activity.GetLogger
	if activity.IsActivity(ctx) {
		logger := activity.GetLogger(ctx)
		logger.Info("Executing activity", "name", activityDef.Name, "type", activityDef.Implementation.Type)
	}

	switch activityDef.Implementation.Type {
	case "http":
		return e.implementation.ExecuteHTTPActivity(ctx, activityDef, inputs)
	case "grpc":
		return e.implementation.ExecuteGRPCActivity(ctx, activityDef, inputs)
	case "script":
		return e.implementation.ExecuteScriptActivity(ctx, activityDef, inputs)
	case "function":
		return e.implementation.ExecuteFunctionActivity(ctx, activityDef, inputs)
	case "ai_prompt":
		return e.implementation.ExecuteAIPromptActivity(ctx, activityDef, inputs)
	default:
		return nil, fmt.Errorf("unsupported activity type: %s", activityDef.Implementation.Type)
	}
}

// RegisterActivities registers all activities with the Temporal worker
func (e *Executor) RegisterActivities(activityDefs []yaml.ActivityDefinition) map[string]interface{} {
	activities := make(map[string]interface{})
	
	for _, def := range activityDefs {
		def := def // capture loop variable
		activities[def.Name] = func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return e.ExecuteActivity(ctx, &def, inputs)
		}
	}
	
	return activities
}