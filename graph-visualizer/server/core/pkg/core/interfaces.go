package core

import "context"

// Storage defines the interface for persistent storage operations.
type Storage interface {
	// Position operations
	SavePosition(ctx context.Context, pos Position) error
	GetPositions(ctx context.Context) ([]Position, error)
	DeletePosition(ctx context.Context, nodeID string) error
	
	// Container ID operations
	SaveContainerID(ctx context.Context, nodeID, containerID string) error
	GetContainerID(ctx context.Context, nodeID string) (string, error)
	DeleteContainerID(ctx context.Context, nodeID string) error
	
	// Lifecycle
	Close() error
}

// GraphBuilder defines the interface for building and managing the dependency graph.
type GraphBuilder interface {
	// BuildGraph constructs the dependency graph from the file system.
	BuildGraph(ctx context.Context) (*Graph, error)
	
	// UpdateNodeDependencies updates the dependencies for a specific node.
	UpdateNodeDependencies(ctx context.Context, nodeID string, dependencies []string) error
	
	// ValidateDependencies checks if the proposed dependencies would create cycles.
	ValidateDependencies(ctx context.Context, nodeID string, dependencies []string) error
	
	// GetNode retrieves a single node by ID.
	GetNode(ctx context.Context, nodeID string) (*Node, error)
}