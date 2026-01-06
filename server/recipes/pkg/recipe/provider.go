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

	// Parse the raw YAML content
	parsedRecipe, err := recipecore.LoadRecipeFromString(recipeWithContent.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recipe %q: %w", recipeName, err)
	}

	return parsedRecipe, nil
}

// parseRecipeRef parses "name@ref" into (name, ref).
// Returns (name, "") if no @ref suffix.
func (p *Provider) parseRecipeRef(nameWithRef string) (string, string) {
	if idx := strings.LastIndex(nameWithRef, "@"); idx > 0 {
		return nameWithRef[:idx], nameWithRef[idx+1:]
	}
	return nameWithRef, ""
}

// RecipeProjectProvider is a function that retrieves a recipe by project ID and recipe reference.
// The recipeRef can be a simple name or a name@ref combination.
type RecipeProjectProvider func(projectId string, recipeRef string) (*recipecore.Recipe, error)

// NewRecipeProjectProvider creates a RecipeProjectProvider from a Recipe Service.
// This is the recommended way to create a recipe provider for use in workflow controls
// and other systems that need project-scoped recipe access.
func NewRecipeProjectProvider(service Service) RecipeProjectProvider {
	return func(projectId string, recipeRef string) (*recipecore.Recipe, error) {
		ctx := context.Background()

		// Parse name@ref syntax
		recipeName := recipeRef
		gitRef := ""
		if idx := strings.LastIndex(recipeRef, "@"); idx > 0 {
			recipeName = recipeRef[:idx]
			gitRef = recipeRef[idx+1:]
		}

		// Call unified GetRecipe API
		recipeWithContent, err := service.GetRecipe(ctx, project.ID(projectId), recipeName, gitRef)
		if err != nil {
			if gitRef != "" {
				return nil, fmt.Errorf("recipe %q at ref %s not found: %w", recipeName, gitRef, err)
			}
			return nil, fmt.Errorf("recipe %q not found: %w", recipeName, err)
		}

		// Parse the raw YAML content
		parsedRecipe, err := recipecore.LoadRecipeFromString(recipeWithContent.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse recipe %q: %w", recipeName, err)
		}

		return parsedRecipe, nil
	}
}
