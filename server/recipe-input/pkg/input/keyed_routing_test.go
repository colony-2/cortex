package input

import (
	"fmt"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

type inlineExecution struct {
	Invocation ops.Invocation
	Input      map[string]interface{}
}

type inlineResult struct {
	Index  int
	Output map[string]interface{}
	Err    error
}

func singleInlineWorkflow(ctx workflow.Context, params inlineExecution) (map[string]interface{}, error) {
	op := GetOp()
	return op.ExecuteInlineV2(params.Invocation, ctx, time.Minute, nil, params.Input)
}

func concurrentInlineWorkflow(ctx workflow.Context, params []inlineExecution) ([]map[string]interface{}, error) {
	op := GetOp()
	results := make([]map[string]interface{}, len(params))
	resultCh := workflow.NewBufferedChannel(ctx, len(params))

	for i, p := range params {
		idx := i
		param := p
		workflow.Go(ctx, func(ctx workflow.Context) {
			out, err := op.ExecuteInlineV2(param.Invocation, ctx, time.Minute, nil, param.Input)
			resultCh.Send(ctx, inlineResult{Index: idx, Output: out, Err: err})
		})
	}

	var firstErr error
	for i := 0; i < len(params); i++ {
		var res inlineResult
		resultCh.Receive(ctx, &res)
		if res.Err != nil && firstErr == nil {
			firstErr = res.Err
		}
		results[res.Index] = res.Output
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func TestInputOp_WaitsOnKeyedSignal(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	inv := ops.Invocation{
		RecipeID:   "recipe",
		NodePath:   "path",
		InvokeSeq:  0,
		BoxID:      "box",
		ActivityID: "activity",
	}
	id := inv.Hash()

	inputMap := map[string]interface{}{
		"box_id":      inv.BoxID,
		"activity_id": inv.ActivityID,
		"config": map[string]interface{}{
			"title":   "Need approval",
			"timeout": 60,
			"fields":  []interface{}{},
		},
	}

	env.RegisterWorkflow(singleInlineWorkflow)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("user-response", UserResponseSignal{UserID: "wrong"})
	}, 10*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(fmt.Sprintf("user-response:%s", id), UserResponseSignal{
			Fields: map[string]interface{}{"approval": "yes"},
			UserID: "correct",
		})
	}, 20*time.Millisecond)

	env.ExecuteWorkflow(singleInlineWorkflow, inlineExecution{Invocation: inv, Input: inputMap})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "correct", result["user_id"])

	fields, ok := result["fields"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "yes", fields["approval"])
}

func TestInputOp_ConcurrentKeyedSignals(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	inv1 := ops.Invocation{RecipeID: "recipe", NodePath: "path", InvokeSeq: 0, BoxID: "box", ActivityID: "activity-a"}
	inv2 := ops.Invocation{RecipeID: "recipe", NodePath: "path", InvokeSeq: 1, BoxID: "box", ActivityID: "activity-b"}
	id1 := inv1.Hash()
	id2 := inv2.Hash()
	require.NotEqual(t, id1, id2)

	inputMap := func(activity string) map[string]interface{} {
		return map[string]interface{}{
			"box_id":      "box",
			"activity_id": activity,
			"config": map[string]interface{}{
				"title":   "Parallel input",
				"timeout": 120,
				"fields":  []interface{}{},
			},
		}
	}

	env.RegisterWorkflow(concurrentInlineWorkflow)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(fmt.Sprintf("user-response:%s", id1), UserResponseSignal{
			Fields: map[string]interface{}{"idx": 1},
			UserID: "user-1",
		})
	}, 10*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(fmt.Sprintf("user-response:%s", id2), UserResponseSignal{
			Fields: map[string]interface{}{"idx": 2},
			UserID: "user-2",
		})
	}, 15*time.Millisecond)

	env.ExecuteWorkflow(concurrentInlineWorkflow, []inlineExecution{
		{Invocation: inv1, Input: inputMap(inv1.ActivityID)},
		{Invocation: inv2, Input: inputMap(inv2.ActivityID)},
	})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var results []map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&results))
	require.Len(t, results, 2)

	assert.Equal(t, "user-1", results[0]["user_id"])
	assert.Equal(t, "user-2", results[1]["user_id"])

	fields1, ok := results[0]["fields"].(map[string]interface{})
	require.True(t, ok)
	fields2, ok := results[1]["fields"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(1), fields1["idx"])
	assert.Equal(t, float64(2), fields2["idx"])
}
