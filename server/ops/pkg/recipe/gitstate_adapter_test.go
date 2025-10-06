package recipe

import (
	"testing"

	ops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/workflow"
)

func TestGitStateAdapterNotRegistered(t *testing.T) {
	ResetGitStateAdapter()
	t.Cleanup(func() {
		RegisterGitStateAdapter(testGitStateAdapter{})
	})

	_, _, err := planDetachedWorkspace(ops.Invocation{}, nil, DetachedWorkspaceOptions{})
	require.ErrorIs(t, err, ErrGitStateAdapterNotRegistered)

	_, err = withInlineWorkspace(nil, ops.Invocation{}, nil, InlineWorkspaceOptions{}, func(workflow.Context, map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{}, nil
	})
	require.ErrorIs(t, err, ErrGitStateAdapterNotRegistered)
}

func TestRegisterGitStateAdapter(t *testing.T) {
	t.Cleanup(func() {
		RegisterGitStateAdapter(testGitStateAdapter{})
	})

	adapter := &adapterSpy{}
	RegisterGitStateAdapter(adapter)

	ctx, childInputs, err := planDetachedWorkspace(ops.Invocation{}, map[string]interface{}{"context": map[string]interface{}{}}, DetachedWorkspaceOptions{})
	require.NoError(t, err)
	require.NotNil(t, ctx)
	require.NotNil(t, childInputs)
	require.True(t, adapter.planCalled)

	result, err := withInlineWorkspace(nil, ops.Invocation{}, map[string]interface{}{}, InlineWorkspaceOptions{}, func(workflow.Context, map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	})
	require.NoError(t, err)
	require.True(t, adapter.inlineCalled)
	require.Equal(t, true, result.Result["ok"])
	require.NotNil(t, result.GitContext)
	require.Equal(t, "hash", result.GitContext.GetPersistHash())
}

type adapterSpy struct {
	inlineCalled bool
	planCalled   bool
}

func (a *adapterSpy) WithInlineWorkspace(ctx workflow.Context, inv ops.Invocation, inputs map[string]interface{}, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc) (*InlineWorkspaceResult, error) {
	a.inlineCalled = true
	if fn == nil {
		return nil, ErrGitStateAdapterNotRegistered
	}
	output, err := fn(ctx, map[string]interface{}{"context": map[string]interface{}{}})
	if err != nil {
		return nil, err
	}
	return &InlineWorkspaceResult{
		Result:     output,
		ContextMap: map[string]interface{}{"git": map[string]interface{}{}},
		GitContext: adapterContext{},
	}, nil
}

func (a *adapterSpy) PlanDetachedWorkspace(inv ops.Invocation, inputs map[string]interface{}, opts DetachedWorkspaceOptions) (GitStateContext, map[string]interface{}, error) {
	a.planCalled = true
	if inputs == nil {
		inputs = make(map[string]interface{})
	}
	if _, ok := inputs["context"]; !ok {
		inputs["context"] = map[string]interface{}{}
	}
	return adapterContext{}, inputs, nil
}

type adapterContext struct{}

func (adapterContext) ToMap() map[string]interface{} { return map[string]interface{}{} }

func (adapterContext) GetPersistHash() string { return "hash" }

func (adapterContext) GetWorktreePath() string { return "" }

func (adapterContext) GetBlobStoreURI() string { return "" }

func (adapterContext) GetTicketID() string { return "" }

func (adapterContext) GetCellName() string { return "" }
