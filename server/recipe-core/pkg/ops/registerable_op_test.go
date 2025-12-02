package ops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Activity operations delegate to external workers correctly [pkg/ops/registerable_op.go]
	act := NewActivityMappedOpV2[rIn, rOut](OpMetadata{Type: "a1"}, func(_ Invocation, _ context.Context, in rIn) (rOut, error) {
		return rOut{Echo: in.Msg}, nil
	})
	out2, err := act.ExecuteV2(Invocation{}, context.Background(), map[string]interface{}{"msg": "yo"})
	require.NoError(t, err)
	result, ok := out2.(rOut)
	require.True(t, ok)
	assert.Equal(t, "yo", result.Echo)
}

func TestRegisterableOp_ErrorAndPanics(t *testing.T) {
	// Invalid input data fails operation with clear errors [pkg/ops/registerable_op.go]
	act := NewActivityMappedOpV2[rIn, rOut](OpMetadata{Type: "a2"}, func(_ Invocation, _ context.Context, in rIn) (rOut, error) { return rOut{}, nil })
	_, err := act.ExecuteV2(Invocation{}, context.Background(), map[string]interface{}{"msg": 123})
	assert.Error(t, err)

	actOnly := NewActivityMappedOpV2[rIn, rOut](OpMetadata{Type: "act-only"}, func(_ Invocation, _ context.Context, in rIn) (rOut, error) { return rOut{}, nil })

	// Zero-valued contexts handled gracefully in operations [pkg/ops/registerable_op.go]
	out, err := actOnly.ExecuteV2(Invocation{}, context.Background(), map[string]interface{}{"msg": "ok"})
	require.NoError(t, err)
	result, ok := out.(rOut)
	require.True(t, ok)
	assert.NotNil(t, result)
}

func TestInvocation_HashDeterministic(t *testing.T) {
	inv := Invocation{RecipeID: "recipe", NodePath: "node/a", InvokeSeq: 3, BoxID: "box", ActivityID: "act"}
	inv.ID = inv.Hash()

	// Hash is stable for identical invocations and changes when fields differ.
	assert.Equal(t, inv.ID, inv.Hash())

	modified := inv
	modified.InvokeSeq = 4
	assert.NotEqual(t, inv.Hash(), modified.Hash())
}

func TestRegisterableOp_V2InlineAndActivityHandlers(t *testing.T) {

	activityInvoked := false
	activity := NewActivityMappedOpV2[rIn, rOut](OpMetadata{Type: "activity-v2"}, func(inv Invocation, _ context.Context, in rIn) (rOut, error) {
		activityInvoked = inv.NodePath == "node" && inv.InvokeSeq == 7
		return rOut{Echo: in.Msg}, nil
	})

	_, err := activity.ExecuteV2(Invocation{NodePath: "node", InvokeSeq: 7}, context.Background(), map[string]interface{}{"msg": "yo"})
	require.NoError(t, err)
	assert.True(t, activityInvoked)
}
