package cells

import (
	"context"
	"errors"
	"testing"

	"github.com/colony-2/c2j/pkg/ops"
	cell "github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListCellsReturnsCells(t *testing.T) {
	ctx := context.Background()
	originalFactory := cellServiceFactory
	t.Cleanup(func() { cellServiceFactory = originalFactory })

	mockSvc := &stubCellService{
		cells: []*cell.Cell{
			{ID: "cel123", Name: "alpha", Description: "first", WorkingPath: "cells/alpha"},
			{ID: "cel456", Name: "beta", Description: "", WorkingPath: "cells/beta"},
		},
	}
	cellServiceFactory = func(db *gorm.DB) (cellLister, error) {
		require.NotNil(t, db)
		return mockSvc, nil
	}

	deps := ops.NewOpDependenciesBuilder().WithDatabase(&gorm.DB{}).Build()

	output, err := listCells(deps, ctx, ListCellsInput{})
	require.NoError(t, err)
	require.Len(t, output.Cells, 2)
	require.Equal(t, CellInfo{ID: "cel123", Name: "alpha", WorkingPath: "cells/alpha", Description: "first"}, output.Cells[0])
	require.Equal(t, CellInfo{ID: "cel456", Name: "beta", WorkingPath: "cells/beta", Description: ""}, output.Cells[1])
}

func TestListCellsRequiresDatabase(t *testing.T) {
	_, err := listCells(ops.NewOpDependenciesBuilder().Build(), context.Background(), ListCellsInput{})
	require.Error(t, err)
}

type stubCellService struct {
	cells []*cell.Cell
}

func (s *stubCellService) ListCells(ctx context.Context, filter cell.SearchFilter) (cell.Iterator[*cell.Cell], error) {
	return &stubCellIterator{cells: s.cells}, nil
}

type stubCellIterator struct {
	cells  []*cell.Cell
	index  int
	closed bool
}

func (it *stubCellIterator) Next(ctx context.Context) (*cell.Cell, error) {
	if it.closed {
		return nil, errors.New("iterator closed")
	}
	if it.index >= len(it.cells) {
		return nil, cell.ErrIteratorDone
	}
	cell := it.cells[it.index]
	it.index++
	return cell, nil
}

func (it *stubCellIterator) Close(ctx context.Context) error {
	it.closed = true
	return nil
}
