package setup

import (
	"reflect"
	"testing"

	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/stretchr/testify/require"
)

func TestArtifactSetFromMapIsDeterministic(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	registerArtifactCELFunctions(builder)

	env, err := cel.NewEnv(
		cel.Variable("arts", cel.DynType),
		ext.NativeTypes(reflect.TypeOf(swf.ArtifactKey{}), ext.ParseStructTag("json")),
	)
	require.NoError(t, err)

	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	keyA := swf.ArtifactKey{JobId: "j", TaskOrdinal: 1, Name: "a.txt", SizeBytes: 10}
	keyB := swf.ArtifactKey{JobId: "j", TaskOrdinal: 1, Name: "b.txt", SizeBytes: 10}

	// Intentionally insert out of order; output order should follow sorted map keys (a then b).
	arts := map[string]swf.ArtifactKey{
		"b.txt": keyB,
		"a.txt": keyA,
	}

	ast, iss := env.Compile(`artifact_names(artifact_set(arts))`)
	require.NoError(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)

	out, _, err := prg.Eval(map[string]any{"arts": arts})
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt", "b.txt"}, out.Value())
}

func TestArtifactSetAcceptsMapOfArtifacts(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	registerArtifactCELFunctions(builder)

	env, err := cel.NewEnv(
		cel.Variable("arts", cel.DynType),
		ext.NativeTypes(reflect.TypeOf(swf.ArtifactKey{}), ext.ParseStructTag("json")),
	)
	require.NoError(t, err)

	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	a := swf.NewArtifactFromBytes("a.txt", []byte("aaaa"))
	swf.AssignArtifactKey(a, swf.ArtifactKey{JobId: "j", TaskOrdinal: 1, Name: "a.txt", SizeBytes: 4})
	b := swf.NewArtifactFromBytes("b.txt", []byte("bbbbbb"))
	swf.AssignArtifactKey(b, swf.ArtifactKey{JobId: "j", TaskOrdinal: 1, Name: "b.txt", SizeBytes: 6})

	arts := map[string]swf.Artifact{
		"b.txt": b,
		"a.txt": a,
	}

	ast, iss := env.Compile(`artifact_names(artifact_set(arts))`)
	require.NoError(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)

	out, _, err := prg.Eval(map[string]any{"arts": arts})
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt", "b.txt"}, out.Value())
}

func TestArtifactConcatPreservesArgumentOrder(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	registerArtifactCELFunctions(builder)

	env, err := cel.NewEnv(
		cel.Variable("a", cel.DynType),
		cel.Variable("b", cel.DynType),
		ext.NativeTypes(reflect.TypeOf(swf.ArtifactKey{}), ext.ParseStructTag("json")),
	)
	require.NoError(t, err)
	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	a := []swf.ArtifactKey{
		{JobId: "j", TaskOrdinal: 1, Name: "first.txt", SizeBytes: 1},
	}
	b := map[string]swf.ArtifactKey{
		"second.txt": {JobId: "j", TaskOrdinal: 1, Name: "second.txt", SizeBytes: 1},
	}

	ast, iss := env.Compile(`artifact_names(artifact_concat(a, b))`)
	require.NoError(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)
	out, _, err := prg.Eval(map[string]any{"a": a, "b": b})
	require.NoError(t, err)
	require.Equal(t, []string{"first.txt", "second.txt"}, out.Value())
}

func TestArtifactFilterRegexAndSizeRange(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	registerArtifactCELFunctions(builder)

	env, err := cel.NewEnv(
		cel.Variable("arts", cel.DynType),
		ext.NativeTypes(reflect.TypeOf(swf.ArtifactKey{}), ext.ParseStructTag("json")),
	)
	require.NoError(t, err)
	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	arts := []swf.ArtifactKey{
		{JobId: "j", TaskOrdinal: 1, Name: "a.tar.gz", SizeBytes: 2048},
		{JobId: "j", TaskOrdinal: 1, Name: "b.zip", SizeBytes: 100},
		{JobId: "j", TaskOrdinal: 1, Name: "c.txt", SizeBytes: 3000},
	}

	ast, iss := env.Compile(`artifact_names(artifact_filter(arts, {"name_regex": ".*\\.(tar\\.gz|zip)$", "min_size": 1024}))`)
	require.NoError(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)

	out, _, err := prg.Eval(map[string]any{"arts": arts})
	require.NoError(t, err)
	require.Equal(t, []string{"a.tar.gz"}, out.Value())
}

func TestArtifactUniqueByNameVsKey(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	registerArtifactCELFunctions(builder)

	env, err := cel.NewEnv(
		cel.Variable("arts", cel.DynType),
		ext.NativeTypes(reflect.TypeOf(swf.ArtifactKey{}), ext.ParseStructTag("json")),
	)
	require.NoError(t, err)
	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	arts := []swf.ArtifactKey{
		{JobId: "j1", TaskOrdinal: 1, Name: "dup.txt", SizeBytes: 1},
		{JobId: "j2", TaskOrdinal: 1, Name: "dup.txt", SizeBytes: 1}, // same name, different key identity
		{JobId: "j1", TaskOrdinal: 1, Name: "uniq.txt", SizeBytes: 1},
	}

	ast, iss := env.Compile(`size(artifact_unique(arts, "name")) == 2 && size(artifact_unique(arts, "key")) == 3`)
	require.NoError(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)
	out, _, err := prg.Eval(map[string]any{"arts": arts})
	require.NoError(t, err)
	require.Equal(t, true, out.Value())
}

func TestArtifactFilterInvalidRegexErrors(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults()
	registerArtifactCELFunctions(builder)

	env, err := cel.NewEnv(
		cel.Variable("arts", cel.DynType),
		ext.NativeTypes(reflect.TypeOf(swf.ArtifactKey{}), ext.ParseStructTag("json")),
	)
	require.NoError(t, err)
	opts, err := builder.FunctionOptions(env.CELTypeAdapter())
	require.NoError(t, err)
	env, err = env.Extend(opts...)
	require.NoError(t, err)

	arts := []swf.ArtifactKey{
		{JobId: "j", TaskOrdinal: 1, Name: "a.txt", SizeBytes: 1},
	}

	ast, iss := env.Compile(`artifact_filter(arts, {"name_regex": "("})`)
	require.NoError(t, iss.Err())
	prg, err := env.Program(ast)
	require.NoError(t, err)
	_, _, err = prg.Eval(map[string]any{"arts": arts})
	require.Error(t, err)
	require.Contains(t, err.Error(), "artifact_filter: invalid name_regex")
}
