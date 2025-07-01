package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"vibethis/core/pkg/core"
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
			t.Fatalf("Failed to create node directory: %v", err)
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
vcs:
  manager: 'git'
  defaultBranch: 'main'
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
		t.Fatalf("Failed to build graph: %v", err)
	}

	// Verify nodes
	if len(graph.Nodes) != 4 {
		t.Errorf("Expected 4 nodes, got %d", len(graph.Nodes))
	}

	// Create a map for easier lookup
	nodeMap := make(map[string]core.Node)
	for _, node := range graph.Nodes {
		nodeMap[node.ID] = node
	}

	// Verify each node exists (with prefix)
	baseName := filepath.Base(tempDir)
	for nodeName := range nodes {
		fullNodeID := fmt.Sprintf("%s-%s", baseName, nodeName)
		if _, exists := nodeMap[fullNodeID]; !exists {
			t.Errorf("Expected node %s not found in graph", fullNodeID)
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
		node, exists := nodeMap[tc.nodeID]
		if !exists {
			t.Errorf("Node %s not found", tc.nodeID)
			continue
		}

		if len(node.Dependencies) != len(tc.expected) {
			t.Errorf("Node %s: expected %d dependencies, got %d", tc.nodeID, len(tc.expected), len(node.Dependencies))
			continue
		}

		// Check each dependency
		depSet := make(map[string]bool)
		for _, dep := range node.Dependencies {
			depSet[dep] = true
		}

		for _, expected := range tc.expected {
			if !depSet[expected] {
				t.Errorf("Node %s: missing expected dependency %s", tc.nodeID, expected)
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

func TestBuildGraphWithExampleDirectory(t *testing.T) {
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
	
	// Use the actual example directory relative to the test file
	exampleDir := filepath.Join(testDir, "..", "..", "..", "..", ".example")
	absExampleDir, err := filepath.Abs(exampleDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	// Check if example directory exists
	if _, err := os.Stat(absExampleDir); os.IsNotExist(err) {
		t.Skip("Example directory not found, skipping test")
	}

	// Build the graph from the real example directory
	builder := New(absExampleDir)
	graph, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Failed to build graph from example directory: %v", err)
	}

	// Verify we got all 13 nodes from the example directory
	expectedNodes := []string{
		"example-api", "example-auth", "example-cache", "example-config", "example-database", "example-frontend",
		"example-gateway", "example-logger", "example-monitoring", "example-service-a", "example-service-b", 
		"example-service-c", "example-shared-utils",
	}

	if len(graph.Nodes) != len(expectedNodes) {
		t.Errorf("Expected %d nodes, got %d", len(expectedNodes), len(graph.Nodes))
		t.Logf("Nodes found: %v", func() []string {
			names := make([]string, len(graph.Nodes))
			for i, n := range graph.Nodes {
				names[i] = n.ID
			}
			return names
		}())
	}

	// Create a map for easier lookup
	nodeMap := make(map[string]core.Node)
	for _, node := range graph.Nodes {
		nodeMap[node.ID] = node
	}

	// Verify each expected node exists
	for _, nodeName := range expectedNodes {
		if _, exists := nodeMap[nodeName]; !exists {
			t.Errorf("Expected node %s not found in graph", nodeName)
		}
	}

	// Verify specific dependencies from our moon.yml files
	apiNode, exists := nodeMap["example-api"]
	if exists {
		expectedDeps := []string{"example-service-a", "example-service-b", "example-service-c", "example-auth"}
		if len(apiNode.Dependencies) != len(expectedDeps) {
			t.Errorf("API node: expected %d dependencies, got %d", len(expectedDeps), len(apiNode.Dependencies))
		}
		
		depSet := make(map[string]bool)
		for _, dep := range apiNode.Dependencies {
			depSet[dep] = true
		}
		
		for _, expected := range expectedDeps {
			if !depSet[expected] {
				t.Errorf("API node: missing expected dependency %s", expected)
			}
		}
	}

	// Verify edges exist
	if len(graph.Edges) == 0 {
		t.Error("Expected edges in the graph, but got none")
	}
}

func TestBuildGraphWithMockData(t *testing.T) {
	// This test simulates moon output without requiring moon to be installed
	mockOutput := `{
		"graph": {
			"nodes": [
				{
					"id": "example-api",
					"alias": "example/api",
					"config": {
						"id": "example-api",
						"language": "unknown",
						"project": {
							"description": "API service"
						},
						"dependsOn": [
							"example-database",
							"example-cache"
						]
					}
				},
				{
					"id": "example-frontend",
					"alias": "example/frontend",
					"config": {
						"id": "example-frontend",
						"language": "unknown",
						"project": {
							"description": "Frontend application"
						},
						"dependsOn": [
							"example-api"
						]
					}
				},
				{
					"id": "example-database",
					"alias": "example/database",
					"config": {
						"id": "example-database",
						"language": "unknown",
						"project": {
							"description": "Database service"
						},
						"dependsOn": []
					}
				},
				{
					"id": "example-cache",
					"alias": "example/cache",
					"config": {
						"id": "example-cache",
						"language": "unknown",
						"project": {
							"description": "Cache service"
						},
						"dependsOn": []
					}
				}
			]
		}
	}`

	// Parse the mock output to verify our data structures work
	var moonGraph MoonGraph
	if err := json.Unmarshal([]byte(mockOutput), &moonGraph); err != nil {
		t.Fatalf("Failed to parse mock moon output: %v", err)
	}

	// Verify we can parse the structure correctly
	if len(moonGraph.Graph.Nodes) != 4 {
		t.Errorf("Expected 4 nodes in mock data, got %d", len(moonGraph.Graph.Nodes))
	}

	// Verify first node structure
	firstNode := moonGraph.Graph.Nodes[0]
	if firstNode.Config.ID != "example-api" {
		t.Errorf("Expected first node ID to be 'example-api', got '%s'", firstNode.Config.ID)
	}

	// Parse the raw JSON dependsOn to check count
	var deps []string
	if err := json.Unmarshal(firstNode.Config.DependsOn, &deps); err != nil {
		t.Fatalf("Failed to parse dependencies: %v", err)
	}
	
	if len(deps) != 2 {
		t.Errorf("Expected first node to have 2 dependencies, got %d", len(deps))
	}
}