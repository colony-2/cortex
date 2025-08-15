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
	
	// Check that recipe defines at least one node type
	if recipe.Op == "" && len(recipe.Sequence) == 0 && len(recipe.Parallel) == 0 && recipe.States == nil {
		return fmt.Errorf("recipe must define one of: op, sequence, parallel, or states")
	}
	
	// Validate sequence nodes if present
	for i, node := range recipe.Sequence {
		if node.ID == "" && (node.Op != "" || node.Shared != "") {
			return fmt.Errorf("sequence node %d: id is recommended", i)
		}
		
		// Validate node has an operation defined
		if node.Op == "" && node.Shared == "" && len(node.Sequence) == 0 && len(node.Parallel) == 0 && node.States == nil {
			return fmt.Errorf("sequence node %d: must define op, shared, sequence, parallel, or states", i)
		}
	}
	
	// Validate parallel nodes if present  
	for i, node := range recipe.Parallel {
		if node.ID == "" && (node.Op != "" || node.Shared != "") {
			return fmt.Errorf("parallel node %d: id is recommended", i)
		}
		
		// Validate node has an operation defined
		if node.Op == "" && node.Shared == "" && len(node.Sequence) == 0 && len(node.Parallel) == 0 && node.States == nil {
			return fmt.Errorf("parallel node %d: must define op, shared, sequence, parallel, or states", i)
		}
	}
	
	return nil
}

// ValidateInputs validates that required inputs are provided
func (rv *RecipeValidator) ValidateInputs(recipe *yamlpkg.RecipeDefinition, inputs map[string]interface{}) error {
	// Check required inputs using InputSchema
	if recipe.InputSchema != nil {
		for name, inputDef := range recipe.InputSchema {
			if inputDef.Required {
				if _, exists := inputs[name]; !exists {
					return fmt.Errorf("required input '%s' not provided", name)
				}
			}
		}
	}
	
	// TODO: Add type validation when needed
	
	return nil
}

// ValidateOutputs validates that outputs are properly defined
func (rv *RecipeValidator) ValidateOutputs(recipe *yamlpkg.RecipeDefinition) error {
	// With the new format, outputs are defined as map[string]interface{}
	// Each output value should be a valid template expression
	for name, value := range recipe.Outputs {
		if value == nil || value == "" {
			return fmt.Errorf("output '%s': value is required", name)
		}
		// TODO: Validate that output references valid step outputs
	}
	
	return nil
}