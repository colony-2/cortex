package compiler

import (
	"errors"
	"time"

	runmd "github.com/divisive-ai/vibethis/server/recipe-core/pkg/runmetadata"
	"go.temporal.io/sdk/workflow"
)

const recipeRunMetadataSignalName = "recipe_run_metadata"

// RecipeRunMetadataSignalName exposes the Temporal signal name used for recipe metadata.
func RecipeRunMetadataSignalName() string {
	return recipeRunMetadataSignalName
}

// Type aliases maintain backwards compatibility for existing call sites while moving the
// canonical definitions into recipe-core.
type RecipeRunMetadataSignal = runmd.Signal
type RecipeRunMetadataParent = runmd.Parent
type RecipeRunMetadataResume = runmd.Resume
type RecipeRunMetadataSegment = runmd.Segment

// waitForRecipeRunMetadata blocks workflow execution until a metadata signal for the current run is received.
// It ignores signals targeted at other runs and returns the latest matching payload when multiple arrive before
// execution resumes.
func waitForRecipeRunMetadata(ctx workflow.Context) (*runmd.Signal, error) {
	signalCh := workflow.GetSignalChannel(ctx, recipeRunMetadataSignalName)
	logger := workflow.GetLogger(ctx)
	runID := workflow.GetInfo(ctx).WorkflowExecution.RunID

	var payload runmd.Signal
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
		var next runmd.Signal
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
