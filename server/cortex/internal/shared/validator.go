package shared

import (
	"fmt"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// RecipeValidator validates recipe structure and inputs
type RecipeValidator struct {
	registryManager *RegistryManager
}

// NewRecipeValidator creates a new recipe validator
func NewRecipeValidator(rm *RegistryManager) *RecipeValidator {
	return &RecipeValidator{registryManager: rm}
}

// ValidateRecipeStructure validates the basic structure of a recipe
func (rv *RecipeValidator) ValidateRecipeStructure(recipe *yamlpkg.RecipeDefinition) error {
	if recipe.Name == "" {
		return fmt.Errorf("recipe name is required")
	}
	
	if len(recipe.Steps) == 0 {
		return fmt.Errorf("recipe must have at least one step")
	}
	
	// Validate each step has required fields
	for i, step := range recipe.Steps {
		if step.ID == "" {
			return fmt.Errorf("step %d: id is required", i)
		}
		if step.Uses == "" && step.Parallel == nil {
			return fmt.Errorf("step %s: must specify uses or parallel", step.ID)
		}
		
		// Validate activity exists
		if step.Uses != "" {
			registry := rv.registryManager.GetRegistry()
			if _, exists := registry.Get(step.Uses); !exists {
				return fmt.Errorf("step %s: unknown activity type %s", step.ID, step.Uses)
			}
		}
		
		// Validate parallel steps recursively
		if step.Parallel != nil {
			for j, parallelStep := range step.Parallel.Steps {
				if parallelStep.ID == "" {
					return fmt.Errorf("step %s: parallel step %d: id is required", step.ID, j)
				}
				if parallelStep.Uses == "" {
					return fmt.Errorf("step %s: parallel step %s: uses is required", step.ID, parallelStep.ID)
				}
				
				// Validate parallel activity exists
				registry := rv.registryManager.GetRegistry()
				if _, exists := registry.Get(parallelStep.Uses); !exists {
					return fmt.Errorf("step %s: parallel step %s: unknown activity type %s", 
						step.ID, parallelStep.ID, parallelStep.Uses)
				}
			}
		}
	}
	
	return nil
}

// ValidateInputs validates that required inputs are provided
func (rv *RecipeValidator) ValidateInputs(recipe *yamlpkg.RecipeDefinition, inputs map[string]interface{}) error {
	// Check required inputs
	for _, input := range recipe.Inputs {
		if input.Required {
			if _, exists := inputs[input.Name]; !exists {
				return fmt.Errorf("required input '%s' not provided", input.Name)
			}
		}
	}
	
	// TODO: Add type validation when schema information is available
	
	return nil
}

// ValidateOutputs validates that outputs are properly defined
func (rv *RecipeValidator) ValidateOutputs(recipe *yamlpkg.RecipeDefinition) error {
	// Validate output references
	for _, output := range recipe.Outputs {
		if output.Value == "" {
			return fmt.Errorf("output '%s': value is required", output.Name)
		}
		// TODO: Validate that output references valid step outputs
	}
	
	return nil
}