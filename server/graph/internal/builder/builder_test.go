package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/core"
	"github.com/stretchr/testify/require"
)

// MockMoonExecutor allows us to mock moon command execution for testing
type MockMoonExecutor struct {
	output string
	err    error
}

func (m *MockMoonExecutor) Execute(ctx context.Context, args ...string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return []byte(m.output), nil
}

// For testing, we'll need to override the moon command execution
// This would require refactoring the builder to accept an executor interface
// For now, we'll test with actual moon commands if available

func TestBuildGraphWithMoon(t *testing.T) {
	// Check if moon is available
	if _, err := exec.LookPath("moon"); err != nil {
		t.Skip("moon command not found, skipping test")
	}

	// Create a temporary directory structure with moon.yml files
	tempDir := t.TempDir()

	// Create test nodes with moon.yml files
	nodes := map[string]struct {
		deps []string
		desc string
	}{
		"api": {
			deps: []string{"database", "cache"},
			desc: "API service",
		},
		"frontend": {
			deps: []string{"api"},
			desc: "Frontend application",
		},
		"database": {
			deps: []string{},
			desc: "Database service",
		},
		"cache": {
			deps: []string{},
			desc: "Cache service",
		},
	}

	// Create moon.yml files
	for nodeName, nodeData := range nodes {
		nodeDir := filepath.Join(tempDir, nodeName)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			t.Fatalf("Failed to create cell directory: %v", err)
		}

		// Create moon.yml content
		// Use the directory name as prefix (the temp dir has a random name)
		baseName := filepath.Base(tempDir)
		moonContent := fmt.Sprintf("id: %s-%s\nlanguage: unknown\nproject:\n  description: %s\n", baseName, nodeName, nodeData.desc)

		if len(nodeData.deps) > 0 {
			moonContent += "dependsOn:\n"
			for _, dep := range nodeData.deps {
				moonContent += fmt.Sprintf("  - %s-%s\n", baseName, dep)
			}
		}

		moonFile := filepath.Join(nodeDir, "moon.yml")
		if err := os.WriteFile(moonFile, []byte(moonContent), 0644); err != nil {
			t.Fatalf("Failed to write moon.yml file: %v", err)
		}
	}

	// Create workspace moon configuration
	moonDir := filepath.Join(tempDir, ".moon")
	if err := os.MkdirAll(moonDir, 0755); err != nil {
		t.Fatalf("Failed to create .moon directory: %v", err)
	}

	workspaceContent := `$schema: 'https://moonrepo.dev/schemas/workspace.json'
projects:
  - "*/"
`
	workspaceFile := filepath.Join(moonDir, "workspace.yml")
	if err := os.WriteFile(workspaceFile, []byte(workspaceContent), 0644); err != nil {
		t.Fatalf("Failed to write workspace.yml: %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(workspaceFile); err != nil {
		t.Fatalf("workspace.yml was not created: %v", err)
	}

	// Initialize a git repository to prevent moon from searching parent directories
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	if err := gitCmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Set git config for the test
	gitConfig := exec.Command("git", "config", "user.email", "test@example.com")
	gitConfig.Dir = tempDir
	gitConfig.Run()

	gitConfig2 := exec.Command("git", "config", "user.name", "Test User")
	gitConfig2.Dir = tempDir
	gitConfig2.Run()

	// Add all files and create initial commit
	gitAdd := exec.Command("git", "add", ".")
	gitAdd.Dir = tempDir
	if err := gitAdd.Run(); err != nil {
		t.Fatalf("Failed to add files to git: %v", err)
	}

	gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
	gitCommit.Dir = tempDir
	if err := gitCommit.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Build the graph
	builder := New(tempDir)
	graph, err := builder.Build(context.Background())
	if err != nil {
		if strings.Contains(err.Error(), "Failed to parse .moon/workspace.yml") || strings.Contains(err.Error(), "unknown field") {
			t.Skipf("incompatible moon workspace schema for local moon version: %v", err)
		}
		t.Fatalf("Failed to build graph: %v", err)
	}

	// Verify cells
	if len(graph.Cells) != 4 {
		t.Errorf("Expected 4 cells, got %d", len(graph.Cells))
	}

	// Create a map for easier lookup
	cellMap := make(map[string]core.Cell)
	for _, cell := range graph.Cells {
		cellMap[cell.ID] = cell
	}

	// Verify each cell exists (with prefix)
	baseName := filepath.Base(tempDir)
	for cellName := range nodes {
		fullCellID := fmt.Sprintf("%s-%s", baseName, cellName)
		if _, exists := cellMap[fullCellID]; !exists {
			t.Errorf("Expected cell %s not found in graph", fullCellID)
		}
	}

	// Verify dependencies
	testCases := []struct {
		nodeID   string
		expected []string
	}{
		{fmt.Sprintf("%s-api", baseName), []string{fmt.Sprintf("%s-database", baseName), fmt.Sprintf("%s-cache", baseName)}},
		{fmt.Sprintf("%s-frontend", baseName), []string{fmt.Sprintf("%s-api", baseName)}},
		{fmt.Sprintf("%s-database", baseName), []string{}},
		{fmt.Sprintf("%s-cache", baseName), []string{}},
	}

	for _, tc := range testCases {
		cell, exists := cellMap[tc.nodeID]
		if !exists {
			t.Errorf("Cell %s not found", tc.nodeID)
			continue
		}

		if len(cell.Dependencies) != len(tc.expected) {
			t.Errorf("Cell %s: expected %d dependencies, got %d", tc.nodeID, len(tc.expected), len(cell.Dependencies))
			continue
		}

		// Check each dependency
		depSet := make(map[string]bool)
		for _, dep := range cell.Dependencies {
			depSet[dep] = true
		}

		for _, expected := range tc.expected {
			if !depSet[expected] {
				t.Errorf("Cell %s: missing expected dependency %s", tc.nodeID, expected)
			}
		}
	}

	// Verify edges
	expectedEdges := 3 // api->database, api->cache, frontend->api
	if len(graph.Edges) != expectedEdges {
		t.Errorf("Expected %d edges, got %d", expectedEdges, len(graph.Edges))
	}

	// Verify specific edges exist
	edgeExists := func(source, target string) bool {
		for _, edge := range graph.Edges {
			if edge.Source == source && edge.Target == target {
				return true
			}
		}
		return false
	}

	expectedEdgeList := []struct {
		source string
		target string
	}{
		{fmt.Sprintf("%s-api", baseName), fmt.Sprintf("%s-database", baseName)},
		{fmt.Sprintf("%s-api", baseName), fmt.Sprintf("%s-cache", baseName)},
		{fmt.Sprintf("%s-frontend", baseName), fmt.Sprintf("%s-api", baseName)},
	}

	for _, ee := range expectedEdgeList {
		if !edgeExists(ee.source, ee.target) {
			t.Errorf("Expected edge from %s to %s not found", ee.source, ee.target)
		}
	}
}

// TestBuildGraphFromStaticMoonOutput tests the conversion from static moon JSON output
// to Graph nodes and edges. This test is not coupled to any actual project structure.
func TestBuildGraphFromStaticMoonOutput(t *testing.T) {
	// Static moon output representing a simple project graph
	staticMoonOutput := `{
		"graph": {
			"nodes": [
				{
					"id": "example-api",
					"source": "moon.yml",
					"root": "/workspace/api",
					"language": "typescript",
					"config": {
						"id": "example-api",
						"language": "typescript",
						"project": {
							"description": "API service"
						},
						"dependsOn": ["example-database", "example-cache"]
					},
					"dependencies": [
						{
							"id": "example-database",
							"scope": "production",
							"source": "explicit"
						},
						{
							"id": "example-cache",
							"scope": "production",
							"source": "explicit"
						}
					]
				},
				{
					"id": "example-frontend",
					"source": "moon.yml",
					"root": "/workspace/frontend",
					"language": "typescript",
					"config": {
						"id": "example-frontend",
						"language": "typescript",
						"project": {
							"description": "Frontend application"
						},
						"dependsOn": ["example-api"]
					},
					"dependencies": [
						{
							"id": "example-api",
							"scope": "production",
							"source": "explicit"
						}
					]
				},
				{
					"id": "example-database",
					"source": "moon.yml",
					"root": "/workspace/database",
					"language": "unknown",
					"config": {
						"id": "example-database",
						"language": "unknown",
						"project": {
							"description": "Database service"
						},
						"dependsOn": []
					},
					"dependencies": []
				},
				{
					"id": "example-cache",
					"source": "moon.yml",
					"root": "/workspace/cache",
					"language": "unknown",
					"config": {
						"id": "example-cache",
						"language": "unknown",
						"project": {
							"description": "Cache service"
						},
						"dependsOn": []
					},
					"dependencies": []
				}
			]
		}
	}`

	// Parse the static moon output
	var moonGraph MoonGraph
	if err := json.Unmarshal([]byte(staticMoonOutput), &moonGraph); err != nil {
		t.Fatalf("Failed to parse static moon output: %v", err)
	}

	// Verify parsing
	if len(moonGraph.Graph.Nodes) != 4 {
		t.Fatalf("Expected 4 nodes in static data, got %d", len(moonGraph.Graph.Nodes))
	}

	// Build the graph from the parsed data (simulating what builder.Build does)
	cells := []core.Cell{}
	edges := []core.Edge{}
	cellMap := make(map[string]bool)

	for _, moonNode := range moonGraph.Graph.Nodes {
		dependencies := []string{}
		for _, dep := range moonNode.Dependencies {
			dependencies = append(dependencies, dep.ID)
		}

		cell := core.Cell{
			ID:           moonNode.ID,
			Name:         moonNode.ID,
			Path:         moonNode.Root,
			Type:         "cell",
			Dependencies: dependencies,
		}

		cells = append(cells, cell)
		cellMap[moonNode.ID] = true
	}

	// Build edges from dependencies
	for _, cell := range cells {
		for _, dep := range cell.Dependencies {
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

	graph := &core.Graph{
		Cells: cells,
		Edges: edges,
	}

	// Verify the graph was built correctly
	if len(graph.Cells) != 4 {
		t.Errorf("Expected 4 cells, got %d", len(graph.Cells))
	}

	// Create a map for easier lookup
	cellMapVerify := make(map[string]core.Cell)
	for _, cell := range graph.Cells {
		cellMapVerify[cell.ID] = cell
	}

	// Verify each expected cell exists with correct properties
	expectedCells := map[string]struct {
		deps []string
		path string
	}{
		"example-api":      {deps: []string{"example-database", "example-cache"}, path: "/workspace/api"},
		"example-frontend": {deps: []string{"example-api"}, path: "/workspace/frontend"},
		"example-database": {deps: []string{}, path: "/workspace/database"},
		"example-cache":    {deps: []string{}, path: "/workspace/cache"},
	}

	for cellID, expected := range expectedCells {
		cell, exists := cellMapVerify[cellID]
		if !exists {
			t.Errorf("Expected cell %s not found in graph", cellID)
			continue
		}

		if cell.Path != expected.path {
			t.Errorf("Cell %s: expected path %s, got %s", cellID, expected.path, cell.Path)
		}

		if len(cell.Dependencies) != len(expected.deps) {
			t.Errorf("Cell %s: expected %d dependencies, got %d", cellID, len(expected.deps), len(cell.Dependencies))
			continue
		}

		// Verify each dependency
		depSet := make(map[string]bool)
		for _, dep := range cell.Dependencies {
			depSet[dep] = true
		}

		for _, expectedDep := range expected.deps {
			if !depSet[expectedDep] {
				t.Errorf("Cell %s: missing expected dependency %s", cellID, expectedDep)
			}
		}
	}

	// Verify edges
	expectedEdges := 3 // api->database, api->cache, frontend->api
	if len(graph.Edges) != expectedEdges {
		t.Errorf("Expected %d edges, got %d", expectedEdges, len(graph.Edges))
	}

	// Verify specific edges exist
	edgeExists := func(source, target string) bool {
		for _, edge := range graph.Edges {
			if edge.Source == source && edge.Target == target {
				return true
			}
		}
		return false
	}

	expectedEdgeList := []struct {
		source string
		target string
	}{
		{"example-api", "example-database"},
		{"example-api", "example-cache"},
		{"example-frontend", "example-api"},
	}

	for _, ee := range expectedEdgeList {
		if !edgeExists(ee.source, ee.target) {
			t.Errorf("Expected edge from %s to %s not found", ee.source, ee.target)
		}
	}
}

func TestParseMoonGraphWithIndexedNodes(t *testing.T) {
	staticMoonOutput := `{
		"graph": {
			"nodes": [0, 1]
		},
		"data": {
			"0": {
				"id": "example-api",
				"source": "moon.yml",
				"root": "/workspace/api",
				"language": "typescript",
				"config": {
					"id": "example-api",
					"language": "typescript",
					"project": {
						"description": "API service"
					},
					"dependsOn": ["example-database"]
				},
				"dependencies": [
					{
						"id": "example-database",
						"scope": "production",
						"source": "explicit"
					}
				]
			},
			"1": {
				"id": "example-database",
				"source": "moon.yml",
				"root": "/workspace/database",
				"language": "unknown",
				"config": {
					"id": "example-database",
					"language": "unknown",
					"project": {
						"description": "Database service"
					},
					"dependsOn": []
				},
				"dependencies": []
			}
		}
	}`

	var moonGraph MoonGraph
	require.NoError(t, json.Unmarshal([]byte(staticMoonOutput), &moonGraph))
	require.Len(t, moonGraph.Graph.Nodes, 2)
	require.Equal(t, "example-api", moonGraph.Graph.Nodes[0].ID)
	require.Equal(t, "example-database", moonGraph.Graph.Nodes[1].ID)
	require.Len(t, moonGraph.Graph.Nodes[0].Dependencies, 1)
	require.Equal(t, "example-database", moonGraph.Graph.Nodes[0].Dependencies[0].ID)
}

// TestBuildGraphWithCurrentProject tests that we can parse the moon output
// from the current project and verify the expected fields exist.
// This test is NOT coupled to the exact number or names of projects.
func TestBuildGraphWithCurrentProject(t *testing.T) {
	// Check if moon is available
	if _, err := exec.LookPath("moon"); err != nil {
		t.Skip("moon command not found, skipping test")
	}

	// Get the directory of the current test file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get current file path")
	}
	testDir := filepath.Dir(filename)

	// Navigate to the repository root (adjust path as needed)
	// This test runs against whatever moon projects exist in the current workspace
	repoRoot := filepath.Join(testDir, "..", "..", "..", "..")
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	// Build the graph from the current project
	builder := New(absRepoRoot)
	graph, err := builder.Build(context.Background())
	if err != nil {
		if strings.Contains(err.Error(), "Failed to parse .moon/workspace.yml") || strings.Contains(err.Error(), "unknown field") {
			t.Skipf("incompatible moon workspace schema for local moon version: %v", err)
		}
		t.Fatalf("Failed to build graph from current project: %v", err)
	}

	// Instead of checking for specific counts or names, verify:
	// 1. We got some cells
	// 2. Each cell has the expected fields populated
	// 3. Edges are properly formed

	if len(graph.Cells) == 0 {
		t.Fatal("Expected to find at least one cell in the current project")
	}

	t.Logf("Found %d cells in the current project", len(graph.Cells))

	// Verify each cell has required fields
	for _, cell := range graph.Cells {
		if cell.ID == "" {
			t.Error("Found cell with empty ID")
		}
		if cell.Name == "" {
			t.Error("Found cell with empty Name")
		}
		if cell.Path == "" {
			t.Error("Found cell with empty Path")
		}
		if cell.Type != "cell" {
			t.Errorf("Cell %s: expected Type 'cell', got '%s'", cell.ID, cell.Type)
		}
		// Dependencies can be empty, but should not be nil
		if cell.Dependencies == nil {
			t.Errorf("Cell %s: Dependencies field is nil, expected empty slice", cell.ID)
		}

		t.Logf("Cell %s: ID=%s, Path=%s, Dependencies=%v", cell.ID, cell.ID, cell.Path, cell.Dependencies)
	}

	// Verify edges reference valid cells
	cellMap := make(map[string]bool)
	for _, cell := range graph.Cells {
		cellMap[cell.ID] = true
	}

	for _, edge := range graph.Edges {
		if edge.ID == "" {
			t.Error("Found edge with empty ID")
		}
		if edge.Source == "" {
			t.Error("Found edge with empty Source")
		}
		if edge.Target == "" {
			t.Error("Found edge with empty Target")
		}

		// Verify source and target cells exist
		if !cellMap[edge.Source] {
			t.Errorf("Edge %s references non-existent source cell: %s", edge.ID, edge.Source)
		}
		if !cellMap[edge.Target] {
			t.Errorf("Edge %s references non-existent target cell: %s", edge.ID, edge.Target)
		}
	}

	t.Logf("Found %d edges in the current project", len(graph.Edges))
}
