package recipe

import (
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipes/internal/service"
)

// CELValidator validates CEL expressions inside recipes.
type CELValidator = service.CELValidator

// RecipeWorkerCELValidator validates CEL expressions using recipe-worker.
type RecipeWorkerCELValidator = service.RecipeWorkerCELValidator

// NewRecipeWorkerCELValidator creates a validator backed by recipe-worker validation mode.
func NewRecipeWorkerCELValidator(deps coreops.ServiceDependencies2) *RecipeWorkerCELValidator {
	return service.NewRecipeWorkerCELValidator(deps)
}

// NewRecipeWorkerCELValidatorWithProvider allows injecting custom CEL functions/types.
func NewRecipeWorkerCELValidatorWithProvider(deps coreops.ServiceDependencies2, provider service.CELOptionsProvider) *RecipeWorkerCELValidator {
	return service.NewRecipeWorkerCELValidatorWithProvider(deps, provider)
}
