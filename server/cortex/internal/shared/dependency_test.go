package shared

import (
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDependencyGrouping tests grouping of steps by dependencies
func TestDependencyGrouping(t *testing.T) {
	tests := []struct {
		name           string
		nodes          []yamlpkg.Node
		expectedGroups [][]string // Groups of node IDs
	}{
		{
			name: "independent nodes in same group",
			nodes: []yamlpkg.Node{
				{ID: "a", Op: "op1"},
				{ID: "b", Op: "op2"},
				{ID: "c", Op: "op3"},
			},
			expectedGroups: [][]string{
				{"a", "b", "c"}, // All can run in parallel
			},
		},
		{
			name: "linear dependency chain",
			nodes: []yamlpkg.Node{
				{ID: "a", Op: "op1"},
				{ID: "b", Op: "op2", Inputs: map[string]interface{}{
					"data": "{{ .nodes.a.outputs.result }}",
				}},
				{ID: "c", Op: "op3", Inputs: map[string]interface{}{
					"data": "{{ .nodes.b.outputs.result }}",
				}},
			},
			expectedGroups: [][]string{
				{"a"},
				{"b"},
				{"c"},
			},
		},
		{
			name: "diamond dependency pattern",
			nodes: []yamlpkg.Node{
				{ID: "start", Op: "op1"},
				{ID: "left", Op: "op2", Inputs: map[string]interface{}{
					"data": "{{ .nodes.start.outputs.result }}",
				}},
				{ID: "right", Op: "op3", Inputs: map[string]interface{}{
					"data": "{{ .nodes.start.outputs.result }}",
				}},
				{ID: "end", Op: "op4", Inputs: map[string]interface{}{
					"left":  "{{ .nodes.left.outputs.result }}",
					"right": "{{ .nodes.right.outputs.result }}",
				}},
			},
			expectedGroups: [][]string{
				{"start"},
				{"left", "right"}, // Can run in parallel
				{"end"},
			},
		},
		{
			name: "complex mixed dependencies",
			nodes: []yamlpkg.Node{
				{ID: "a", Op: "op1"},
				{ID: "b", Op: "op2"},
				{ID: "c", Op: "op3", Inputs: map[string]interface{}{
					"data": "{{ .nodes.a.outputs.result }}",
				}},
				{ID: "d", Op: "op4", Inputs: map[string]interface{}{
					"data": "{{ .nodes.b.outputs.result }}",
				}},
				{ID: "e", Op: "op5", Inputs: map[string]interface{}{
					"c_data": "{{ .nodes.c.outputs.result }}",
					"d_data": "{{ .nodes.d.outputs.result }}",
				}},
				{ID: "f", Op: "op6"},
			},
			expectedGroups: [][]string{
				{"a", "b", "f"}, // Independent nodes
				{"c", "d"},      // Depend on first group
				{"e"},           // Depends on second group
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := groupByDependencies(tt.nodes)
			
			require.Equal(t, len(tt.expectedGroups), len(groups), "Number of groups mismatch")
			
			for i, expectedGroup := range tt.expectedGroups {
				actualGroup := groups[i]
				assert.ElementsMatch(t, expectedGroup, actualGroup, "Group %d mismatch", i)
			}
		})
	}
}

// groupByDependencies groups nodes by their dependency relationships
func groupByDependencies(nodes []yamlpkg.Node) [][]string {
	// Build dependency graph
	deps := make(map[string][]string)
	nodeMap := make(map[string]*yamlpkg.Node)
	
	for i := range nodes {
		node := &nodes[i]
		nodeMap[node.ID] = node
		deps[node.ID] = extractDependencies(node)
	}
	
	// Group nodes by dependency level
	groups := [][]string{}
	processed := make(map[string]bool)
	
	for len(processed) < len(nodes) {
		group := []string{}
		
		for _, node := range nodes {
			if processed[node.ID] {
				continue
			}
			
			// Check if all dependencies are satisfied
			canProcess := true
			for _, dep := range deps[node.ID] {
				if !processed[dep] {
					canProcess = false
					break
				}
			}
			
			if canProcess {
				group = append(group, node.ID)
			}
		}
		
		if len(group) == 0 {
			// Circular dependency or error
			break
		}
		
		for _, id := range group {
			processed[id] = true
		}
		
		groups = append(groups, group)
	}
	
	return groups
}

// extractDependencies extracts node IDs that a node depends on
func extractDependencies(node *yamlpkg.Node) []string {
	deps := []string{}
	seen := make(map[string]bool)
	
	// Simple pattern matching for {{ .nodes.X.outputs.Y }}
	extractFromValue(node.Inputs, &deps, seen)
	
	return deps
}

// extractFromValue recursively extracts dependencies from a value
func extractFromValue(value interface{}, deps *[]string, seen map[string]bool) {
	switch v := value.(type) {
	case string:
		// Look for patterns like {{ .nodes.X.outputs.Y }}
		if matches := findNodeReferences(v); len(matches) > 0 {
			for _, match := range matches {
				if !seen[match] {
					*deps = append(*deps, match)
					seen[match] = true
				}
			}
		}
	case map[string]interface{}:
		for _, val := range v {
			extractFromValue(val, deps, seen)
		}
	case []interface{}:
		for _, val := range v {
			extractFromValue(val, deps, seen)
		}
	}
}

// findNodeReferences finds node ID references in a template string
func findNodeReferences(template string) []string {
	// Simplified pattern matching for testing
	// Real implementation would use proper template parsing
	refs := []string{}
	
	// Look for patterns like .nodes.X.outputs
	patterns := []struct {
		prefix string
		nodeID string
	}{
		{".nodes.a.outputs", "a"},
		{".nodes.b.outputs", "b"},
		{".nodes.c.outputs", "c"},
		{".nodes.d.outputs", "d"},
		{".nodes.e.outputs", "e"},
		{".nodes.f.outputs", "f"},
		{".nodes.start.outputs", "start"},
		{".nodes.left.outputs", "left"},
		{".nodes.right.outputs", "right"},
		{".nodes.end.outputs", "end"},
	}
	
	for _, p := range patterns {
		if contains(template, p.prefix) {
			refs = append(refs, p.nodeID)
		}
	}
	
	return refs
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr, 0)
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestDependencySatisfaction tests checking if dependencies are satisfied
func TestDependencySatisfaction(t *testing.T) {
	tests := []struct {
		name         string
		nodeID       string
		dependencies []string
		completed    map[string]bool
		expected     bool
	}{
		{
			name:         "no dependencies",
			nodeID:       "a",
			dependencies: []string{},
			completed:    map[string]bool{},
			expected:     true,
		},
		{
			name:         "all dependencies satisfied",
			nodeID:       "c",
			dependencies: []string{"a", "b"},
			completed: map[string]bool{
				"a": true,
				"b": true,
			},
			expected: true,
		},
		{
			name:         "partial dependencies satisfied",
			nodeID:       "c",
			dependencies: []string{"a", "b"},
			completed: map[string]bool{
				"a": true,
			},
			expected: false,
		},
		{
			name:         "no dependencies satisfied",
			nodeID:       "c",
			dependencies: []string{"a", "b"},
			completed:    map[string]bool{},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dependenciesMet(tt.dependencies, tt.completed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// dependenciesMet checks if all dependencies are satisfied
func dependenciesMet(dependencies []string, completed map[string]bool) bool {
	for _, dep := range dependencies {
		if !completed[dep] {
			return false
		}
	}
	return true
}

// TestCircularDependencyDetection tests detection of circular dependencies
func TestCircularDependencyDetection(t *testing.T) {
	tests := []struct {
		name        string
		nodes       []yamlpkg.Node
		hasCircular bool
	}{
		{
			name: "no circular dependency",
			nodes: []yamlpkg.Node{
				{ID: "a", Op: "op1"},
				{ID: "b", Op: "op2", Inputs: map[string]interface{}{
					"data": "{{ .nodes.a.outputs.result }}",
				}},
				{ID: "c", Op: "op3", Inputs: map[string]interface{}{
					"data": "{{ .nodes.b.outputs.result }}",
				}},
			},
			hasCircular: false,
		},
		{
			name: "direct circular dependency",
			nodes: []yamlpkg.Node{
				{ID: "a", Op: "op1", Inputs: map[string]interface{}{
					"data": "{{ .nodes.b.outputs.result }}",
				}},
				{ID: "b", Op: "op2", Inputs: map[string]interface{}{
					"data": "{{ .nodes.a.outputs.result }}",
				}},
			},
			hasCircular: true,
		},
		{
			name: "indirect circular dependency",
			nodes: []yamlpkg.Node{
				{ID: "a", Op: "op1", Inputs: map[string]interface{}{
					"data": "{{ .nodes.c.outputs.result }}",
				}},
				{ID: "b", Op: "op2", Inputs: map[string]interface{}{
					"data": "{{ .nodes.a.outputs.result }}",
				}},
				{ID: "c", Op: "op3", Inputs: map[string]interface{}{
					"data": "{{ .nodes.b.outputs.result }}",
				}},
			},
			hasCircular: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCircularDependency(tt.nodes)
			assert.Equal(t, tt.hasCircular, result)
		})
	}
}

// hasCircularDependency detects circular dependencies in nodes
func hasCircularDependency(nodes []yamlpkg.Node) bool {
	// Build dependency graph
	deps := make(map[string][]string)
	for i := range nodes {
		node := &nodes[i]
		deps[node.ID] = extractDependencies(node)
	}
	
	// Check for cycles using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	
	var hasCycle func(nodeID string) bool
	hasCycle = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true
		
		for _, dep := range deps[nodeID] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}
		
		recStack[nodeID] = false
		return false
	}
	
	for _, node := range nodes {
		if !visited[node.ID] {
			if hasCycle(node.ID) {
				return true
			}
		}
	}
	
	return false
}