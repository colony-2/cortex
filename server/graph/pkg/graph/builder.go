// Package graph provides graph building and dependency management for vibethis.
package graph

import (
	"context"
	"vibethis/core/pkg/core"
	"vibethis/graph/internal/builder"
)

// Builder provides graph building functionality.
type Builder struct {
	rootPath  string
	builder   *builder.Builder
}

// NewBuilder creates a new graph builder for the given root path.
func NewBuilder(rootPath string) *Builder {
	return &Builder{
		rootPath:  rootPath,
		builder:   builder.New(rootPath),
	}
}

// BuildGraph constructs the dependency graph from the file system.
func (b *Builder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return b.builder.Build(ctx)
}

// GetNode retrieves a single node by ID.
func (b *Builder) GetNode(ctx context.Context, nodeID string) (*core.Node, error) {
	graph, err := b.builder.Build(ctx)
	if err != nil {
		return nil, err
	}
	
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return &node, nil
		}
	}
	
	return nil, ErrNodeNotFound
}

// ErrNodeNotFound is returned when a requested node doesn't exist.
var ErrNodeNotFound = &NodeNotFoundError{}

// NodeNotFoundError indicates that a requested node was not found.
type NodeNotFoundError struct{}

func (e *NodeNotFoundError) Error() string {
	return "node not found"
}