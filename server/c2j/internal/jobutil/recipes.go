package jobutil

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/colony-2/colony2/server/api/pkg/serverdeps"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerworkflow "github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/colony2/server/registry/pkg/registry"
)

func BuildRecipeProvider(recipesDir string) (workerworkflow.RecipeProjectProvider, func(), error) {
	embedded, embeddedErr := serverdeps.NewEmbeddedProvider()

	var localRegistry *registry.Registry
	var stop func()
	recipesDir = strings.TrimSpace(recipesDir)
	if recipesDir != "" {
		absDir, err := filepath.Abs(recipesDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve recipes dir: %w", err)
		}
		localRegistry, err = registry.NewRegistry(slog.Default(), absDir)
		if err != nil {
			return nil, nil, fmt.Errorf("create local recipe registry: %w", err)
		}
		if err := localRegistry.Start(); err != nil {
			return nil, nil, fmt.Errorf("start local recipe registry: %w", err)
		}
		stop = func() {
			_ = localRegistry.Stop()
		}
	}

	provider := func(projectID string, recipeRef string) (*recipe.Recipe, error) {
		name := recipeRef
		if idx := strings.LastIndex(recipeRef, "@"); idx > 0 {
			name = recipeRef[:idx]
		}
		name = strings.TrimSpace(name)

		if localRegistry != nil {
			if rec, err := localRegistry.GetRecipe(name); err == nil {
				return rec, nil
			}
		}
		if embedded != nil {
			if rec, err := embedded.GetRecipe(name); err == nil {
				return rec, nil
			}
		}
		if embeddedErr != nil {
			return nil, fmt.Errorf("recipe %q not found locally; embedded recipes unavailable for c2j runtime: %w", recipeRef, embeddedErr)
		}
		return nil, fmt.Errorf("recipe %q not found locally or in embedded recipes; set --recipes-dir to a repository containing the referenced recipes", recipeRef)
	}
	return provider, stop, nil
}

func BuildRecipeSourceResolver(recipesDir string) (compiler.RecipeSourceResolver, func(), error) {
	provider, stop, err := BuildRecipeProvider(recipesDir)
	if err != nil {
		return nil, nil, err
	}

	resolver := compiler.NewRecipeSourceResolver(compiler.RecipeSourceResolverOptions{
		RecipeRefResolver: compiler.NewProviderBackedRecipeRefResolver(func(projectID string, recipeRef string) (*recipe.Recipe, error) {
			return provider(projectID, recipeRef)
		}),
	})
	return resolver, stop, nil
}
