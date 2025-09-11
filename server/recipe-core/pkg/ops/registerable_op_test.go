package ops

import (
    "context"
    "testing"
    "time"
    "reflect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type rIn struct {
	Msg string `json:"msg"`
}
type rOut struct {
	Echo string `json:"echo"`
}

// Operation Execution
func TestRegisterableOp_Activity_And_Inline(t *testing.T) {
	// Inline operations execute synchronously within workflows [pkg/ops/registerable_op.go]
	inline := NewInlineOp[rIn, rOut](OpMetadata{Type: "i1"}, func(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in rIn) (rOut, error) {
		return rOut{Echo: in.Msg}, nil
	})
	out, err := inline.ExecuteInline(nil, time.Second, nil, map[string]interface{}{"msg": "hi"})
	require.NoError(t, err)
	assert.Equal(t, "hi", out["echo"]) // JSON-tagged structs decode from input maps correctly

	// Activity operations delegate to external workers correctly [pkg/ops/registerable_op.go]
	act := NewActivityMappedOp[rIn, rOut](OpMetadata{Type: "a1"}, func(ctx context.Context, in rIn) (rOut, error) {
		return rOut{Echo: in.Msg}, nil
	})
	out2, err := act.Execute(context.Background(), map[string]interface{}{"msg": "yo"})
	require.NoError(t, err)
	assert.Equal(t, "yo", out2["echo"]) // Output data converts to maps preserving field names
}

func TestRegisterableOp_ErrorAndPanics(t *testing.T) {
	// Invalid input data fails operation with clear errors [pkg/ops/registerable_op.go]
	act := NewActivityMappedOp[rIn, rOut](OpMetadata{Type: "a2"}, func(ctx context.Context, in rIn) (rOut, error) { return rOut{}, nil })
	_, err := act.Execute(context.Background(), map[string]interface{}{"msg": 123})
	assert.Error(t, err)

	// Missing handlers fail fast with clear panic messages [pkg/ops/registerable_op.go]
	inlineOnly := NewInlineOp[rIn, rOut](OpMetadata{Type: "inline-only"}, func(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in rIn) (rOut, error) {
		return rOut{}, nil
	})
	assert.PanicsWithValue(t, "this must be run inline, not as an activity", func() {
		_, _ = inlineOnly.Execute(context.Background(), map[string]interface{}{"msg": "x"})
	})

	actOnly := NewActivityMappedOp[rIn, rOut](OpMetadata{Type: "act-only"}, func(ctx context.Context, in rIn) (rOut, error) { return rOut{}, nil })
	assert.PanicsWithValue(t, "this must be run as an activity, not inline", func() {
		_, _ = actOnly.ExecuteInline(nil, time.Second, nil, map[string]interface{}{"msg": "x"})
	})

	// Nil contexts handled gracefully in operations [pkg/ops/registerable_op.go]
	out, err := actOnly.Execute(nil, map[string]interface{}{"msg": "ok"})
	require.NoError(t, err)
    assert.NotNil(t, out)
}

func TestRegisterableOp_GetInputType_Inline_Is_ActualInput(t *testing.T) {
    type inlineIn struct{ X string `json:"x"` }
    type inlineOut struct{ Y string `json:"y"` }

    inline := NewInlineOp[inlineIn, inlineOut](OpMetadata{Type: "inline-check"}, func(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in inlineIn) (inlineOut, error) {
        return inlineOut{Y: in.X}, nil
    })

    // Ensure GetInputType returns the typed input (not time.Duration)
    got := inline.GetInputType()
    require.Equal(t, reflect.TypeOf(inlineIn{}), got)
}
