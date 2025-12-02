package ops

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/contextual"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
)

// ExecutionContext holds typed workflow context available to templates. It is created per task.
type ExecutionContext struct {
	Actor       contextual.ActorContext       `json:"actor,omitempty"`
	Environment contextual.EnvironmentContext `json:"environment,omitempty"`
	Git         gitstate.Context              `json:"git,omitempty"`
	Workflow    contextual.WorkflowEnvelope   `json:"workflow,omitempty"`
}

type TaskExecutionContext struct {
	ExecutionContext
	ops.Invocation
}
