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
	Alias  string `json:"alias"`
	Config struct {
		ID       string          `json:"id"`
		Language string          `json:"language"`
		Project  struct {
			Description string `json:"description"`
		} `json:"project"`
		DependsOn json.RawMessage `json:"dependsOn"`
	} `json:"config"`
	ID string `json:"id"`
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
	// We only want nodes that are in our rootPath directory
	rootName := filepath.Base(b.rootPath)
	
	// Handle special case where directory starts with dot (e.g., .example)
	// but moon IDs don't include the dot (e.g., example-api instead of .example-api)
	idPrefix := rootName
	if strings.HasPrefix(rootName, ".") {
		idPrefix = strings.TrimPrefix(rootName, ".")
	}
	
	for _, moonNode := range moonGraph.Graph.Nodes {
		// Only include nodes that belong to our root directory
		// For example, if rootPath is "/path/to/.example", we want nodes starting with "example-"
		if !strings.HasPrefix(moonNode.Config.ID, idPrefix+"-") {
			continue
		}
		
		// Extract the box name from the ID by removing the prefix
		boxName := strings.TrimPrefix(moonNode.Config.ID, idPrefix+"-")
		
		// Build dependencies list
		dependencies := []string{}
		
		// Parse dependsOn which can be either []string or []MoonDependency
		if len(moonNode.Config.DependsOn) > 0 {
			// Try to parse as array of strings first
			var stringDeps []string
			if err := json.Unmarshal(moonNode.Config.DependsOn, &stringDeps); err == nil {
				for _, dep := range stringDeps {
					// Only include dependencies from our root directory
					if strings.HasPrefix(dep, idPrefix+"-") {
						depName := strings.TrimPrefix(dep, idPrefix+"-")
						dependencies = append(dependencies, depName)
					}
				}
			} else {
				// Try to parse as array of objects
				var objDeps []MoonDependency
				if err := json.Unmarshal(moonNode.Config.DependsOn, &objDeps); err == nil {
					for _, dep := range objDeps {
						// Only include dependencies from our root directory
						if strings.HasPrefix(dep.ID, idPrefix+"-") {
							depName := strings.TrimPrefix(dep.ID, idPrefix+"-")
							dependencies = append(dependencies, depName)
						}
					}
				}
			}
		}

		node := core.Node{
			ID:           boxName,
			Name:         boxName,
			Path:         filepath.Join(b.rootPath, boxName),
			Type:         "box",
			Dependencies: dependencies,
		}

		nodes = append(nodes, node)
		nodeMap[boxName] = true
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