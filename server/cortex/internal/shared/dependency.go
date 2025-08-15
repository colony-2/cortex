package shared

import (
	"regexp"
	"strings"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// DependencyManager manages node dependencies and execution order
type DependencyManager struct {
	nodeReferencePattern *regexp.Regexp
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager() *DependencyManager {
	return &DependencyManager{
		// Pattern to match {{ .nodes.X.outputs.Y }} references
		nodeReferencePattern: regexp.MustCompile(`\{\{\s*\.nodes\.([^.\s]+)\.outputs[^}]*\}\}`),
	}
}

// GroupByDependencies groups nodes by their dependency relationships for parallel execution
func (dm *DependencyManager) GroupByDependencies(nodes []yamlpkg.Node) [][]yamlpkg.Node {
	// Build dependency graph
	dependencies := make(map[string][]string)
	nodeMap := make(map[string]*yamlpkg.Node)
	
	for i := range nodes {
		node := &nodes[i]
		nodeMap[node.ID] = node
		dependencies[node.ID] = dm.ExtractDependencies(node)
	}
	
	// Group nodes by dependency level
	groups := [][]yamlpkg.Node{}
	processed := make(map[string]bool)
	
	for len(processed) < len(nodes) {
		group := []yamlpkg.Node{}
		
		for _, node := range nodes {
			if processed[node.ID] {
				continue
			}
			
			// Check if all dependencies are satisfied
			if dm.DependenciesMet(dependencies[node.ID], processed) {
				group = append(group, node)
			}
		}
		
		if len(group) == 0 {
			// Circular dependency or error - add remaining nodes
			for _, node := range nodes {
				if !processed[node.ID] {
					group = append(group, node)
				}
			}
			if len(group) > 0 {
				groups = append(groups, group)
			}
			break
		}
		
		// Mark nodes as processed
		for _, node := range group {
			processed[node.ID] = true
		}
		
		groups = append(groups, group)
	}
	
	return groups
}

// ExtractDependencies extracts node IDs that a node depends on
func (dm *DependencyManager) ExtractDependencies(node *yamlpkg.Node) []string {
	deps := []string{}
	seen := make(map[string]bool)
	
	// Extract from inputs
	dm.extractFromValue(node.Inputs, &deps, seen)
	
	// Extract from outputs (in case they reference other nodes)
	dm.extractFromValue(node.Outputs, &deps, seen)
	
	// Extract from conditional expressions
	if node.When != "" {
		dm.extractFromString(node.When, &deps, seen)
	}
	
	return deps
}

// DependenciesMet checks if all dependencies are satisfied
func (dm *DependencyManager) DependenciesMet(dependencies []string, completed map[string]bool) bool {
	for _, dep := range dependencies {
		if !completed[dep] {
			return false
		}
	}
	return true
}

// HasCircularDependency detects circular dependencies in nodes
func (dm *DependencyManager) HasCircularDependency(nodes []yamlpkg.Node) bool {
	// Build dependency graph
	deps := make(map[string][]string)
	for i := range nodes {
		node := &nodes[i]
		deps[node.ID] = dm.ExtractDependencies(node)
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

// extractFromValue recursively extracts dependencies from a value
func (dm *DependencyManager) extractFromValue(value interface{}, deps *[]string, seen map[string]bool) {
	switch v := value.(type) {
	case string:
		dm.extractFromString(v, deps, seen)
	case map[string]interface{}:
		for _, val := range v {
			dm.extractFromValue(val, deps, seen)
		}
	case []interface{}:
		for _, val := range v {
			dm.extractFromValue(val, deps, seen)
		}
	}
}

// extractFromString extracts node references from a string
func (dm *DependencyManager) extractFromString(s string, deps *[]string, seen map[string]bool) {
	matches := dm.nodeReferencePattern.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		if len(match) > 1 {
			nodeID := match[1]
			if !seen[nodeID] {
				*deps = append(*deps, nodeID)
				seen[nodeID] = true
			}
		}
	}
}

// GetExecutionOrder returns a linear execution order respecting dependencies
func (dm *DependencyManager) GetExecutionOrder(nodes []yamlpkg.Node) ([]string, error) {
	// Use topological sort
	dependencies := make(map[string][]string)
	inDegree := make(map[string]int)
	
	// Initialize
	for i := range nodes {
		node := &nodes[i]
		dependencies[node.ID] = dm.ExtractDependencies(node)
		inDegree[node.ID] = 0
	}
	
	// Calculate in-degrees
	for _, deps := range dependencies {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}
	
	// Find nodes with no dependencies
	queue := []string{}
	for _, node := range nodes {
		if inDegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	
	// Process queue
	order := []string{}
	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)
		
		// Update in-degrees
		for nodeID, deps := range dependencies {
			for _, dep := range deps {
				if dep == current {
					inDegree[nodeID]--
					if inDegree[nodeID] == 0 {
						queue = append(queue, nodeID)
					}
				}
			}
		}
	}
	
	// Check if all nodes were processed
	if len(order) != len(nodes) {
		// Circular dependency exists
		return nil, nil
	}
	
	return order, nil
}

// ValidateDependencies validates that all referenced nodes exist
func (dm *DependencyManager) ValidateDependencies(nodes []yamlpkg.Node) []string {
	errors := []string{}
	nodeIDs := make(map[string]bool)
	
	// Collect all node IDs
	for _, node := range nodes {
		nodeIDs[node.ID] = true
	}
	
	// Check each node's dependencies
	for _, node := range nodes {
		deps := dm.ExtractDependencies(&node)
		for _, dep := range deps {
			if !nodeIDs[dep] {
				errors = append(errors, "Node '"+node.ID+"' references non-existent node '"+dep+"'")
			}
		}
	}
	
	return errors
}

// OptimizeExecutionGroups optimizes parallel execution groups based on dependencies
func (dm *DependencyManager) OptimizeExecutionGroups(nodes []yamlpkg.Node) [][]yamlpkg.Node {
	groups := dm.GroupByDependencies(nodes)
	
	// Further optimize by considering resource constraints
	// For now, just return the dependency-based groups
	// Future enhancements could consider:
	// - Node execution time estimates
	// - Resource requirements
	// - Priority levels
	
	return groups
}

// GetDependencyGraph returns a representation of the dependency graph
func (dm *DependencyManager) GetDependencyGraph(nodes []yamlpkg.Node) map[string][]string {
	graph := make(map[string][]string)
	
	for i := range nodes {
		node := &nodes[i]
		graph[node.ID] = dm.ExtractDependencies(node)
	}
	
	return graph
}

// FindIndependentNodes finds nodes that have no dependencies
func (dm *DependencyManager) FindIndependentNodes(nodes []yamlpkg.Node) []string {
	independent := []string{}
	
	for i := range nodes {
		node := &nodes[i]
		deps := dm.ExtractDependencies(node)
		if len(deps) == 0 {
			independent = append(independent, node.ID)
		}
	}
	
	return independent
}

// FindTerminalNodes finds nodes that no other nodes depend on
func (dm *DependencyManager) FindTerminalNodes(nodes []yamlpkg.Node) []string {
	// Build reverse dependency map
	dependedOn := make(map[string]bool)
	
	for i := range nodes {
		node := &nodes[i]
		deps := dm.ExtractDependencies(node)
		for _, dep := range deps {
			dependedOn[dep] = true
		}
	}
	
	// Find nodes not depended on
	terminal := []string{}
	for _, node := range nodes {
		if !dependedOn[node.ID] {
			terminal = append(terminal, node.ID)
		}
	}
	
	return terminal
}

// ResolveTemplateReferences resolves template references in node inputs/outputs
func (dm *DependencyManager) ResolveTemplateReferences(template string, nodeOutputs map[string]map[string]interface{}) string {
	result := template
	
	// Find all node references
	matches := dm.nodeReferencePattern.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		if len(match) > 1 {
			nodeID := match[1]
			if outputs, ok := nodeOutputs[nodeID]; ok {
				// Simple replacement for testing
				// Real implementation would use proper template engine
				for key, value := range outputs {
					oldPattern := "{{ .nodes." + nodeID + ".outputs." + key + " }}"
					newValue := toString(value)
					result = strings.ReplaceAll(result, oldPattern, newValue)
				}
			}
		}
	}
	
	return result
}

// toString converts a value to string
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int32, int64:
		return string(rune(val.(int)))
	case float32, float64:
		return string(rune(int(val.(float64))))
	default:
		return ""
	}
}