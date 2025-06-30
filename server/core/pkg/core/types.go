// Package core defines the core domain types and interfaces used across all vibethis modules.
package core

// Node represents a single node in the dependency graph.
type Node struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Type         string   `json:"type"`
	Dependencies []string `json:"dependencies"` // List of node IDs this node depends on
}

// Edge represents a directed edge in the dependency graph.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"` // Node ID of the source
	Target string `json:"target"` // Node ID of the target
}

// Graph represents the complete dependency graph.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Position represents the visual position of a node in the UI.
type Position struct {
	NodeID string  `json:"nodeId"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}