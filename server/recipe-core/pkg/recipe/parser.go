package recipe

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// Parser parses recipe files from disk
type Parser struct {
	logger *zap.Logger
	activityTypeRegistry *ActivityTypeRegistry
}

// NewParser creates a new recipe parser
func NewParser(logger *zap.Logger) *Parser {
	return &Parser{
		logger: logger,
		activityTypeRegistry: NewActivityTypeRegistry(),
	}
}

// NewParserWithRegistry creates a new recipe parser with a custom activity type registry
func NewParserWithRegistry(logger *zap.Logger, registry *ActivityTypeRegistry) *Parser {
	return &Parser{
		logger: logger,
		activityTypeRegistry: registry,
	}
}

// GetActivityTypeRegistry returns the parser's activity type registry
func (p *Parser) GetActivityTypeRegistry() *ActivityTypeRegistry {
	return p.activityTypeRegistry
}

// ParseRecipe parses a recipe from a path (file or directory)
func (p *Parser) ParseRecipe(path string) (*Recipe, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return p.parseMultiFileRecipe(path)
	}

	return p.parseSingleFileRecipe(path)
}

// parseSingleFileRecipe parses a single-file recipe using unified format
func (p *Parser) parseSingleFileRecipe(filePath string) (*Recipe, error) {
	// Use the YAML parser to parse the unified recipe format
	parser := yamlpkg.NewParser()
	recipeDefinition, err := parser.ParseRecipe(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recipe YAML: %w", err)
	}

	// Create Recipe struct from RecipeDefinition
	recipe := &Recipe{
		Name:         recipeDefinition.Name,
		Version:      recipeDefinition.Version,
		Description:  recipeDefinition.Description,
		BasePath:     filepath.Dir(filePath),
		ManifestPath: filePath,
		Recipe:       recipeDefinition,
	}

	// Validate recipe
	if err := p.validateRecipe(recipe); err != nil {
		return nil, err
	}

	// Set last modified time
	fileInfo, err := os.Stat(filePath)
	if err == nil {
		recipe.LastModified = fileInfo.ModTime()
	}

	// Compute hash
	hc := NewHashComputer()
	recipe.Hash = hc.ComputeRecipeHash(recipe)

	return recipe, nil
}

// parseMultiFileRecipe parses a multi-file recipe from a directory
// NOTE: Multi-file format is deprecated. Use single-file unified format instead.
func (p *Parser) parseMultiFileRecipe(dirPath string) (*Recipe, error) {
	return nil, fmt.Errorf("multi-file recipe format is deprecated. Please migrate to the unified single-file format")
}

// validateRecipe validates a recipe definition
func (p *Parser) validateRecipe(recipe *Recipe) error {
	if recipe == nil {
		return fmt.Errorf("recipe cannot be nil")
	}

	if recipe.Name == "" {
		return fmt.Errorf("recipe name is required")
	}

	if recipe.Version == "" {
		return fmt.Errorf("recipe version is required")
	}

	if recipe.Recipe == nil {
		return fmt.Errorf("recipe definition is required")
	}

	// Validate steps exist and have uses field
	if len(recipe.Recipe.Steps) == 0 {
		return fmt.Errorf("recipe must have at least one step")
	}

	for i, step := range recipe.Recipe.Steps {
		if step.ID == "" {
			return fmt.Errorf("step %d is missing required 'id' field", i)
		}
		// Step must have either 'uses' field or 'parallel' field
		if step.Uses == "" && step.Parallel == nil {
			return fmt.Errorf("step '%s' must have either 'uses' field or 'parallel' field", step.ID)
		}
	}

	return nil
}