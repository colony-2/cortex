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
	return &recipe, nil
}

