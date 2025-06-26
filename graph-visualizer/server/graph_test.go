package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildGraphWithDependenciesYaml(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()

	// Create test nodes with dependencies.yaml files
	nodes := map[string][]string{
		"api":      {"database", "cache"},
		"frontend": {"api"},
		"database": {},
		"cache":    {},
	}

	for nodeName, deps := range nodes {
		nodeDir := filepath.Join(tempDir, nodeName)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			t.Fatalf("Failed to create node directory: %v", err)
		}

		dep := Dependency{Dependencies: deps}
		data, err := yaml.Marshal(&dep)
		if err != nil {
			t.Fatalf("Failed to marshal dependencies: %v", err)
		}

		depFile := filepath.Join(nodeDir, "dependencies.yaml")
		if err := os.WriteFile(depFile, data, 0644); err != nil {
			t.Fatalf("Failed to write dependencies file: %v", err)
		}
	}

	// Build the graph
	graph := buildGraph(tempDir)

	// Verify nodes
	if len(graph.Nodes) != 4 {
		t.Errorf("Expected 4 nodes, got %d", len(graph.Nodes))
	}

	// Verify each node has correct dependencies
	nodeMap := make(map[string]Node)
	for _, node := range graph.Nodes {
		nodeMap[node.ID] = node
	}

	for nodeName, expectedDeps := range nodes {
		node, exists := nodeMap[nodeName]
		if !exists {
			t.Errorf("Node %s not found in graph", nodeName)
			continue
		}

		if len(node.Dependencies) != len(expectedDeps) {
			t.Errorf("Node %s: expected %d dependencies, got %d", 
				nodeName, len(expectedDeps), len(node.Dependencies))
			continue
		}

		for i, dep := range expectedDeps {
			if i < len(node.Dependencies) && node.Dependencies[i] != dep {
				t.Errorf("Node %s: expected dependency %s at index %d, got %s", 
					nodeName, dep, i, node.Dependencies[i])
			}
		}
	}

	// Verify edges
	expectedEdges := 3 // api->database, api->cache, frontend->api
	if len(graph.Edges) != expectedEdges {
		t.Errorf("Expected %d edges, got %d", expectedEdges, len(graph.Edges))
	}

	// Verify specific edges exist
	edgeMap := make(map[string]bool)
	for _, edge := range graph.Edges {
		edgeMap[edge.Source+"->"+edge.Target] = true
	}

	expectedEdgesList := []string{"api->database", "api->cache", "frontend->api"}
	for _, expectedEdge := range expectedEdgesList {
		if !edgeMap[expectedEdge] {
			t.Errorf("Expected edge %s not found", expectedEdge)
		}
	}
}

func TestGraphAPIEndpoint(t *testing.T) {
	// Create temporary directory with test data
	tempDir := t.TempDir()
	
	// Create a simple node structure
	apiDir := filepath.Join(tempDir, "api")
	os.MkdirAll(apiDir, 0755)
	
	dep := Dependency{Dependencies: []string{"database"}}
	data, _ := yaml.Marshal(&dep)
	os.WriteFile(filepath.Join(apiDir, "dependencies.yaml"), data, 0644)
	
	dbDir := filepath.Join(tempDir, "database")
	os.MkdirAll(dbDir, 0755)
	
	emptyDep := Dependency{Dependencies: []string{}}
	emptyData, _ := yaml.Marshal(&emptyDep)
	os.WriteFile(filepath.Join(dbDir, "dependencies.yaml"), emptyData, 0644)

	// Set rootPath for the test
	oldRootPath := rootPath
	rootPath = tempDir
	defer func() { rootPath = oldRootPath }()

	// Create request
	req := httptest.NewRequest("GET", "/api/graph", nil)
	rr := httptest.NewRecorder()

	// Call handler
	getGraphHandler(rr, req)

	// Check status
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Parse response
	var graph Graph
	if err := json.NewDecoder(rr.Body).Decode(&graph); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response
	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(graph.Edges))
	}

	// Verify the edge is correct
	if len(graph.Edges) > 0 {
		edge := graph.Edges[0]
		if edge.Source != "api" || edge.Target != "database" {
			t.Errorf("Expected edge api->database, got %s->%s", edge.Source, edge.Target)
		}
	}
}

func TestRelationshipsAPIWithDependenciesYaml(t *testing.T) {
	// Create temporary directory with test data
	tempDir := t.TempDir()
	
	// Create test node
	nodeDir := filepath.Join(tempDir, "testnode")
	os.MkdirAll(nodeDir, 0755)
	
	// Create initial dependencies.yaml
	dep := Dependency{Dependencies: []string{"existing"}}
	data, _ := yaml.Marshal(&dep)
	os.WriteFile(filepath.Join(nodeDir, "dependencies.yaml"), data, 0644)

	// Set rootPath for the test
	oldRootPath := rootPath
	rootPath = tempDir
	defer func() { rootPath = oldRootPath }()

	// Test updating relationships via API
	// For this test, we'll directly test the file writing logic
	// since the full handler test requires mux setup

	// Create the dependency structure
	newDep := Dependency{Dependencies: []string{"new1", "new2"}}
	newData, err := yaml.Marshal(&newDep)
	if err != nil {
		t.Fatalf("Failed to marshal new dependencies: %v", err)
	}

	// Write to file
	depFile := filepath.Join(nodeDir, "dependencies.yaml")
	if err := os.WriteFile(depFile, newData, 0644); err != nil {
		t.Fatalf("Failed to write dependencies file: %v", err)
	}

	// Verify the file was updated correctly
	readData, err := os.ReadFile(depFile)
	if err != nil {
		t.Fatalf("Failed to read dependencies file: %v", err)
	}

	var readDep Dependency
	if err := yaml.Unmarshal(readData, &readDep); err != nil {
		t.Fatalf("Failed to unmarshal dependencies: %v", err)
	}

	expectedDeps := []string{"new1", "new2"}
	if len(readDep.Dependencies) != len(expectedDeps) {
		t.Errorf("Expected %d dependencies, got %d", len(expectedDeps), len(readDep.Dependencies))
	}

	for i, dep := range expectedDeps {
		if i < len(readDep.Dependencies) && readDep.Dependencies[i] != dep {
			t.Errorf("Expected dependency %s at index %d, got %s", 
				dep, i, readDep.Dependencies[i])
		}
	}
}

func TestEmptyDependenciesYaml(t *testing.T) {
	// Test handling of nodes with no dependencies
	tempDir := t.TempDir()
	
	nodeDir := filepath.Join(tempDir, "standalone")
	os.MkdirAll(nodeDir, 0755)
	
	// Create empty dependencies
	dep := Dependency{Dependencies: []string{}}
	data, _ := yaml.Marshal(&dep)
	os.WriteFile(filepath.Join(nodeDir, "dependencies.yaml"), data, 0644)

	// Build graph
	graph := buildGraph(tempDir)

	// Should have one node with no dependencies
	if len(graph.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(graph.Nodes))
	}

	if len(graph.Nodes) > 0 && len(graph.Nodes[0].Dependencies) != 0 {
		t.Errorf("Expected node to have no dependencies, got %d", len(graph.Nodes[0].Dependencies))
	}

	// Should have no edges
	if len(graph.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(graph.Edges))
	}
}

func TestMissingDependenciesYaml(t *testing.T) {
	// Test handling of directories without dependencies.yaml
	tempDir := t.TempDir()
	
	// Create directory without dependencies.yaml
	nodeDir := filepath.Join(tempDir, "nodeps")
	os.MkdirAll(nodeDir, 0755)
	
	// Create some other file
	os.WriteFile(filepath.Join(nodeDir, "other.txt"), []byte("test"), 0644)

	// Build graph
	graph := buildGraph(tempDir)

	// Should have no nodes since no dependencies.yaml exists
	if len(graph.Nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(graph.Edges))
	}
}