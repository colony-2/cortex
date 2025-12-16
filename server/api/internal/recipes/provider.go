package recipes

import (
	_ "embed"
	"fmt"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
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
	for _, rec := range p.recipes {
		// Return the sole embedded recipe as a fallback.
		return rec, nil
	}
	return nil, fmt.Errorf("recipe %q not found", name)
}
