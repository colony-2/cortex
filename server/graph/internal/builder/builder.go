package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/colony-2/c2j/pkg/core"
)

// MoonGraph represents the moon project-graph JSON output.
// Moon 2.x can emit either embedded node objects in graph.nodes or
// integer references with the node payloads stored in the top-level data map.
type MoonGraph struct {
	Graph struct {
		Nodes []MoonNode `json:"nodes"`
	} `json:"graph"`
}

type rawMoonGraph struct {
	Graph struct {
		Nodes json.RawMessage `json:"nodes"`
	} `json:"graph"`
	Data map[string]MoonNode `json:"data"`
}

func (g *MoonGraph) UnmarshalJSON(data []byte) error {
	var raw rawMoonGraph
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	nodes, err := parseMoonNodes(raw.Graph.Nodes, raw.Data)
	if err != nil {
		return err
	}

	g.Graph.Nodes = nodes
	return nil
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

func parseMoonNodes(rawNodes json.RawMessage, indexedNodes map[string]MoonNode) ([]MoonNode, error) {
	if len(rawNodes) == 0 || string(rawNodes) == "null" {
		return nil, nil
	}

	var embedded []MoonNode
	if err := json.Unmarshal(rawNodes, &embedded); err == nil {
		return embedded, nil
	}

	var refs []json.RawMessage
	if err := json.Unmarshal(rawNodes, &refs); err != nil {
		return nil, fmt.Errorf("decode moon graph nodes: %w", err)
	}

	nodes := make([]MoonNode, 0, len(refs))
	for _, ref := range refs {
		var embeddedNode MoonNode
		if err := json.Unmarshal(ref, &embeddedNode); err == nil && embeddedNode.ID != "" {
			nodes = append(nodes, embeddedNode)
			continue
		}

		key, err := parseMoonNodeRef(ref)
		if err != nil {
			return nil, err
		}
		node, ok := indexedNodes[key]
		if !ok {
			return nil, fmt.Errorf("missing moon node payload for ref %q", key)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func parseMoonNodeRef(rawRef json.RawMessage) (string, error) {
	var intRef int
	if err := json.Unmarshal(rawRef, &intRef); err == nil {
		return strconv.Itoa(intRef), nil
	}

	var stringRef string
	if err := json.Unmarshal(rawRef, &stringRef); err == nil && strings.TrimSpace(stringRef) != "" {
		return stringRef, nil
	}

	return "", fmt.Errorf("unsupported moon node reference: %s", string(rawRef))
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

		// Calculate relative path from repository root
		relPath, err := filepath.Rel(rootPathWithSymlinks[:len(rootPathWithSymlinks)-1], absNodePath)

		// Debug: Log each cell being processed
		fmt.Printf("[DEBUG] Processing cell %s with absolute path %s, relative path %s\n", moonNode.ID, absNodePath, relPath)
		if err != nil {
			// If we can't get a relative path, log a warning but continue
			fmt.Printf("[WARN] Failed to get relative path for cell %s: %v\n", moonNode.ID, err)
			relPath = absNodePath // Fallback to absolute path
		}

		// Build dependencies list
		dependencies := []string{}
		for _, dep := range moonNode.Dependencies {
			dependencies = append(dependencies, dep.ID)
		}

		cell := core.Cell{
			ID:           moonNode.ID,
			Name:         moonNode.ID,
			Path:         relPath,
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
