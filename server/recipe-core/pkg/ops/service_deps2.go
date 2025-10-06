package ops

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
)

// ServiceDependencies2 defines the typed dependencies exposed to management services
// and ops that require access to shared runtime collaborators.
type ServiceDependencies2 interface {
	// WorkflowControl returns a typed workflow controller if available.
	WorkflowControl() (workflowctl.WorkflowControl, bool)

	// SSEManager returns the server-sent-event manager when supported.
	SSEManager() (SSEManager, bool)

	// TemporalNamespace returns the Temporal namespace associated with the environment.
	TemporalNamespace() (string, bool)
}
