package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
	"vibethis/core/pkg/core"
)

// Use the Dependency type from the builder package

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

	// Verify each node exists
	for nodeName := range nodes {
		if _, exists := nodeMap[nodeName]; !exists {
			t.Errorf("Expected node %s not found in graph", nodeName)
		}
	}

	// Verify dependencies
	testCases := []struct {
		nodeID   string
		expected []string
	}{
		{"api", []string{"database", "cache"}},
		{"frontend", []string{"api"}},
		{"database", []string{}},
		{"cache", []string{}},
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
		{"api", "database"},
		{"api", "cache"},
		{"frontend", "api"},
	}

	for _, ee := range expectedEdgeList {
		if !edgeExists(ee.source, ee.target) {
			t.Errorf("Expected edge from %s to %s not found", ee.source, ee.target)
		}
	}
}

func TestEmptyDependenciesYaml(t *testing.T) {
	tempDir := t.TempDir()

	// Create a node with empty dependencies
	nodeDir := filepath.Join(tempDir, "empty-node")
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		t.Fatalf("Failed to create node directory: %v", err)
	}

	dep := Dependency{Dependencies: []string{}}
	data, err := yaml.Marshal(&dep)
	if err != nil {
		t.Fatalf("Failed to marshal dependencies: %v", err)
	}

	depFile := filepath.Join(nodeDir, "dependencies.yaml")
	if err := os.WriteFile(depFile, data, 0644); err != nil {
		t.Fatalf("Failed to write dependencies file: %v", err)
	}

	// Build the graph
	builder := New(tempDir)
	graph, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	// Should have one node with no dependencies
	if len(graph.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(graph.Edges))
	}

	if len(graph.Nodes) > 0 && len(graph.Nodes[0].Dependencies) != 0 {
		t.Errorf("Expected 0 dependencies, got %d", len(graph.Nodes[0].Dependencies))
	}
}

func TestMissingDependenciesYaml(t *testing.T) {
	tempDir := t.TempDir()

	// Create a node without dependencies.yaml
	nodeDir := filepath.Join(tempDir, "no-deps-node")
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		t.Fatalf("Failed to create node directory: %v", err)
	}

	// Build the graph
	builder := New(tempDir)
	graph, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	// Should have one node with no dependencies
	if len(graph.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(graph.Edges))
	}

	if len(graph.Nodes) > 0 && len(graph.Nodes[0].Dependencies) != 0 {
		t.Errorf("Expected 0 dependencies, got %d", len(graph.Nodes[0].Dependencies))
	}
}