package ops

import (
    "fmt"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
)

// Note: A string key for workflow control is no longer used.

// GetWorkflowControl retrieves the workflow control interface from the provided
// ServiceDependencies container using the well-known key. A typed helper is
// provided for convenience.
func GetWorkflowControl(deps ServiceDependencies2) (workflowctl.WorkflowControl, error) {
    if ctl, ok := deps.WorkflowControl(); ok && ctl != nil {
        return ctl, nil
    }
    return nil, fmt.Errorf("workflow control not available")
}
