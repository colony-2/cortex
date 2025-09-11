package shared

import (
    "fmt"

    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// RecipeValidator validates basic recipe structure and inputs
type RecipeValidator struct{}

func NewRecipeValidator() *RecipeValidator { return &RecipeValidator{} }

// ValidateInputs checks required inputs from the recipe metadata's InputSchema
func (rv *RecipeValidator) ValidateInputs(r *recipe.Recipe, inputs map[string]interface{}) error {
    if r == nil || r.RecipeImpl == nil {
        return nil
    }
    meta := r.RecipeImpl.GetMetadata()
    for key, schema := range meta.InputSchema {
        if schema.Required {
            if _, ok := inputs[key]; !ok {
                return fmt.Errorf("required input '%s' not provided", key)
            }
        }
    }
    return nil
}
