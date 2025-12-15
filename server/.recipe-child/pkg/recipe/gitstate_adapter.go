package recipe

import (
	"errors"
	"sync"

	ops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"go.temporal.io/sdk/workflow"
)

var (
	gitStateAdapterMu sync.RWMutex
	gitStateAdapter   GitStateAdapter = noopGitStateAdapter{}

	// ErrGitStateAdapterNotRegistered is returned when no git workspace adapter has been configured.
	ErrGitStateAdapterNotRegistered = errors.New("recipe: git state adapter not registered")
)

// GitStateAdapter encapsulates git workspace orchestration behaviours required by the recipe op.
type GitStateAdapter interface {
	WithInlineWorkspace(ctx workflow.Context, inv ops.Invocation, inputs map[string]interface{}, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc) (*InlineWorkspaceResult, error)
	PlanDetachedWorkspace(inv ops.Invocation, inputs map[string]interface{}, opts DetachedWorkspaceOptions) (GitStateContext, map[string]interface{}, error)
}

// InlineWorkspaceFunc represents the inline logic executed within a managed git workspace lifecycle.
type InlineWorkspaceFunc func(workflow.Context, map[string]interface{}) (map[string]interface{}, error)

// GitStateContext exposes the git metadata required by the recipe op without depending on recipe-worker types.
type GitStateContext interface {
	ToMap() map[string]interface{}
	GetPersistHash() string
	GetWorktreePath() string
	GetBlobStoreURI() string
	GetTicketID() string
	GetCellName() string
}

// InlineWorkspaceOptions customises inline git workspace execution.
type InlineWorkspaceOptions struct {
	ChildID      string
	SkipFinalize bool
}

// InlineWorkspaceResult captures the outputs from inline git workspace execution.
type InlineWorkspaceResult struct {
	Result     map[string]interface{}
	ContextMap map[string]interface{}
	GitContext GitStateContext
}

// DetachedWorkspaceOptions customises detached git workspace derivation.
type DetachedWorkspaceOptions struct {
	ChildID string
}

// RegisterGitStateAdapter configures the git workspace adapter used by the recipe op.
func RegisterGitStateAdapter(adapter GitStateAdapter) {
	gitStateAdapterMu.Lock()
	defer gitStateAdapterMu.Unlock()

	if adapter == nil {
		gitStateAdapter = noopGitStateAdapter{}
		return
	}
	gitStateAdapter = adapter
}

// ResetGitStateAdapter clears the configured git workspace adapter. Primarily used by tests.
func ResetGitStateAdapter() {
	RegisterGitStateAdapter(nil)
}

func withInlineWorkspace(ctx workflow.Context, inv ops.Invocation, inputs map[string]interface{}, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc) (*InlineWorkspaceResult, error) {
	gitStateAdapterMu.RLock()
	adapter := gitStateAdapter
	gitStateAdapterMu.RUnlock()
	return adapter.WithInlineWorkspace(ctx, inv, inputs, opts, fn)
}

func planDetachedWorkspace(inv ops.Invocation, inputs map[string]interface{}, opts DetachedWorkspaceOptions) (GitStateContext, map[string]interface{}, error) {
	gitStateAdapterMu.RLock()
	adapter := gitStateAdapter
	gitStateAdapterMu.RUnlock()
	return adapter.PlanDetachedWorkspace(inv, inputs, opts)
}

type noopGitStateAdapter struct{}

func (noopGitStateAdapter) WithInlineWorkspace(workflow.Context, ops.Invocation, map[string]interface{}, InlineWorkspaceOptions, InlineWorkspaceFunc) (*InlineWorkspaceResult, error) {
	return nil, ErrGitStateAdapterNotRegistered
}

func (noopGitStateAdapter) PlanDetachedWorkspace(ops.Invocation, map[string]interface{}, DetachedWorkspaceOptions) (GitStateContext, map[string]interface{}, error) {
	return nil, nil, ErrGitStateAdapterNotRegistered
}
