package ops

import "github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"

// WorkflowControlDepName re-exports the well-known dependency name
// so callers in ops can reference it without importing workflowctl directly
// if they prefer using constants.
const WorkflowControlDepName = workflowctl.DependencyName

// GetWorkflowControl retrieves the workflow control interface from the provided
// ServiceDependencies container using the well-known key. A typed helper is
// provided for convenience.
func GetWorkflowControl(deps ServiceDependencies) (workflowctl.WorkflowControl, error) {
    return workflowctl.From(deps)
}

