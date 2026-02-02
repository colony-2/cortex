package setup

import (
	"context"
	"testing"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"
)

// This unit test exercises the CEL builder wiring for the new cells() function
// without spinning up the full server. It uses an in-memory slice iterator to
// simulate the cell service.
func TestCellsFunctionReturnsExpectedShape(t *testing.T) {
	items := []*cell.Cell{
		{Name: "alpha", ID: cell.ID("1"), WorkingPath: "/a"},
		{Name: "beta", ID: cell.ID("2"), WorkingPath: "/b"},
	}

	builder := funcregistry.NewBuilder().WithDefaults()
	funcregistry.AddZeroFunc(builder, "cells", func(ctx context.Context) ([]map[string]interface{}, error) {
		// use the items slice directly
		out := make([]map[string]interface{}, 0, len(items))
		for _, c := range items {
			out = append(out, map[string]interface{}{
				"name": c.Name,
				"id":   string(c.ID),
				"path": c.WorkingPath,
			})
		}
		return out, nil
	})

	env, err := cel.NewEnv(builder.TypeOptions()...)
	require.NoError(t, err)
	fnOpts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(fnOpts...)
	require.NoError(t, err)

	ast, iss := env.Compile(`size(cells()) == 2 && cells()[0].name == "alpha" && cells()[1].path == "/b"`)
	require.Nil(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)
	out, _, err := prg.Eval(map[string]interface{}{})
	require.NoError(t, err)
	require.Equal(t, true, out.Value())
}
