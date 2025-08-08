package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
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
	ID       string `json:"id"`
	Source   string `json:"source"`
	Root     string `json:"root"`
	Language string `json:"language"`
	Config   struct {
		ID       string `json:"id"`
		Language string `json:"language"`
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
	// Ensure rootPath is absolute before using it
	absRootPath, err := filepath.Abs(b.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Debug: Log the rootPath being used
	fmt.Printf("[DEBUG] Graph builder using rootPath: %s\n", absRootPath)

	// Execute moon project-graph command
	cmd := exec.CommandContext(ctx, "moon", "project-graph", "--json")
	cmd.Dir = absRootPath
	cmd.Env = append(os.Environ(), fmt.Sprintf("MOON_WORKSPACE_ROOT=%s", absRootPath))
	output, err := cmd.Output()

	// Debug: Log the command output
	fmt.Printf("[DEBUG] Root path:\n%s\n", string(absRootPath))

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

	// Debug: Log the number of cells from moon
	fmt.Printf("[DEBUG] Moon returned %d cells\n", len(moonGraph.Graph.Nodes))

	cells := []core.Cell{}
	edges := []core.Edge{}
	cellMap := make(map[string]bool)

	// Build cells from moon graph
	// Handle symlinks in the root path
	rootPathWithSymlinks := absRootPath
	if evalPath, err := filepath.EvalSymlinks(absRootPath); err == nil {
		rootPathWithSymlinks = evalPath
	}

	// Ensure paths end with separator for proper prefix matching
	if !strings.HasSuffix(rootPathWithSymlinks, string(filepath.Separator)) {
		rootPathWithSymlinks += string(filepath.Separator)
	}

	// Debug: Log the rootPath being used for filtering
	fmt.Printf("[DEBUG] Filtering cells with rootPath: %s\n", rootPathWithSymlinks)

	for _, moonNode := range moonGraph.Graph.Nodes {
		// Check if this cell's root path is within our rootPath
		// Handle symlinks by evaluating them
		nodeRoot := moonNode.Root
		if evalPath, err := filepath.EvalSymlinks(nodeRoot); err == nil {
			nodeRoot = evalPath
		}
		absNodePath, _ := filepath.Abs(nodeRoot)

		// Debug: Log each cell being processed
		fmt.Printf("[DEBUG] Processing cell %s with path %s\n", moonNode.ID, absNodePath)

		// Build dependencies list
		dependencies := []string{}
		for _, dep := range moonNode.Dependencies {
			dependencies = append(dependencies, dep.ID)
		}

		cell := core.Cell{
			ID:           moonNode.ID,
			Name:         moonNode.ID,
			Path:         absNodePath,
			Type:         "cell",
			Dependencies: dependencies,
		}

		cells = append(cells, cell)
		cellMap[moonNode.ID] = true
	}

	// Build edges from dependencies
	for _, cell := range cells {
		for _, dep := range cell.Dependencies {
			// Only create edge if target exists in our cell set
			if cellMap[dep] {
				edge := core.Edge{
					ID:     fmt.Sprintf("%s-%s", cell.ID, dep),
					Source: cell.ID,
					Target: dep,
				}
				edges = append(edges, edge)
			}
		}
	}

	// Debug: Log final result
	fmt.Printf("[DEBUG] Final graph has %d cells and %d edges\n", len(cells), len(edges))

	return &core.Graph{
		Cells: cells,
		Edges: edges,
	}, nil
}
