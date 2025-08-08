// Package graph provides graph building and dependency management for vibethis.
package graph

import (
	"context"
	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/graph/internal/builder"
)

// Builder provides graph building functionality.
type Builder struct {
	rootPath string
	builder  *builder.Builder
}

// NewBuilder creates a new graph builder for the given root path.
func NewBuilder(rootPath string) *Builder {
	return &Builder{
		rootPath: rootPath,
		builder:  builder.New(rootPath),
	}
}

// BuildGraph constructs the dependency graph from the file system.
func (b *Builder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return b.builder.Build(ctx)
}

// GetCell retrieves a single cell by ID.
func (b *Builder) GetCell(ctx context.Context, cellID string) (*core.Cell, error) {
	graph, err := b.builder.Build(ctx)
	if err != nil {
		return nil, err
	}

	for _, cell := range graph.Cells {
		if cell.ID == cellID {
			return &cell, nil
		}
	}

	return nil, ErrCellNotFound
}

// ErrCellNotFound is returned when a requested cell doesn't exist.
var ErrCellNotFound = &CellNotFoundError{}

// CellNotFoundError indicates that a requested cell was not found.
type CellNotFoundError struct{}

func (e *CellNotFoundError) Error() string {
	return "cell not found"
}
