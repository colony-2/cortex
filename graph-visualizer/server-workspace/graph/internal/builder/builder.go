package builder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"vibethis/core/pkg/core"
	"vibethis/graph/internal/validator"
)

// Dependency represents the structure of dependencies.yaml file
type Dependency struct {
	Dependencies []string `yaml:"dependencies"`
}

// Builder handles graph construction from the filesystem
type Builder struct {
	rootPath string
}

// New creates a new graph builder
func New(rootPath string) *Builder {
	return &Builder{
		rootPath: rootPath,
	}
}

// Build constructs the dependency graph from the filesystem
func (b *Builder) Build(ctx context.Context) (*core.Graph, error) {
	nodes := []core.Node{}
	edges := []core.Edge{}

	// Read all directories in rootPath
	entries, err := os.ReadDir(b.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read root directory: %w", err)
	}

	// Build nodes
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		node := core.Node{
			ID:           entry.Name(),
			Name:         entry.Name(),
			Path:         filepath.Join(b.rootPath, entry.Name()),
			Type:         "box",
			Dependencies: []string{},
		}

		// Check for dependencies.yaml
		depFile := filepath.Join(node.Path, "dependencies.yaml")
		if data, err := os.ReadFile(depFile); err == nil {
			var dep Dependency
			if err := yaml.Unmarshal(data, &dep); err == nil && dep.Dependencies != nil {
				node.Dependencies = dep.Dependencies
			}
		}

		nodes = append(nodes, node)
	}

	// Build edges from dependencies
	for _, node := range nodes {
		for _, dep := range node.Dependencies {
			edge := core.Edge{
				ID:     fmt.Sprintf("%s-%s", node.ID, dep),
				Source: node.ID,
				Target: dep,
			}
			edges = append(edges, edge)
		}
	}

	return &core.Graph{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// UpdateDependencies updates the dependencies for a specific node
func (b *Builder) UpdateDependencies(ctx context.Context, nodeID string, dependencies []string) error {
	nodePath := filepath.Join(b.rootPath, nodeID)
	
	// Check if node exists
	if _, err := os.Stat(nodePath); os.IsNotExist(err) {
		return fmt.Errorf("node %s does not exist", nodeID)
	}

	// Validate dependencies exist
	for _, dep := range dependencies {
		depPath := filepath.Join(b.rootPath, dep)
		if _, err := os.Stat(depPath); os.IsNotExist(err) {
			return fmt.Errorf("dependency %s does not exist", dep)
		}
	}

	// Check for self-dependency
	for _, dep := range dependencies {
		if dep == nodeID {
			return fmt.Errorf("node cannot depend on itself")
		}
	}

	// Check for circular dependencies
	graph, err := b.Build(ctx)
	if err != nil {
		return fmt.Errorf("failed to build graph for validation: %w", err)
	}
	
	v := validator.New()
	if err := v.ValidateDependencies(graph, nodeID, dependencies); err != nil {
		return err
	}

	// Update the dependencies.yaml file
	dep := Dependency{Dependencies: dependencies}
	data, err := yaml.Marshal(&dep)
	if err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}

	depFile := filepath.Join(nodePath, "dependencies.yaml")
	if err := os.WriteFile(depFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write dependencies file: %w", err)
	}

	return nil
}