package recipes

import (
	"context"
	"fmt"
	"strings"

	recipecore "github.com/colony-2/c2j/pkg/recipe"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
)

// ServiceProvider wraps the recipe service to implement RecipeProvider interface
type ServiceProvider struct {
	svc       recipesvc.Service
	projectID project.ID
}

// NewServiceProvider creates a provider for a specific project
func NewServiceProvider(svc recipesvc.Service, projectID project.ID) *ServiceProvider {
	return &ServiceProvider{
		svc:       svc,
		projectID: projectID,
	}
}

// GetRecipe implements recipe.RecipeProvider
// Supports both simple names ("workflow/build") and versioned refs ("workflow/build@v1.0.0")
func (p *ServiceProvider) GetRecipe(name string) (*recipecore.Recipe, error) {
	ctx := context.Background()

	// Parse name@ref syntax
	recipeName := name
	ref := "" // empty = published version

	if idx := strings.Index(name, "@"); idx != -1 {
		recipeName = name[:idx]
		ref = name[idx+1:]
	}

	// Get recipe from service
	recipeWithContent, err := p.svc.GetRecipe(ctx, p.projectID, recipeName, ref)
	if err != nil {
		return nil, fmt.Errorf("recipe service get %s: %w", name, err)
	}

	// Parse the raw YAML content
	parsedRecipe, err := recipecore.LoadRecipeFromString(recipeWithContent.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recipe %s: %w", name, err)
	}

	return parsedRecipe, nil
}
