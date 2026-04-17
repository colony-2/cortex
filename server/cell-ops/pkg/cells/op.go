package cells

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/colony-2/c2j/pkg/ops"
	cell "github.com/colony-2/colony2/server/cell/pkg/cell"
	"gorm.io/gorm"
)

// ListCellsInput has no fields; it exists to satisfy the op contract.
type ListCellsInput struct{}

// CellInfo captures the user-facing cell attributes we return.
type CellInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	WorkingPath string `json:"working_path"`
	Description string `json:"description"`
}

// ListCellsOutput is the structured response for the op.
type ListCellsOutput struct {
	Cells []CellInfo `json:"cells"`
}

type cellLister interface {
	ListCells(ctx context.Context, filter cell.SearchFilter) (cell.Iterator[*cell.Cell], error)
}

var cellServiceFactory = func(db *gorm.DB) (cellLister, error) {
	return cell.NewServiceFromDB(db)
}

// GetListOp exposes the cells.list activity.
func GetListOp() ops.RegisterableOp {
	return ops.NewActivityMappedOpV2[ListCellsInput, ListCellsOutput](
		ops.OpMetadata{
			Type:           "cells.list",
			Description:    "Lists all current cells with their IDs, names, working paths, and descriptions",
			Version:        "1.0.0",
			DefaultTimeout: time.Minute,
		},
		listCells,
	)
}

func listCells(deps ops.OpDependencies, ctx context.Context, _ ListCellsInput) (ListCellsOutput, error) {
	db := deps.Database()
	if db == nil {
		return ListCellsOutput{}, errors.New("database dependency is required")
	}

	cellSvc, err := cellServiceFactory(db)
	if err != nil {
		return ListCellsOutput{}, fmt.Errorf("create cell service: %w", err)
	}

	iter, err := cellSvc.ListCells(ctx, cell.SearchFilter{})
	if err != nil {
		return ListCellsOutput{}, fmt.Errorf("list cells: %w", err)
	}
	defer iter.Close(ctx)

	var cellsOut []CellInfo
	for {
		c, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, cell.ErrIteratorDone) {
				break
			}
			return ListCellsOutput{}, fmt.Errorf("iterate cells: %w", err)
		}
		cellsOut = append(cellsOut, CellInfo{
			ID:          string(c.ID),
			Name:        c.Name,
			WorkingPath: c.WorkingPath,
			Description: c.Description,
		})
	}

	return ListCellsOutput{Cells: cellsOut}, nil
}
