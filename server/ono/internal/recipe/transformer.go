package recipe

import (
	"fmt"
	"strings"

	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	recipecore "github.com/vibethis/server/recipe-core"
	recipehistory "github.com/vibethis/server/recipe-history"
)

// Transformer is a wrapper around the recipe-history transformer
// to maintain backward compatibility
type Transformer struct {
	historyTransformer *recipehistory.Transformer
	registry           *Registry
}

// NewTransformer creates a new transformer instance
func NewTransformer(registry *Registry) *Transformer {
	// Create a GetRecipeFunc that uses the registry
	getRecipe := func(name string) (*recipecore.Recipe, error) {
		return registry.GetRecipe(name)
	}

	return &Transformer{
		historyTransformer: recipehistory.NewTransformer(getRecipe),
		registry:           registry,
	}
}

// WorkflowExecutionToJob delegates to recipe-history transformer
func (t *Transformer) WorkflowExecutionToJob(execution *workflow.WorkflowExecutionInfo, recipeName string) (*Job, error) {
	return t.historyTransformer.WorkflowExecutionToJob(execution, recipeName)
}

// WorkflowExecutionsToJobs delegates to recipe-history transformer
func (t *Transformer) WorkflowExecutionsToJobs(executions []*workflow.WorkflowExecutionInfo, recipeName string) ([]*Job, error) {
	return t.historyTransformer.WorkflowExecutionsToJobs(executions, recipeName)
}

// DescribeWorkflowToJob delegates to recipe-history transformer
func (t *Transformer) DescribeWorkflowToJob(desc *workflowservice.DescribeWorkflowExecutionResponse, recipeName string) (*Job, error) {
	return t.historyTransformer.DescribeWorkflowToJob(desc, recipeName)
}

// HistoryToActivityExecutions delegates to recipe-history transformer
func (t *Transformer) HistoryToActivityExecutions(history *history.History, recipe *Recipe) ([]*ActivityExecution, error) {
	recipeName := ""
	if recipe != nil {
		recipeName = recipe.Name
	}
	return t.historyTransformer.HistoryToActivityExecutions(history, recipeName)
}

// GetRecipeForWorkflow determines which recipe a workflow belongs to
func (t *Transformer) GetRecipeForWorkflow(workflowID string, taskQueue string) (string, error) {
	// Task queue format: ono-recipes-{recipe-name}
	if strings.HasPrefix(taskQueue, "ono-recipes-") {
		return strings.TrimPrefix(taskQueue, "ono-recipes-"), nil
	}

	// Try to extract from workflow ID (format: {recipe-name}-{timestamp})
	parts := strings.Split(workflowID, "-")
	if len(parts) >= 2 {
		// Get all recipes and check if any match the prefix
		recipes, err := t.registry.ListRecipes(nil)
		if err != nil {
			return "", err
		}

		for _, recipe := range recipes {
			if strings.HasPrefix(workflowID, recipe.Name+"-") {
				return recipe.Name, nil
			}
		}
	}

	return "", fmt.Errorf("could not determine recipe for workflow %s", workflowID)
}