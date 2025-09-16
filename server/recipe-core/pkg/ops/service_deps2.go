package ops

import (
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
)

// ServiceDependencies2 is a backwards-compatible extension of ServiceDependencies
// that exposes a typed accessor for workflow control. This allows callers to pass
// an implementation of ServiceDependencies2 into Initialize without changing the
// existing Initialize signature. Receivers can then type-assert to this interface
// and use WorkflowControl() when available.
//
// Migration plan:
// - Phase 1: Introduce ServiceDependencies2 (this type) and optional wrappers.
//            No signature changes; code continues to accept ServiceDependencies.
// - Phase 2: Call sites begin passing a value that implements ServiceDependencies2.
//            Callees optionally type-assert deps.(ServiceDependencies2).
// - Phase 3: Once broadly adopted, consider updating Initialize signatures to
//            depend on ServiceDependencies2 directly.
type ServiceDependencies2 interface {
    ServiceDependencies
    // WorkflowControl returns a typed workflow controller if available.
    // ok will be false if the dependency is not present.
    WorkflowControl() (ctl workflowctl.WorkflowControl, ok bool)
}

// serviceDepsWrapper is a small adapter that implements ServiceDependencies2
// by delegating Get to an underlying ServiceDependencies and optionally
// providing a typed WorkflowControl. If not explicitly set, it attempts to
// resolve via the well-known key using workflowctl.From(Base).
type serviceDepsWrapper struct {
    Base ServiceDependencies
    Ctl  workflowctl.WorkflowControl
}

func (w serviceDepsWrapper) Get(name string) (interface{}, error) {
    return w.Base.Get(name)
}

// WorkflowControl implements ServiceDependencies2.
func (w serviceDepsWrapper) WorkflowControl() (workflowctl.WorkflowControl, bool) {
    if w.Ctl != nil {
        return w.Ctl, true
    }
    if w.Base == nil {
        return nil, false
    }
    // Best-effort discovery via dependency map
    if ctl, err := workflowctl.From(w.Base); err == nil && ctl != nil {
        return ctl, true
    }
    return nil, false
}

// WithWorkflowControl wraps an existing ServiceDependencies with a typed
// WorkflowControl, returning a value that implements both ServiceDependencies
// and ServiceDependencies2. This can be passed anywhere a ServiceDependencies
// is expected today, while also enabling type assertions for the new method.
func WithWorkflowControl(base ServiceDependencies, ctl workflowctl.WorkflowControl) ServiceDependencies {
    return serviceDepsWrapper{Base: base, Ctl: ctl}
}

