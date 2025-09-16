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
