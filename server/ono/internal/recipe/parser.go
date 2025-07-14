package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	yamlpkg "vibethis/ono/pkg/yaml"
)

// Parser parses recipe files from disk
type Parser struct {
	logger *zap.Logger
}

// NewParser creates a new recipe parser
func NewParser(logger *zap.Logger) *Parser {
	return &Parser{
		logger: logger,
	}
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

// parseSingleFileRecipe parses a single-file recipe
func (p *Parser) parseSingleFileRecipe(filePath string) (*Recipe, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %w", err)
	}

	// Parse as workflow definition
	var workflow yamlpkg.WorkflowDefinition
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse recipe YAML: %w", err)
	}

	// Extract recipe info from workflow
	recipe := &Recipe{
		Name:         workflow.Name,
		Version:      workflow.Version,
		Description:  workflow.Description,
		BasePath:     filepath.Dir(filePath),
		ManifestPath: filePath,
		Workflow:     &workflow,
	}

	// Parse activities if present
	var doc struct {
		Activities []yamlpkg.ActivityDefinition `yaml:"activities"`
	}
	if err := yaml.Unmarshal(data, &doc); err == nil && len(doc.Activities) > 0 {
		recipe.Activities = doc.Activities
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
func (p *Parser) parseMultiFileRecipe(dirPath string) (*Recipe, error) {
	// Look for recipe.yaml manifest
	manifestPath := filepath.Join(dirPath, "recipe.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe manifest: %w", err)
	}

	// Parse manifest
	var manifest struct {
		Recipe struct {
			Name        string            `yaml:"name"`
			Version     string            `yaml:"version"`
			Description string            `yaml:"description"`
			Files       map[string]string `yaml:"files"`
		} `yaml:"recipe"`
	}
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse recipe manifest: %w", err)
	}

	recipe := &Recipe{
		Name:         manifest.Recipe.Name,
		Version:      manifest.Recipe.Version,
		Description:  manifest.Recipe.Description,
		BasePath:     dirPath,
		ManifestPath: manifestPath,
	}

	// Parse workflow file
	if workflowFile, ok := manifest.Recipe.Files["workflow"]; ok {
		workflow, err := p.parseWorkflowFile(filepath.Join(dirPath, workflowFile))
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow: %w", err)
		}
		recipe.Workflow = workflow
	}

	// Parse activities
	if activitiesPath, ok := manifest.Recipe.Files["activities"]; ok {
		activities, err := p.parseActivities(filepath.Join(dirPath, activitiesPath))
		if err != nil {
			return nil, fmt.Errorf("failed to parse activities: %w", err)
		}
		recipe.Activities = activities
	}

	// Parse agents
	if agentsPath, ok := manifest.Recipe.Files["agents"]; ok {
		agents, err := p.parseAgents(filepath.Join(dirPath, agentsPath))
		if err != nil {
			return nil, fmt.Errorf("failed to parse agents: %w", err)
		}
		recipe.Agents = agents
	}

	// Validate recipe
	if err := p.validateRecipe(recipe); err != nil {
		return nil, err
	}

	// Set last modified time
	fileInfo, err := os.Stat(manifestPath)
	if err == nil {
		recipe.LastModified = fileInfo.ModTime()
	}

	// Compute hash
	hc := NewHashComputer()
	recipe.Hash = hc.ComputeRecipeHash(recipe)

	return recipe, nil
}

// parseWorkflowFile parses a workflow definition file
func (p *Parser) parseWorkflowFile(filePath string) (*yamlpkg.WorkflowDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var workflow yamlpkg.WorkflowDefinition
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	return &workflow, nil
}

// parseActivities parses activities from a file or directory
func (p *Parser) parseActivities(path string) ([]yamlpkg.ActivityDefinition, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat activities path: %w", err)
	}

	var activities []yamlpkg.ActivityDefinition

	if info.IsDir() {
		// Parse all YAML files in directory
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read activities directory: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}

			filePath := filepath.Join(path, entry.Name())
			fileActivities, err := p.parseActivityFile(filePath)
			if err != nil {
				p.logger.Warn("Failed to parse activity file",
					zap.String("file", filePath),
					zap.Error(err))
				continue
			}

			activities = append(activities, fileActivities...)
		}
	} else {
		// Single file
		fileActivities, err := p.parseActivityFile(path)
		if err != nil {
			return nil, err
		}
		activities = fileActivities
	}

	return activities, nil
}

// parseActivityFile parses activities from a single file
func (p *Parser) parseActivityFile(filePath string) ([]yamlpkg.ActivityDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read activity file: %w", err)
	}

	// Try parsing as document with activities field first (most common format)
	var doc struct {
		Activities []yamlpkg.ActivityDefinition `yaml:"activities"`
	}
	if err := yaml.Unmarshal(data, &doc); err == nil {
		return doc.Activities, nil
	}

	// Try parsing as array
	var activities []yamlpkg.ActivityDefinition
	if err := yaml.Unmarshal(data, &activities); err == nil && len(activities) > 0 && activities[0].Name != "" {
		return activities, nil
	}

	// Try parsing as single activity
	var activity yamlpkg.ActivityDefinition
	if err := yaml.Unmarshal(data, &activity); err == nil && activity.Name != "" {
		return []yamlpkg.ActivityDefinition{activity}, nil
	}

	return nil, fmt.Errorf("failed to parse activity file")
}

// parseAgents parses agents from a file or directory
func (p *Parser) parseAgents(path string) (map[string]yamlpkg.AgentDefinition, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat agents path: %w", err)
	}

	agents := make(map[string]yamlpkg.AgentDefinition)

	if info.IsDir() {
		// Parse all YAML files in directory
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read agents directory: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}

			filePath := filepath.Join(path, entry.Name())
			fileAgents, err := p.parseAgentFile(filePath)
			if err != nil {
				p.logger.Warn("Failed to parse agent file",
					zap.String("file", filePath),
					zap.Error(err))
				continue
			}

			for name, agent := range fileAgents {
				agents[name] = agent
			}
		}
	} else {
		// Single file
		fileAgents, err := p.parseAgentFile(path)
		if err != nil {
			return nil, err
		}
		agents = fileAgents
	}

	return agents, nil
}

// parseAgentFile parses agents from a single file
func (p *Parser) parseAgentFile(filePath string) (map[string]yamlpkg.AgentDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent file: %w", err)
	}

	// Try parsing as map first
	var agentsMap map[string]yamlpkg.AgentDefinition
	if err := yaml.Unmarshal(data, &agentsMap); err == nil {
		return agentsMap, nil
	}

	// Try parsing as document with agents field (list)
	// Note: AgentDefinition doesn't have a Name field, so we can't support list format
	// We'll just return empty map for empty agent lists
	var doc struct {
		Agents []interface{} `yaml:"agents"`
	}
	if err := yaml.Unmarshal(data, &doc); err == nil && len(doc.Agents) == 0 {
		// Empty agents list
		return make(map[string]yamlpkg.AgentDefinition), nil
	}

	// Try parsing as document with agents field (map)
	var docMap struct {
		Agents map[string]yamlpkg.AgentDefinition `yaml:"agents"`
	}
	if err := yaml.Unmarshal(data, &docMap); err == nil {
		return docMap.Agents, nil
	}

	return nil, fmt.Errorf("failed to parse agent YAML: unsupported format")
}

// validateRecipe validates that a recipe has required fields
func (p *Parser) validateRecipe(recipe *Recipe) error {
	if recipe.Name == "" {
		return fmt.Errorf("recipe name is required")
	}

	if recipe.Version == "" {
		return fmt.Errorf("recipe version is required")
	}

	if recipe.Workflow == nil {
		return fmt.Errorf("workflow definition is required")
	}

	return nil
}

// ActivityDefinition represents an activity definition
type ActivityDefinition = yamlpkg.ActivityDefinition