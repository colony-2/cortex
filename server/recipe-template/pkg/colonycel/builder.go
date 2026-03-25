package colonycel

import (
	"context"
	"fmt"
	"strings"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
)

type Options struct {
	CellsService cell.Service
}

func NewBuilder(opts Options) *funcregistry.Builder {
	builder := funcregistry.NewBuilder().WithDefaults()
	RegisterArtifactFunctions(builder)
	registerCellsFunction(builder, opts.CellsService)
	return builder
}

func registerCellsFunction(builder *funcregistry.Builder, svc cell.Service) {
	if builder == nil || svc == nil {
		return
	}

	funcregistry.AddZeroFuncWithContext(builder, "cells", func(ctx context.Context, taskCtx contextual.TaskExecutionContext) ([]funcregistry.CELCell, error) {
		projectID := strings.TrimSpace(taskCtx.Workflow.ProjectId)
		if projectID == "" {
			return nil, fmt.Errorf("cells: project_id is required in context.workflow.project_id")
		}

		var (
			cached    []funcregistry.CELCell
			cachedErr error
			fetched   bool
		)

		fetch := func() ([]funcregistry.CELCell, error) {
			if fetched {
				return cached, cachedErr
			}
			fetched = true

			it, err := svc.ListCells(ctx, cell.SearchFilter{ProjectIDs: []project.ID{project.ID(projectID)}})
			if err != nil {
				cachedErr = fmt.Errorf("cells: failed to list cells: %w", err)
				return nil, cachedErr
			}
			defer it.Close(ctx)

			for {
				c, err := it.Next(ctx)
				if err == cell.ErrIteratorDone {
					break
				}
				if err != nil {
					cachedErr = fmt.Errorf("cells: failed to list cells: %w", err)
					return nil, cachedErr
				}
				cached = append(cached, funcregistry.CELCell{
					Name:        c.Name,
					ID:          string(c.ID),
					Path:        c.WorkingPath,
					Description: c.Description,
				})
			}
			return cached, nil
		}

		return fetch()
	})
}
