package jobutil

import (
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
)

func BuildRecipeSourceResolver() (compiler.RecipeSourceResolver, func(), error) {
	return compiler.NewRecipeSourceResolver(compiler.RecipeSourceResolverOptions{}), nil, nil
}
