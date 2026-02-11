package story

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	coreworkflow "github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

var (
	ErrReplayMismatch   = errors.New("story replay mismatch")
	ErrReplayInProgress = errors.New("story replay in progress")
)

// BuildJobRunStory constructs a recipe-centric JobRunStory by replaying the recipe execution
// using recorded outcomes from swf.GetJobRunResponse.
func BuildJobRunStory(ctx context.Context, engine swf.SWFEngine, projectID string, run swf.GetJobRunResponse, start workflowctl.StartJob, logger *slog.Logger, opts ...compiler.ExecutionOptions) (*model.JobRunStory, error) {
	if engine == nil {
		return nil, fmt.Errorf("engine is required")
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("projectID is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	jobKey := run.Job.JobKey

	rec, recipeArtifactName, err := loadRecipeFromJobStartArtifacts(ctx, engine, projectID, jobKey.JobId, run.Start, start.RecipeName)
	if err != nil {
		return nil, err
	}

	commitCtx := contextual.GitCommitContext{ParentRef: start.GitRef}
	inputs := start.Inputs
	if inputs == nil {
		inputs = map[string]interface{}{}
	}

	// Build one story per SWF job attempt (recipe retry). Each job attempt is a distinct ExecuteRecipe
	// run and should not consume tasks from other attempts.
	attemptRoots := make([]*model.JobRunStoryNode, 0, len(run.Attempts))
	var latestExecErr error
	for i := range run.Attempts {
		att := run.Attempts[i]

		// For non-latest attempts, force terminal semantics so replay doesn't treat missing tasks
		// as "in progress".
		replayStatus := run.Job.Status
		if i < len(run.Attempts)-1 {
			if att.Outcome.Status == swf.TaskOutcomeStatusFailed {
				replayStatus = swf.JobStatusCrashConcern
			} else {
				replayStatus = swf.JobStatusCompleted
			}
		}

		jobCtx := NewStoryBuildingContext(engine, projectID, jobKey, run.Job.JobType, replayStatus, []swf.JobAttempt{att}, logger)
		tree := newTreeBuilder()
		recJobCtx := newRecordingJobContext(jobCtx, jobKey.JobId, tree)
		exec := newRecordingExecutor(compiler.DefaultRecipeExecutor{}, tree, recJobCtx)

		wCtx := coreworkflow.Context{
			JobContext:           recJobCtx,
			ServiceDependencies2: coreops.NewServiceDepsBuilder().Build(),
		}
		_, _, execErr := compiler.ExecuteRecipeWithExecutor(exec, wCtx, *rec, inputs, start.JobContext, commitCtx, opts...)

		root := exec.Root()
		if root != nil {
			root.JobAttempt = att.Attempt
		}
		attemptRoots = append(attemptRoots, root)

		if i == len(run.Attempts)-1 {
			latestExecErr = execErr
		}
	}

	var root *model.JobRunStoryNode
	if len(attemptRoots) > 0 {
		root = attemptRoots[len(attemptRoots)-1]
		if root != nil && len(attemptRoots) > 1 {
			root.PastAttempts = attemptRoots[:len(attemptRoots)-1]
		}
	}

	storyStatus := mapStoryStatus(run.Job.Status)
	if storyStatus == model.WorkflowStatusRunning && root != nil {
		// If replay stopped due to an in-progress task, the root status should reflect "running".
		root.Status = model.JobRunStoryNodeStatusRunning
	}

	recipeID := strings.TrimSpace(rec.GetMetadata().ID)
	if recipeID == "" {
		recipeID = strings.TrimSpace(start.RecipeName)
	}
	recipeName := recipeID
	if recipeName == "" {
		recipeName = strings.TrimSpace(start.RecipeName)
	}

	story := &model.JobRunStory{
		JobID:              jobKey.JobId,
		InvocationSequence: 0,
		Recipe: model.JobRunStoryRecipe{
			ID:      recipeID,
			Name:    recipeName,
			Version: strings.TrimSpace(rec.GetMetadata().Version),
			Source: model.JobRunStoryRecipeSource{
				Kind:         "jobStartArtifact",
				ArtifactName: recipeArtifactName,
			},
		},
		Status:     storyStatus,
		StartedAt:  run.Job.CreatedAt,
		FinishedAt: run.Job.ArchivedAt,
		Root:       root,
	}
	if root != nil {
		story.InvocationSequence = root.InvokeSeq
	}
	// Best-effort: infer finished_at for nodes from sibling ordering + job finished time.
	inferFinishedTimes(story.Root, story.FinishedAt)

	// For failed/running runs, recipe replay naturally returns an error. The story itself is still useful.
	if latestExecErr == nil {
		return story, nil
	}

	// If the run is still active, treat replay errors as partial story.
	if story.Status == model.WorkflowStatusRunning {
		return story, nil
	}

	// If the job is terminal but we could not deterministically replay the recipe structure, surface a mismatch.
	// This typically indicates the recipe definition differs from what was executed, or the run timeline is incomplete.
	if errors.Is(latestExecErr, ErrReplayMismatch) {
		return story, latestExecErr
	}
	// JSON decode issues for task outputs are hard failures; they indicate corrupted/non-conforming task outputs.
	var syntaxErr *json.SyntaxError
	if errors.As(latestExecErr, &syntaxErr) {
		return story, latestExecErr
	}
	// Default: return story without failing the API for terminal failed runs.
	return story, nil
}

func loadRecipeFromJobStartArtifacts(ctx context.Context, engine swf.SWFEngine, tenantID, jobID string, start swf.JobStart, recipeName string) (*recipe.Recipe, string, error) {
	if start.Input == nil {
		return nil, "", fmt.Errorf("%w: job start missing input", ErrReplayMismatch)
	}
	arts := start.Input.Artifacts
	if len(arts) == 0 {
		return nil, "", fmt.Errorf("%w: job start missing artifacts", ErrReplayMismatch)
	}

	candidates := make([]*recipe.Recipe, 0, 2)
	candidateNames := make([]string, 0, 2)

	for _, info := range arts {
		if !strings.HasSuffix(info.Name, starter.RecipeArtifactSuffix) {
			continue
		}
		key := swf.ArtifactKey{
			JobId:       jobID,
			TaskOrdinal: start.Ordinal,
			Name:        info.Name,
			SizeBytes:   info.SizeBytes,
		}
		art, err := engine.GetArtifact(tenantID, key)
		if err != nil {
			return nil, "", fmt.Errorf("%w: failed to load recipe artifact %q: %w", ErrReplayMismatch, info.Name, err)
		}
		bytes, err := art.Bytes(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("%w: failed to read recipe artifact %q: %w", ErrReplayMismatch, info.Name, err)
		}
		var r recipe.Recipe
		if err := yaml.Unmarshal(bytes, &r); err != nil {
			return nil, "", fmt.Errorf("%w: failed to parse recipe artifact %q: %w", ErrReplayMismatch, info.Name, err)
		}
		candidates = append(candidates, &r)
		candidateNames = append(candidateNames, info.Name)
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("%w: no recipe artifacts found on job start", ErrReplayMismatch)
	}

	recipeName = strings.TrimSpace(recipeName)
	if recipeName != "" {
		for i, r := range candidates {
			if r == nil {
				continue
			}
			id := strings.TrimSpace(r.GetMetadata().ID)
			artName := strings.TrimSuffix(candidateNames[i], starter.RecipeArtifactSuffix)
			if recipeName == id || recipeName == artName {
				return r, candidateNames[i], nil
			}
		}
	}

	if len(candidates) == 1 {
		return candidates[0], candidateNames[0], nil
	}

	return nil, "", fmt.Errorf("%w: multiple recipes found on job start but none matched recipe name %q", ErrReplayMismatch, recipeName)
}

func mapStoryStatus(status swf.JobStatus) model.WorkflowStatus {
	switch status {
	case swf.JobStatusActive, swf.JobStatusPendingJobs, swf.JobStatusAwaitingFuture, swf.JobStatusReady:
		return model.WorkflowStatusRunning
	case swf.JobStatusCompleted:
		return model.WorkflowStatusCompleted
	case swf.JobStatusCancelled:
		return model.WorkflowStatusCanceled
	case swf.JobStatusExpired:
		return model.WorkflowStatusTimedOut
	case swf.JobStatusCrashConcern:
		return model.WorkflowStatusFailed
	default:
		return model.WorkflowStatusUnknown
	}
}
