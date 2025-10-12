package compiler

import (
	"errors"
	"time"

	"go.temporal.io/sdk/workflow"
)

const recipeRunMetadataSignalName = "recipe_run_metadata"

// RecipeRunMetadataSignalName exposes the Temporal signal name used for recipe metadata.
func RecipeRunMetadataSignalName() string {
	return recipeRunMetadataSignalName
}

// RecipeRunMetadataSignal models the payload delivered via the recipe_run_metadata signal.
type RecipeRunMetadataSignal struct {
	TargetRunID string                   `json:"target_run_id"`
	Parent      *RecipeRunMetadataParent `json:"parent,omitempty"`
	Resume      *RecipeRunMetadataResume `json:"resume,omitempty"`
	Reason      string                   `json:"reason,omitempty"`
}

// RecipeRunMetadataParent captures linkage to the parent workflow invocation.
type RecipeRunMetadataParent struct {
	WorkflowID     string `json:"workflow_id"`
	RunID          string `json:"run_id"`
	InvocationHash string `json:"invocation_hash"`
	RecipeSetIndex *int   `json:"recipe_set_index,omitempty"`
}

// RecipeRunMetadataResume contains the execution path to be replayed when rewinding.
type RecipeRunMetadataResume struct {
	ExecutionPath []RecipeRunMetadataSegment `json:"execution_path,omitempty"`
}

// RecipeRunMetadataSegment describes a single step in the execution path.
type RecipeRunMetadataSegment struct {
	InvocationHash string `json:"invocation_hash"`
	RunID          string `json:"run_id"`
	EventID        int64  `json:"event_id"`
	RecipeSetIndex *int   `json:"recipe_set_index,omitempty"`
}

type recipeRunMetadataKey struct{}

// waitForRecipeRunMetadata blocks workflow execution until a metadata signal for the current run is received.
// It ignores signals targeted at other runs and returns the latest matching payload when multiple arrive before
// execution resumes.
func waitForRecipeRunMetadata(ctx workflow.Context) (*RecipeRunMetadataSignal, error) {
	signalCh := workflow.GetSignalChannel(ctx, recipeRunMetadataSignalName)
	logger := workflow.GetLogger(ctx)
	runID := workflow.GetInfo(ctx).WorkflowExecution.RunID

	var payload RecipeRunMetadataSignal
	for {
		if ok := signalCh.Receive(ctx, &payload); !ok {
			return nil, errors.New("recipe_run_metadata signal channel closed")
		}

		if payload.TargetRunID == "" {
			logger.Warn("received recipe_run_metadata without target_run_id; defaulting to current run")
			payload.TargetRunID = runID
		}

		if payload.TargetRunID != runID {
			logger.Debug("ignoring recipe_run_metadata for different run", "target_run_id", payload.TargetRunID, "run_id", runID)
			continue
		}
		break
	}

	latest := payload
	const idleDrainChecks = 3
	idleCount := 0
	for idleCount < idleDrainChecks {
		var next RecipeRunMetadataSignal
		if !signalCh.ReceiveAsync(&next) {
			idleCount++
			_ = workflow.Sleep(ctx, time.Millisecond)
			continue
		}
		idleCount = 0

		if next.TargetRunID == "" {
			logger.Warn("received recipe_run_metadata without target_run_id; defaulting to current run")
			next.TargetRunID = runID
		}

		if next.TargetRunID != runID {
			logger.Debug("ignoring recipe_run_metadata for different run", "target_run_id", next.TargetRunID, "run_id", runID)
			continue
		}

		latest = next
	}

	return &latest, nil
}

func withRecipeRunMetadata(ctx workflow.Context, metadata *RecipeRunMetadataSignal) workflow.Context {
	if metadata == nil {
		return ctx
	}
	return workflow.WithValue(ctx, recipeRunMetadataKey{}, metadata)
}

func recipeRunMetadataFromContext(ctx workflow.Context) (*RecipeRunMetadataSignal, bool) {
	if val := ctx.Value(recipeRunMetadataKey{}); val != nil {
		if metadata, ok := val.(*RecipeRunMetadataSignal); ok {
			return metadata, true
		}
	}
	return nil, false
}
