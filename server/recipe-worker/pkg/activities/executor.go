package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	worker "github.com/divisive-ai/vibethis/server/recipe-worker"
)

// ExecutorImplementation defines the interface for activity execution implementations
type ExecutorImplementation interface {
	ExecuteHTTPActivity(ctx context.Context, step *yamlpkg.Step, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteGRPCActivity(ctx context.Context, step *yamlpkg.Step, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteScriptActivity(ctx context.Context, step *yamlpkg.Step, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteFunctionActivity(ctx context.Context, step *yamlpkg.Step, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteAIPromptActivity(ctx context.Context, step *yamlpkg.Step, inputs map[string]interface{}) (map[string]interface{}, error)
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

// ExecuteActivity executes an activity based on its step definition
func (e *Executor) ExecuteActivity(ctx context.Context, step *yamlpkg.Step, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Check if this is an activity context before using activity.GetLogger
	if activity.IsActivity(ctx) {
		logger := activity.GetLogger(ctx)
		logger.Info("Executing activity", "step", step.ID, "uses", step.Uses)
	}

	// Get activity type from config or try to infer from uses
	activityType, ok := step.Config["type"].(string)
	if !ok {
		// Default fallback based on uses field or try to infer
		activityType = "function" // Default to function type
	}

	// First check if a provider is registered for this activity type
	if e.providerRegistry.Has(activityType) {
		provider, err := e.providerRegistry.Get(activityType)
		if err != nil {
			return nil, fmt.Errorf("failed to get provider for type %s: %w", activityType, err)
		}
		
		// Execute using the provider
		result, err := provider.Execute(ctx, step.Config, inputs)
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
	switch activityType {
	case "http":
		return e.implementation.ExecuteHTTPActivity(ctx, step, inputs)
	case "grpc":
		return e.implementation.ExecuteGRPCActivity(ctx, step, inputs)
	case "script":
		return e.implementation.ExecuteScriptActivity(ctx, step, inputs)
	case "function":
		return e.implementation.ExecuteFunctionActivity(ctx, step, inputs)
	case "ai_prompt":
		return e.implementation.ExecuteAIPromptActivity(ctx, step, inputs)
	default:
		return nil, fmt.Errorf("unsupported activity type: %s", activityType)
	}
}

// RegisterActivities registers activities for the given steps with the Temporal worker
func (e *Executor) RegisterActivities(steps []yamlpkg.Step) map[string]interface{} {
	activities := make(map[string]interface{})
	
	for _, step := range steps {
		step := step // capture loop variable
		// Register activity by step ID or Uses name
		activityName := step.ID
		if activityName == "" {
			activityName = step.Uses
		}
		activities[activityName] = func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return e.ExecuteActivity(ctx, &step, inputs)
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