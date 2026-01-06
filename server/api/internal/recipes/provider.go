package recipes

import (
	_ "embed"
	"fmt"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
)

//go:embed new_ticket.yaml
var newTicketRecipeBytes []byte

// EmbeddedProvider serves embedded internal recipes (currently new_ticket).
type EmbeddedProvider struct {
	recipes map[string]*recipe.Recipe
}

func NewEmbeddedProvider() (*EmbeddedProvider, error) {
	rec, err := recipe.LoadRecipeFromString(newTicketRecipeBytes)
	if err != nil {
		return nil, fmt.Errorf("load embedded new_ticket recipe: %w", err)
	}
	return &EmbeddedProvider{
		recipes: map[string]*recipe.Recipe{
			rec.GetMetdata().ID: rec,
		},
	}, nil
}

func (p *EmbeddedProvider) GetRecipe(name string) (*recipe.Recipe, error) {
	if p == nil {
		return nil, fmt.Errorf("embedded provider is nil")
	}
	if rec, ok := p.recipes[name]; ok {
		return rec, nil
	}
	return nil, fmt.Errorf("recipe %q not found", name)
}

// NewRecipeProjectProviderWithFallback creates a function that retrieves recipes by project ID and recipe reference.
// It tries the recipe service first, then falls back to the embedded provider for internal recipes like "internal://new_ticket".
// The returned function can be used wherever a RecipeProjectProvider function type is needed.
func NewRecipeProjectProviderWithFallback(svc recipesvc.Service, embeddedProvider *EmbeddedProvider) func(projectId string, recipeRef string) (*recipe.Recipe, error) {
	return func(projectId string, recipeRef string) (*recipe.Recipe, error) {
		// Try project-specific recipe service first
		provider := NewServiceProvider(svc, project.ID(projectId))
		rec, err := provider.GetRecipe(recipeRef)
		if err == nil {
			return rec, nil
		}

		// Fall back to embedded provider for internal recipes
		return embeddedProvider.GetRecipe(recipeRef)
	}
}
