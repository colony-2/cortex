package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"vibethis/core/pkg/core"
)

// MoonGraph represents the moon project-graph JSON output
type MoonGraph struct {
	Graph struct {
		Nodes []MoonNode `json:"nodes"`
	} `json:"graph"`
}

// MoonDependency can be either a string or an object
type MoonDependency struct {
	ID     string  `json:"id"`
	Scope  string  `json:"scope"`
	Source string  `json:"source"`
	Via    *string `json:"via"`
}

// MoonNode represents a node in the moon project graph
type MoonNode struct {
	ID           string           `json:"id"`
	Source       string           `json:"source"`
	Root         string           `json:"root"`
	Language     string           `json:"language"`
	Config       struct {
		ID       string          `json:"id"`
		Language string          `json:"language"`
		Project  struct {
			Description string `json:"description"`
		} `json:"project"`
		DependsOn json.RawMessage `json:"dependsOn"`
	} `json:"config"`
	Dependencies []MoonDependency `json:"dependencies"`
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

// Build constructs the dependency graph from moon's project-graph output
func (b *Builder) Build(ctx context.Context) (*core.Graph, error) {
	// Execute moon project-graph command
	cmd := exec.CommandContext(ctx, "moon", "project-graph", "--json")
	cmd.Dir = b.rootPath
	output, err := cmd.Output()
	if err != nil {
		// If error, capture combined output for debugging
		if execErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to execute moon project-graph: %w\nStderr: %s", err, string(execErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute moon project-graph: %w", err)
	}

	// Parse the JSON output
	var moonGraph MoonGraph
	if err := json.Unmarshal(output, &moonGraph); err != nil {
		return nil, fmt.Errorf("failed to parse moon graph output: %w", err)
	}

	nodes := []core.Node{}
	edges := []core.Edge{}
	nodeMap := make(map[string]bool)

	// Build nodes from moon graph
	// Get the absolute path of the root directory to handle symlinks
	absRootPath, err := filepath.EvalSymlinks(b.rootPath)
	if err != nil {
		absRootPath = b.rootPath
	}
	absRootPath, _ = filepath.Abs(absRootPath)
	
	for _, moonNode := range moonGraph.Graph.Nodes {
		// Check if this node's root path is within our rootPath
		// Handle symlinks by evaluating them
		nodeRoot := moonNode.Root
		if evalPath, err := filepath.EvalSymlinks(nodeRoot); err == nil {
			nodeRoot = evalPath
		}
		absNodePath, _ := filepath.Abs(nodeRoot)
		
		// Skip nodes that are not within our rootPath
		if !strings.HasPrefix(absNodePath, absRootPath) {
			continue
		}
		
		// Build dependencies list
		dependencies := []string{}
		for _, dep := range moonNode.Dependencies {
			dependencies = append(dependencies, dep.ID)
		}

		node := core.Node{
			ID:           moonNode.ID,
			Name:         moonNode.ID,
			Path:         moonNode.Root,
			Type:         "box",
			Dependencies: dependencies,
		}

		nodes = append(nodes, node)
		nodeMap[moonNode.ID] = true
	}

	// Build edges from dependencies
	for _, node := range nodes {
		for _, dep := range node.Dependencies {
			// Only create edge if target exists in our node set
			if nodeMap[dep] {
				edge := core.Edge{
					ID:     fmt.Sprintf("%s-%s", node.ID, dep),
					Source: node.ID,
					Target: dep,
				}
				edges = append(edges, edge)
			}
		}
	}

	return &core.Graph{
		Nodes: nodes,
		Edges: edges,
	}, nil
}