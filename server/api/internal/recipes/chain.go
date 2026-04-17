package recipes

import (
	"fmt"

	"github.com/colony-2/c2j/pkg/recipe"
)

// ChainedProvider tries multiple recipe providers in order, returning the first successful result.
type ChainedProvider struct {
	providers []recipe.RecipeProvider
}

// NewChainedProvider creates a new chained provider that tries providers in order.
func NewChainedProvider(providers ...recipe.RecipeProvider) *ChainedProvider {
	return &ChainedProvider{providers: providers}
}

// GetRecipe attempts to get a recipe from each provider in order until one succeeds.
func (c *ChainedProvider) GetRecipe(name string) (*recipe.Recipe, error) {
	if c == nil {
		return nil, fmt.Errorf("chained provider is nil")
	}
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	var lastErr error
	for i, provider := range c.providers {
		if provider == nil {
			continue
		}
		rec, err := provider.GetRecipe(name)
		if err == nil && rec != nil {
			return rec, nil
		}
		lastErr = err
		// Not the last provider, will try next
		if i < len(c.providers)-1 {
			continue
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("recipe %q not found in any provider: %w", name, lastErr)
	}
	return nil, fmt.Errorf("recipe %q not found in any provider", name)
}
