package recipes

import (
	"fmt"
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/runtime/pkg/internalrecipes"
	"github.com/stretchr/testify/require"
)

// mockProvider is a test implementation of RecipeProvider
type mockProvider struct {
	recipes map[string]*recipe.Recipe
	name    string
}

func (m *mockProvider) GetRecipe(recipeName string) (*recipe.Recipe, error) {
	if rec, ok := m.recipes[recipeName]; ok {
		return rec, nil
	}
	return nil, fmt.Errorf("recipe %q not found in %s", recipeName, m.name)
}

func TestChainedProvider_FirstProviderWins(t *testing.T) {
	// Create a dummy recipe
	rec1 := &recipe.Recipe{}

	// First provider has the recipe
	provider1 := &mockProvider{
		recipes: map[string]*recipe.Recipe{"test-recipe": rec1},
		name:    "provider1",
	}

	// Second provider also has a recipe (but should not be called)
	provider2 := &mockProvider{
		recipes: map[string]*recipe.Recipe{"test-recipe": &recipe.Recipe{}},
		name:    "provider2",
	}

	chain := NewChainedProvider(provider1, provider2)
	result, err := chain.GetRecipe("test-recipe")

	require.NoError(t, err)
	require.Equal(t, rec1, result, "should return recipe from first provider")
}

func TestChainedProvider_FallbackToSecond(t *testing.T) {
	// Create a dummy recipe
	rec2 := &recipe.Recipe{}

	// First provider does not have the recipe
	provider1 := &mockProvider{
		recipes: map[string]*recipe.Recipe{},
		name:    "provider1",
	}

	// Second provider has the recipe
	provider2 := &mockProvider{
		recipes: map[string]*recipe.Recipe{"test-recipe": rec2},
		name:    "provider2",
	}

	chain := NewChainedProvider(provider1, provider2)
	result, err := chain.GetRecipe("test-recipe")

	require.NoError(t, err)
	require.Equal(t, rec2, result, "should return recipe from second provider")
}

func TestChainedProvider_NotFoundInAny(t *testing.T) {
	// Neither provider has the recipe
	provider1 := &mockProvider{
		recipes: map[string]*recipe.Recipe{},
		name:    "provider1",
	}

	provider2 := &mockProvider{
		recipes: map[string]*recipe.Recipe{},
		name:    "provider2",
	}

	chain := NewChainedProvider(provider1, provider2)
	result, err := chain.GetRecipe("missing-recipe")

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "not found in any provider")
}

func TestChainedProvider_NilProvider(t *testing.T) {
	var nilChain *ChainedProvider
	result, err := nilChain.GetRecipe("test-recipe")

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "chained provider is nil")
}

func TestChainedProvider_NoProviders(t *testing.T) {
	chain := NewChainedProvider()
	result, err := chain.GetRecipe("test-recipe")

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "no providers configured")
}

func TestChainedProvider_SkipNilProviders(t *testing.T) {
	// Create a dummy recipe
	rec2 := &recipe.Recipe{}

	// Second provider has the recipe (first is nil)
	provider2 := &mockProvider{
		recipes: map[string]*recipe.Recipe{"test-recipe": rec2},
		name:    "provider2",
	}

	chain := NewChainedProvider(nil, provider2)
	result, err := chain.GetRecipe("test-recipe")

	require.NoError(t, err)
	require.Equal(t, rec2, result, "should skip nil provider and use second")
}

func TestChainedProvider_WithEmbeddedProvider(t *testing.T) {
	// Test with the actual embedded provider
	// Note: This may fail if ops are not registered, which is expected in unit tests
	embedded, err := internalrecipes.NewEmbeddedProvider()
	if err != nil {
		t.Skipf("Skipping test due to ops not being registered: %v", err)
		return
	}

	// Mock registry provider that doesn't have internal://new_ticket
	registry := &mockProvider{
		recipes: map[string]*recipe.Recipe{},
		name:    "registry",
	}

	chain := NewChainedProvider(registry, embedded)
	result, err := chain.GetRecipe("internal://new_ticket")

	require.NoError(t, err)
	require.NotNil(t, result, "should find internal://new_ticket in embedded provider")
}
