package funcregistry

import (
	"context"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"
)

func TestAddZeroFuncRegistersAndExecutes(t *testing.T) {
	builder := NewBuilder()
	AddZeroFunc(builder, "cells", func(ctx context.Context) ([]map[string]interface{}, error) {
		return []map[string]interface{}{
			{"name": "a", "id": "1", "path": "/a"},
			{"name": "b", "id": "2", "path": "/b"},
		}, nil
	})

	env, err := cel.NewEnv(builder.TypeOptions()...)
	require.NoError(t, err)
	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	ast, iss := env.Compile(`cells()[0].name == "a" && cells()[1].path == "/b"`)
	require.Nil(t, iss.Err())

	prg, err := env.Program(ast)
	require.NoError(t, err)
	out, _, err := prg.Eval(map[string]interface{}{})
	require.NoError(t, err)
	require.Equal(t, true, out.Value())
}
