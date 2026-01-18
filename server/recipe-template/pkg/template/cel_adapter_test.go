package template

import (
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/require"
)

func TestClampedListReturnsFirstElement(t *testing.T) {
	list := types.NewDynamicList(types.DefaultTypeAdapter, []interface{}{1, 2, 3})
	clamped := newClampedList(list)

	val := clamped.(interface{ Get(ref.Val) ref.Val }).Get(types.Int(4))
	_, ok := val.(types.Int)
	require.True(t, ok, "expected int value")
	require.Equal(t, types.Int(1), val)
}

func TestClampNumericIndexes(t *testing.T) {
	input := "sequence.step.runs[3].outputs.items[10]"
	require.Equal(t, "sequence.step.runs[0].outputs.items[0]", clampNumericIndexes(input))
}

func TestEvaluateCELClampsRunIndexes(t *testing.T) {
	ctx, err := NewRecipeResolutionContext(
		&contextual.GitCommitContext{},
		map[string]interface{}{},
		contextual.JobContext{},
		ResolutionOptions{ClampSliceIndex: true},
	)
	require.NoError(t, err)

	ctx.TemplateData.Sequence["step"] = StepOutput{
		Outputs: map[string]interface{}{},
		Runs: []RunOutput{
			{Outputs: map[string]interface{}{"value": "ok"}},
		},
	}

	value, err := ctx.evaluateCELExpression("sequence.step.runs[3].outputs.value")
	require.NoError(t, err)
	require.Equal(t, "ok", value)
}

func TestEvaluateCELErrorsWithoutClamp(t *testing.T) {
	ctx, err := NewRecipeResolutionContext(
		&contextual.GitCommitContext{},
		map[string]interface{}{},
		contextual.JobContext{},
		ResolutionOptions{ClampSliceIndex: false},
	)
	require.NoError(t, err)

	ctx.TemplateData.Sequence["step"] = StepOutput{
		Outputs: map[string]interface{}{},
		Runs:    []RunOutput{{Outputs: map[string]interface{}{"value": "ok"}}},
	}

	_, err = ctx.evaluateCELExpression("sequence.step.runs[3].outputs.value")
	require.Error(t, err)
}
