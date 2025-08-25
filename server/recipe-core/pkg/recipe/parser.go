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
	logger               *zap.Logger
	activityTypeRegistry *ActivityTypeRegistry
}

// NewParser creates a new recipe parser
func NewParser(logger *zap.Logger) *Parser {
	return &Parser{
		logger:               logger,
		activityTypeRegistry: NewActivityTypeRegistry(),
	}
}

// NewParserWithRegistry creates a new recipe parser with a custom activity type registry
func NewParserWithRegistry(logger *zap.Logger, registry *ActivityTypeRegistry) *Parser {
	return &Parser{
		logger:               logger,
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
		return nil, fmt.Errorf("directory not supported")
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

	// Resolve shared node references using visitor pattern
	resolvedDefinition, err := yamlpkg.ResolveSharedReferences(recipeDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve shared references: %w", err)
	}

	// Create Recipe struct from RecipeDefinition
	recipe := &Recipe{
		ID:           resolvedDefinition.ID,
		Version:      resolvedDefinition.Version,
		Description:  resolvedDefinition.Desc,
		BasePath:     filepath.Dir(filePath),
		ManifestPath: filePath,
		Recipe:       resolvedDefinition,
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

// validateRecipe validates a recipe definition
func (p *Parser) validateRecipe(recipe *Recipe) error {
	if recipe == nil {
		return fmt.Errorf("recipe cannot be nil")
	}

	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}

	if recipe.Version == "" {
		return fmt.Errorf("recipe version is required")
	}

	if recipe.Recipe == nil {
		return fmt.Errorf("recipe definition is required")
	}

	// Validate that recipe has a root node
	count := 0
	if recipe.Recipe.Op != "" {
		count++
	}
	if len(recipe.Recipe.Sequence) > 0 {
		count++
	}
	if len(recipe.Recipe.Parallel) > 0 {
		count++
	}
	if recipe.Recipe.States != nil {
		count++
	}

	if count == 0 {
		return fmt.Errorf("recipe must have one of: op, sequence, parallel, or states")
	}
	if count > 1 {
		return fmt.Errorf("recipe must have exactly one of: op, sequence, parallel, or states")
	}

	return nil
}
