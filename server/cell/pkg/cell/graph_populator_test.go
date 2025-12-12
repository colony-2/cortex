package cell

import (
	"context"
	"testing"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
)

type fakeGraphBuilder struct {
	graph *core.Graph
	err   error
}

func (f fakeGraphBuilder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return f.graph, f.err
}

func TestGraphPopulatorTranslatesCells(t *testing.T) {
	gb := &fakeGraphBuilder{
		graph: &core.Graph{
			Cells: []core.Cell{
				{ID: "cell-a", Name: "cell-a", Path: "/repo/a", Dependencies: []string{"cell-b"}},
				{ID: "cell-b", Name: "cell-b", Path: "/repo/b", Dependencies: nil},
			},
		},
	}
	pop := &GraphPopulator{name: "graph/moon", builder: gb}

	result, err := pop.Populate(context.Background(), "")
	if err != nil {
		t.Fatalf("populate: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(result))
	}
	gotA := result[0]
	if gotA.Name != "cell-a" || gotA.ExternalID != "cell-a" || gotA.WorkingPath != "/repo/a" {
		t.Fatalf("unexpected cell-a: %+v", gotA)
	}
	if len(gotA.Dependencies) != 1 || gotA.Dependencies[0] != "cell-b" {
		t.Fatalf("unexpected deps for cell-a: %+v", gotA.Dependencies)
	}
}
