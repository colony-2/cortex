package recipe

import (
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/runmetadata"
	"go.temporal.io/sdk/workflow"
)

const metadataSignalName = "recipe_run_metadata"

// SignalChildRunMetadata delivers the standard run metadata signal to a child recipe workflow.
// When resume is non-nil the payload is forwarded so the child can continue the rewind sequence.
func SignalChildRunMetadata(ctx workflow.Context, inv ops.Invocation, exec workflow.Execution, resume *runmetadata.Resume, recipeSetIndex *int) error {
	if inv.Deps == nil {
		return nil
	}
	info := workflow.GetInfo(ctx)
	parent := &runmetadata.Parent{
		WorkflowID:     info.WorkflowExecution.ID,
		RunID:          info.WorkflowExecution.RunID,
		InvocationHash: invocationHash(inv),
	}
	if recipeSetIndex != nil {
		parent.RecipeSetIndex = recipeSetIndex
	}

	signal := runmetadata.Signal{
		TargetRunID: exec.RunID,
		Parent:      parent,
	}

	if meta, ok := inv.Deps.RunMetadata(); ok && meta != nil {
		// Preserve the original reason so downstream observers retain context.
		signal.Reason = meta.Reason
	}

	if resume != nil && len(resume.ExecutionPath) > 0 {
		signal.Resume = &runmetadata.Resume{ExecutionPath: append([]runmetadata.Segment(nil), resume.ExecutionPath...)}
	}

	if err := workflow.SignalExternalWorkflow(ctx, exec.ID, exec.RunID, metadataSignalName, signal).Get(ctx, nil); err != nil {
		if retryErr := workflow.SignalExternalWorkflow(ctx, exec.ID, "", metadataSignalName, signal).Get(ctx, nil); retryErr != nil {
			return retryErr
		}
		return nil
	}
	return nil
}

func invocationHash(inv ops.Invocation) string {
	if inv.ID != "" {
		return inv.ID
	}
	return inv.Hash()
}
