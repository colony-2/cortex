package recipe

import (
	"context"
	"fmt"
	"strings"

	"github.com/colony-2/colony2/server/project/pkg/project"
	recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
)

// Provider implements the RecipeProvider interface from recipe-core.
// It supports the name@ref syntax for version-specific recipe retrieval.
type Provider struct {
	service   Service
	projectID project.ID
}

// NewProvider creates a new recipe provider for a project.
func NewProvider(service Service, projectID project.ID) *Provider {
	return &Provider{
		service:   service,
		projectID: projectID,
	}
}

// GetRecipe retrieves a recipe by name, supporting the name@ref syntax.
// - Simple name (e.g., "workflows/ci/build"): Returns the published version
// - Name with ref (e.g., "workflows/ci/build@a1b2c3d"): Returns recipe at that specific git ref
func (p *Provider) GetRecipe(name string) (*recipecore.Recipe, error) {
	ctx := context.Background()

	// Parse name@ref syntax
	recipeName, gitRef := p.parseRecipeRef(name)

	// Call unified GetRecipe API
	recipeWithContent, err := p.service.GetRecipe(ctx, p.projectID, recipeName, gitRef)
	if err != nil {
		if gitRef != "" {
			return nil, fmt.Errorf("recipe %q at ref %s not found: %w", recipeName, gitRef, err)
		}
		return nil, fmt.Errorf("recipe %q not found: %w", recipeName, err)
	}

	return recipeWithContent.Content, nil
}

// parseRecipeRef parses "name@ref" into (name, ref).
// Returns (name, "") if no @ref suffix.
func (p *Provider) parseRecipeRef(nameWithRef string) (string, string) {
	if idx := strings.LastIndex(nameWithRef, "@"); idx > 0 {
		return nameWithRef[:idx], nameWithRef[idx+1:]
	}
	return nameWithRef, ""
}
