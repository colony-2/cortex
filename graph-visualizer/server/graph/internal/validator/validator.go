package validator

import (
	"fmt"
	"vibethis/core/pkg/core"
)

// Validator handles dependency validation
type Validator struct{}

// New creates a new validator
func New() *Validator {
	return &Validator{}
}

// ValidateDependencies checks if the proposed dependencies would create cycles
func (v *Validator) ValidateDependencies(graph *core.Graph, nodeID string, dependencies []string) error {
	// Build adjacency list from current graph
	adjacency := make(map[string][]string)
	for _, node := range graph.Nodes {
		adjacency[node.ID] = node.Dependencies
	}

	// Create a temporary adjacency list with the proposed changes
	tempAdjacency := make(map[string][]string)
	for k, v := range adjacency {
		tempAdjacency[k] = make([]string, len(v))
		copy(tempAdjacency[k], v)
	}
	tempAdjacency[nodeID] = dependencies

	// Check for cycles using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		for _, dep := range tempAdjacency[node] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	// Check all nodes for cycles
	for nodeID := range tempAdjacency {
		if !visited[nodeID] {
			if hasCycle(nodeID) {
				return fmt.Errorf("adding these dependencies would create a circular dependency")
			}
		}
	}

	return nil
}