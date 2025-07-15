package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
	"vibethis/ono/pkg/activities/llm"
	"github.com/vibethis/server/recipe-core/pkg/yaml"
)

// Executor handles execution of activities based on their implementation type
type Executor struct {
	mockHandler *MockActivityHandler
	llmExecutor *llm.Executor
	useMock     bool
}

// NewExecutor creates a new activity executor
func NewExecutor() *Executor {
	return &Executor{
		mockHandler: NewMockActivityHandler(),
		llmExecutor: llm.NewExecutor(),
		useMock:     true, // Default to mock for testing
	}
}

// SetUseMock enables or disables mock mode
func (e *Executor) SetUseMock(useMock bool) {
	e.useMock = useMock
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
		return e.executeHTTPActivity(ctx, activityDef, inputs)
	case "grpc":
		return e.executeGRPCActivity(ctx, activityDef, inputs)
	case "script":
		return e.executeScriptActivity(ctx, activityDef, inputs)
	case "function":
		return e.executeFunctionActivity(ctx, activityDef, inputs)
	case "ai_prompt":
		return e.executeAIPromptActivity(ctx, activityDef, inputs)
	default:
		return nil, fmt.Errorf("unsupported activity type: %s", activityDef.Implementation.Type)
	}
}

func (e *Executor) executeHTTPActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// For now, use mock implementation
	// In a real implementation, this would make HTTP requests based on the config
	if activity.IsActivity(ctx) {
		activity.GetLogger(ctx).Info("Executing HTTP activity (mock)", "config", activityDef.Implementation.Config)
	}
	
	// Route to appropriate mock handler based on activity name
	switch activityDef.Name {
	case "research_activity":
		return e.mockHandler.ResearchActivity(ctx, inputs)
	case "analyze_activity":
		return e.mockHandler.AnalyzeActivity(ctx, inputs)
	case "write_report_activity":
		return e.mockHandler.WriteReportActivity(ctx, inputs)
	default:
		return e.mockHandler.GenericActivity(ctx, activityDef.Name, inputs)
	}
}

func (e *Executor) executeGRPCActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Mock implementation
	return e.mockHandler.GenericActivity(ctx, activityDef.Name, inputs)
}

func (e *Executor) executeScriptActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Mock implementation
	return e.mockHandler.GenericActivity(ctx, activityDef.Name, inputs)
}

func (e *Executor) executeFunctionActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// For now, route to mock handlers based on activity name
	switch activityDef.Name {
	case "research_activity":
		return e.mockHandler.ResearchActivity(ctx, inputs)
	case "analyze_activity":
		return e.mockHandler.AnalyzeActivity(ctx, inputs)
	case "write_report_activity":
		return e.mockHandler.WriteReportActivity(ctx, inputs)
	default:
		return e.mockHandler.GenericActivity(ctx, activityDef.Name, inputs)
	}
}

func (e *Executor) executeAIPromptActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Check if we should use real LLM or mock
	if !e.useMock {
		// Use real LLM executor
		return e.llmExecutor.ExecuteAIPromptActivity(ctx, activityDef, inputs)
	}
	
	// Use mock implementation
	if activity.IsActivity(ctx) {
		activity.GetLogger(ctx).Info("Executing AI prompt activity (mock)", 
			"model", activityDef.Implementation.Config["model"],
			"prompt", activityDef.Implementation.Config["prompt"])
	}
	
	// Route to write report handler for ai_prompt activities
	if activityDef.Name == "write_report_activity" {
		return e.mockHandler.WriteReportActivity(ctx, inputs)
	}
	
	return e.mockHandler.GenericActivity(ctx, activityDef.Name, inputs)
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