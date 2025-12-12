package cell

import (
	"context"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	"github.com/divisive-ai/vibethis/server/project/pkg/project"
)

// GraphPopulator adapts the graph builder into a cell populator.
type GraphPopulator struct {
	name    string
	builder graphBuilder
}

type graphBuilder interface {
	BuildGraph(ctx context.Context) (*core.Graph, error)
}

// NewGraphPopulator creates a populator that uses the graph builder rooted at rootPath.
// Name defaults to "graph/moon" to reflect the moon-based builder.
func NewGraphPopulator(rootPath string) *GraphPopulator {
	return &GraphPopulator{
		name:    "graph/moon",
		builder: graph.NewBuilder(rootPath),
	}
}

// Name returns the populator name.
func (p *GraphPopulator) Name() string { return p.name }

// Populate builds the graph and translates it into PopulatorCell values.
func (p *GraphPopulator) Populate(ctx context.Context, projectID project.ID) ([]PopulatorCell, error) {
	g, err := p.builder.BuildGraph(ctx)
	if err != nil {
		return nil, err
	}
	cells := make([]PopulatorCell, 0, len(g.Cells))
	for _, c := range g.Cells {
		cells = append(cells, PopulatorCell{
			Name:         c.ID,
			Description:  "",
			WorkingPath:  c.Path,
			ExternalID:   c.ID,
			Dependencies: c.Dependencies,
		})
	}
	return cells, nil
}
