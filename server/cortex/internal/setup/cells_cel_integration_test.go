package setup

import (
	"context"
	"fmt"
	"testing"

	"github.com/colony-2/c2j/pkg/contextual"
	"github.com/colony-2/c2j/pkg/template/funcregistry"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"
)

// This unit test exercises the CEL builder wiring for the new cells() function
// without spinning up the full server. It uses an in-memory slice iterator to
// simulate the cell service.
func TestCellsFunctionReturnsExpectedShape(t *testing.T) {
	items := []*cell.Cell{
		{Name: "alpha", ID: cell.ID("1"), WorkingPath: "/a", ProjectID: project.ID("proj-A")},
		{Name: "beta", ID: cell.ID("2"), WorkingPath: "/b", ProjectID: project.ID("proj-B")},
	}

	builder := funcregistry.NewBuilder().WithDefaults()
	funcregistry.AddZeroFuncWithContext(builder, "cells", func(ctx context.Context, taskCtx contextual.TaskExecutionContext) ([]funcregistry.CELCell, error) {
		projectID := project.ID(taskCtx.Workflow.ProjectId)
		out := []funcregistry.CELCell{}
		for _, c := range items {
			if c.ProjectID != projectID {
				continue
			}
			out = append(out, funcregistry.CELCell{
				Name: c.Name,
				ID:   string(c.ID),
				Path: c.WorkingPath,
			})
		}
		return out, nil
	})

	env, err := cel.NewEnv(builder.TypeOptions()...)
	require.NoError(t, err)
	fnOpts, err := builder.FunctionOptionsWithContext(env.CELTypeAdapter(), func() contextual.TaskExecutionContext {
		return contextual.TaskExecutionContext{
			Workflow: contextual.WorkflowContext{ProjectId: "proj-A"},
		}
	})
	require.NoError(t, err)
	env, err = env.Extend(fnOpts...)
	require.NoError(t, err)

	ast, iss := env.Compile(`size(cells()) == 1 && cells()[0].name == "alpha" && cells()[0].path == "/a"`)
	require.Nil(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)
	out, _, err := prg.Eval(map[string]interface{}{})
	require.NoError(t, err)
	require.Equal(t, true, out.Value())
}

func TestCellsFunctionRequiresProjectID(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	funcregistry.AddZeroFuncWithContext(builder, "cells", func(ctx context.Context, taskCtx contextual.TaskExecutionContext) ([]funcregistry.CELCell, error) {
		if taskCtx.Workflow.ProjectId == "" {
			return nil, fmt.Errorf("cells: project_id is required in context.workflow.project_id")
		}
		return nil, nil
	})

	env, err := cel.NewEnv(builder.TypeOptions()...)
	require.NoError(t, err)

	opts, err := builder.FunctionOptionsWithContext(env.CELTypeAdapter(), func() contextual.TaskExecutionContext {
		return contextual.TaskExecutionContext{}
	})
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	ast, iss := env.Compile(`cells()`)
	require.Nil(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)
	_, _, err = prg.Eval(map[string]interface{}{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cells: project_id is required in context.workflow.project_id")
}
