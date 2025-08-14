package yaml

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Parser handles parsing of YAML recipe definitions
type Parser struct{}

// NewParser creates a new YAML parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseRecipe parses a unified recipe definition from a file
func (p *Parser) ParseRecipe(path string) (*RecipeDefinition, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return p.ParseRecipeReader(file)
}

// ParseRecipeReader parses a unified recipe definition from an io.Reader
func (p *Parser) ParseRecipeReader(r io.Reader) (*RecipeDefinition, error) {
	var recipe RecipeDefinition
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&recipe); err != nil {
		return nil, fmt.Errorf("failed to decode recipe: %w", err)
	}
	
	// Validate that recipe has exactly one root node type
	if err := p.validateRootNode(&recipe); err != nil {
		return nil, err
	}
	
	return &recipe, nil
}

// validateRootNode ensures the recipe has exactly one root node type
func (p *Parser) validateRootNode(recipe *RecipeDefinition) error {
	count := 0
	if recipe.Op != "" {
		count++
	}
	if len(recipe.Sequence) > 0 {
		count++
	}
	if len(recipe.Parallel) > 0 {
		count++
	}
	if recipe.States != nil {
		count++
	}
	
	if count == 0 {
		return fmt.Errorf("recipe must have one of: op, sequence, parallel, or states")
	}
	if count > 1 {
		return fmt.Errorf("recipe must have exactly one of: op, sequence, parallel, or states")
	}
	
	// Validate that operations don't have outputs
	if recipe.Op != "" && recipe.Outputs != nil && len(recipe.Outputs) > 0 {
		return fmt.Errorf("operation nodes cannot have outputs")
	}
	
	return nil
}

