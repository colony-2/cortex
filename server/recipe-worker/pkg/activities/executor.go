package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	worker "github.com/divisive-ai/vibethis/server/recipe-worker"
)

// ExecutorImplementation defines the interface for activity execution implementations
type ExecutorImplementation interface {
	ExecuteHTTPActivity(ctx context.Context, activityDef *recipe.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteGRPCActivity(ctx context.Context, activityDef *recipe.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteScriptActivity(ctx context.Context, activityDef *recipe.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteFunctionActivity(ctx context.Context, activityDef *recipe.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteAIPromptActivity(ctx context.Context, activityDef *recipe.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
}

// Executor handles execution of activities based on their implementation type
type Executor struct {
	implementation ExecutorImplementation
	providerRegistry *worker.ProviderRegistry
}

// NewExecutor creates a new activity executor
func NewExecutor(impl ExecutorImplementation) *Executor {
	return &Executor{
		implementation: impl,
		providerRegistry: worker.NewProviderRegistry(),
	}
}

// NewExecutorWithRegistry creates a new activity executor with a custom provider registry
func NewExecutorWithRegistry(impl ExecutorImplementation, registry *worker.ProviderRegistry) *Executor {
	return &Executor{
		implementation: impl,
		providerRegistry: registry,
	}
}

// ExecuteActivity executes an activity based on its definition
func (e *Executor) ExecuteActivity(ctx context.Context, activityDef *recipe.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Check if this is an activity context before using activity.GetLogger
	if activity.IsActivity(ctx) {
		logger := activity.GetLogger(ctx)
		logger.Info("Executing activity", "name", activityDef.Name, "type", activityDef.Implementation.Type)
	}

	// First check if a provider is registered for this activity type
	if e.providerRegistry.Has(activityDef.Implementation.Type) {
		provider, err := e.providerRegistry.Get(activityDef.Implementation.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to get provider for type %s: %w", activityDef.Implementation.Type, err)
		}
		
		// Execute using the provider
		result, err := provider.Execute(ctx, activityDef.Implementation.Config, inputs)
		if err != nil {
			return nil, err
		}
		
		// Convert result to map if needed
		if resultMap, ok := result.(map[string]interface{}); ok {
			return resultMap, nil
		}
		return map[string]interface{}{"result": result}, nil
	}
	
	// Fall back to built-in implementations
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
func (e *Executor) RegisterActivities(activityDefs []recipe.ActivityDefinition) map[string]interface{} {
	activities := make(map[string]interface{})
	
	for _, def := range activityDefs {
		def := def // capture loop variable
		activities[def.Name] = func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return e.ExecuteActivity(ctx, &def, inputs)
		}
	}
	
	return activities
}

// RegisterProvider registers a custom activity provider
func (e *Executor) RegisterProvider(provider worker.ActivityProvider) error {
	return e.providerRegistry.Register(provider)
}

// GetProviderRegistry returns the provider registry
func (e *Executor) GetProviderRegistry() *worker.ProviderRegistry {
	return e.providerRegistry
}