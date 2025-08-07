package statemachine

import (
	"context"
	"fmt"

	"github.com/divisive-ai/vibethis/server/activity/pkg/recipe"
)

// ActivityExecutor implements ExecutorImplementation for executing activities
type ActivityExecutor struct {
	activityRegistry map[string]interface{}
	isWorkflow       bool
}

// NewActivityExecutor creates a new activity executor
func NewActivityExecutor() *ActivityExecutor {
	return &ActivityExecutor{
		activityRegistry: make(map[string]interface{}),
		isWorkflow:       false,
	}
}

// NewWorkflowExecutor creates an executor for use within workflows
func NewWorkflowExecutor() *ActivityExecutor {
	return &ActivityExecutor{
		activityRegistry: make(map[string]interface{}),
		isWorkflow:       true,
	}
}

// RegisterActivity registers an activity for execution
func (e *ActivityExecutor) RegisterActivity(name string, activity interface{}) {
	e.activityRegistry[name] = activity
}

// ExecuteActivity executes a registered activity
func (e *ActivityExecutor) ExecuteActivity(ctx context.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	_, exists := e.activityRegistry[activityName]
	if !exists {
		return nil, fmt.Errorf("activity '%s' not found in registry", activityName)
	}

	// Direct activity execution for non-workflow contexts
	// This would need to be implemented based on the actual activity interface
	return nil, fmt.Errorf("direct activity execution not yet implemented for '%s'", activityName)
}

// ExecuteRecipe executes a recipe (workflow)
func (e *ActivityExecutor) ExecuteRecipe(ctx context.Context, recipeName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Build recipe activity input
	recipeActivity := recipe.RecipeActivity{
		Recipe: recipeName,
		Inputs: inputs,
	}

	// Extract context if available
	if ctxVal, ok := inputs["context"]; ok {
		if recipeCtx, ok := ctxVal.(*recipe.RecipeContext); ok {
			recipeActivity.Context = recipeCtx
		}
	}

	// Use ExecuteRecipeActivity for direct execution
	output, err := recipe.ExecuteRecipeActivity(ctx, recipeActivity)
	if err != nil {
		return nil, fmt.Errorf("failed to execute recipe '%s': %w", recipeName, err)
	}
	return output.Result, nil
}

// DefaultExecutor provides a simple implementation for testing
type DefaultExecutor struct {
	activities map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error)
	recipes    map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error)
}

// NewDefaultExecutor creates a new default executor for testing
func NewDefaultExecutor() *DefaultExecutor {
	return &DefaultExecutor{
		activities: make(map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error)),
		recipes:    make(map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error)),
	}
}

// RegisterActivity registers an activity function
func (e *DefaultExecutor) RegisterActivity(name string, fn func(context.Context, map[string]interface{}) (map[string]interface{}, error)) {
	e.activities[name] = fn
}

// RegisterRecipe registers a recipe function
func (e *DefaultExecutor) RegisterRecipe(name string, fn func(context.Context, map[string]interface{}) (map[string]interface{}, error)) {
	e.recipes[name] = fn
}

// ExecuteActivity executes a registered activity
func (e *DefaultExecutor) ExecuteActivity(ctx context.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	activity, exists := e.activities[activityName]
	if !exists {
		return nil, fmt.Errorf("activity '%s' not found", activityName)
	}
	return activity(ctx, inputs)
}

// ExecuteRecipe executes a registered recipe
func (e *DefaultExecutor) ExecuteRecipe(ctx context.Context, recipeName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	recipe, exists := e.recipes[recipeName]
	if !exists {
		return nil, fmt.Errorf("recipe '%s' not found", recipeName)
	}
	return recipe(ctx, inputs)
}