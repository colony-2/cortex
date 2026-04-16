package internalrecipes

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
)

//go:embed new_ticket.yaml
var newTicketRecipeBytes []byte

// EmbeddedProvider serves embedded internal recipes (currently new_ticket).
type EmbeddedProvider struct {
	recipes    map[string]*recipe.Recipe
	recipeYAML map[string][]byte
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
		recipeYAML: map[string][]byte{
			rec.GetMetdata().ID: append([]byte(nil), newTicketRecipeBytes...),
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

func (p *EmbeddedProvider) GetRecipeYAML(name string) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("embedded provider is nil")
	}
	if yamlBytes, ok := p.recipeYAML[name]; ok {
		return append([]byte(nil), yamlBytes...), nil
	}
	return nil, fmt.Errorf("recipe %q not found", name)
}

// NewRecipeProjectProviderWithFallback retrieves recipes by project ID and recipe reference.
// It tries the recipe service first, then falls back to the embedded provider for internal recipes.
func NewRecipeProjectProviderWithFallback(svc recipesvc.Service, embeddedProvider *EmbeddedProvider) recipesvc.RecipeProjectProvider {
	return func(projectID string, recipeRef string) (*recipe.Recipe, error) {
		provider := recipesvc.NewProvider(svc, project.ID(projectID))
		rec, err := provider.GetRecipe(recipeRef)
		if err == nil {
			return rec, nil
		}
		return embeddedProvider.GetRecipe(recipeRef)
	}
}

func NewRecipeSourceResolverWithFallback(svc recipesvc.Service, embeddedProvider *EmbeddedProvider) compiler.RecipeSourceResolver {
	return compiler.NewRecipeSourceResolver(compiler.RecipeSourceResolverOptions{
		RecipeRefResolver: recipeServiceRefResolver{
			service:  svc,
			embedded: embeddedProvider,
		},
	})
}

type recipeServiceRefResolver struct {
	service  recipesvc.Service
	embedded *EmbeddedProvider
}

func (r recipeServiceRefResolver) ResolveRecipeRef(ctx context.Context, projectID string, selector string) (compiler.RecipeSourceResolution, error) {
	name, ref := splitServerRecipeSelector(selector)

	if r.service != nil {
		resolved, err := r.service.GetRecipe(ctx, project.ID(projectID), name, ref)
		if err == nil {
			stableSelector := name
			if strings.TrimSpace(resolved.CommitHash) != "" {
				stableSelector += "@" + strings.TrimSpace(resolved.CommitHash)
			}
			return compiler.RecipeSourceResolution{
				SourceKind:        compiler.RecipeSourceKindServerRef,
				SubmittedSelector: selector,
				ResolvedSelector:  stableSelector,
				ResolvedCommit:    strings.TrimSpace(resolved.CommitHash),
				WasAlreadyPinned:  strings.TrimSpace(ref) != "" && strings.TrimSpace(ref) == strings.TrimSpace(resolved.CommitHash),
			}, nil
		}
		if r.embedded == nil || strings.TrimSpace(ref) != "" {
			return compiler.RecipeSourceResolution{}, err
		}
	}

	if r.embedded == nil {
		return compiler.RecipeSourceResolution{}, fmt.Errorf("recipe source resolver is not configured for %q", selector)
	}
	if _, err := r.embedded.GetRecipe(name); err != nil {
		return compiler.RecipeSourceResolution{}, err
	}
	return compiler.RecipeSourceResolution{
		SourceKind:        compiler.RecipeSourceKindServerRef,
		SubmittedSelector: selector,
		ResolvedSelector:  selector,
		WasAlreadyPinned:  true,
	}, nil
}

func (r recipeServiceRefResolver) LoadRecipeRef(ctx context.Context, projectID string, selector string) (*recipe.Recipe, error) {
	name, ref := splitServerRecipeSelector(selector)

	if r.service != nil {
		resolved, err := r.service.GetRecipe(ctx, project.ID(projectID), name, ref)
		if err == nil {
			return recipe.LoadRecipeFromString([]byte(resolved.Content))
		}
		if r.embedded == nil || strings.TrimSpace(ref) != "" {
			return nil, err
		}
	}

	if r.embedded == nil {
		return nil, fmt.Errorf("recipe source resolver is not configured for %q", selector)
	}
	return r.embedded.GetRecipe(name)
}

func (r recipeServiceRefResolver) LoadRecipeRefYAML(ctx context.Context, projectID string, selector string) ([]byte, error) {
	name, ref := splitServerRecipeSelector(selector)

	if r.service != nil {
		resolved, err := r.service.GetRecipe(ctx, project.ID(projectID), name, ref)
		if err == nil {
			return []byte(resolved.Content), nil
		}
		if r.embedded == nil || strings.TrimSpace(ref) != "" {
			return nil, err
		}
	}

	if r.embedded == nil {
		return nil, fmt.Errorf("recipe source resolver is not configured for %q", selector)
	}
	return r.embedded.GetRecipeYAML(name)
}

func splitServerRecipeSelector(selector string) (string, string) {
	selector = strings.TrimSpace(selector)
	if idx := strings.LastIndex(selector, "@"); idx > 0 {
		return strings.TrimSpace(selector[:idx]), strings.TrimSpace(selector[idx+1:])
	}
	return selector, ""
}
