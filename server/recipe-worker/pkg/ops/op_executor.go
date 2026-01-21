package ops

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/swf-go/pkg/swf"
)

type opExecutor struct {
	deps       ops.ServiceDependencies2
	reg        ActivityRegistration
	controller *gitstate.Controller
}

func (t *opExecutor) do(ctx context.Context, jobKey swf.JobKey, req ActivityInvocationRequest, inputArtifacts []swf.Artifact) (output ActivityInvocationOutput, outputArtifacts []swf.Artifact, err error) {
	deps := t.deps
	controller := t.controller
	reg := t.reg

	var zero ActivityInvocationOutput

	// Create temporary worktree directory for this invocation
	workDir, err := createWorkDir()
	worktreePath := filepath.Join(workDir, "worktree")
	inbox := filepath.Join(workDir, "inbox")
	outbox := filepath.Join(workDir, "outbox")
	replacements := map[string]string{
		contextual.WorktreePathSentinel:   worktreePath,
		contextual.WorkdirPathSentinel:    workDir,
		contextual.ArtifactInboxSentinel:  inbox,
		contextual.ArtifactOutboxSentinel: outbox,
	}

	if err != nil {
		return zero, nil, fmt.Errorf("create temp worktree: %w", err)
	}
	defer removeWorkDir(worktreePath)

	// Build full GitTaskContext for controller from global context + local worktree path
	fullContext := &gitstate.GitTaskContext{
		GlobalGitTaskContext: &req.GitTaskContext,
		WorktreePath:         worktreePath,
	}

	// Rehydrate referenced artifacts from keys.
	if len(req.ArtifactKeys) > 0 {

		ctl := deps.WorkflowControl()
		if ctl == nil {
			return zero, nil, fmt.Errorf("workflow control is required for artifact resolution")
		}
		rehydrated := make([]swf.Artifact, 0, len(req.ArtifactKeys))
		for _, key := range req.ArtifactKeys {
			artifact, err := ctl.GetArtifact(ctx, jobKey.TenantId, key)
			if err != nil {
				return zero, nil, err
			}
			rehydrated = append(rehydrated, artifact)
		}
		inputArtifacts = append(inputArtifacts, rehydrated...)
	}

	// Find and filter input thin pack artifact
	var thinPackArtifact swf.Artifact
	var nonThinPackArtifacts []swf.Artifact

	for _, art := range inputArtifacts {
		if art.Name() == gitstate.ThinPackArtifactName {
			thinPackArtifact = art
		} else {
			nonThinPackArtifacts = append(nonThinPackArtifacts, art)
		}
	}

	// Call Restore with full context (includes WorktreePath)
	if err := controller.Restore(context.Background(), fullContext, thinPackArtifact); err != nil {
		return zero, nil, err
	}

	// CRITICAL: Hydrate sentinel values in input with actual worktree path
	// Templates like {{ environment.worktree_path }} resolved to sentinel at compile time
	// Now replace with real local path
	hydratedInput := replaceSentinels(req.Input, replacements)

	// Build OpDependencies with WorktreePath and filtered artifacts (thin pack hidden from operation)
	db := deps.Database()
	if tx, ok := swf.TxFromCtx(ctx); ok && tx != nil {
		db = tx
	}
	opDeps := ops.NewOpDependenciesBuilder().
		WithArtifacts(nonThinPackArtifacts).
		WithDatabase(db).
		WithWorkflowControl(deps.WorkflowControl()).
		WithWorktreePath(fullContext.WorktreePath).
		Build()

	// Ensure artifacts are collected on both success and failure paths
	defer func() {
		artifacts := opDeps.GetOutputArtifacts()
		outputArtifacts = append(outputArtifacts, artifacts...)
	}()

	// Execute operation with HYDRATED input (sentinels replaced)
	outputData, err := reg.Step.Invoke(opDeps, ctx, hydratedInput)
	if err != nil {
		return zero, outputArtifacts, err
	}

	// Call PersistWithDiffs with full context
	_, persistArtifacts, err := controller.PersistWithDiffs(context.Background(), fullContext)
	if err != nil {
		return zero, outputArtifacts, err
	}

	// Handle artifact pass-through logic
	if len(persistArtifacts) > 0 {
		// PersistWithDiffs created artifacts (changes were made)
		// Append all artifacts (thin pack + diffs) to output
		outputArtifacts = append(outputArtifacts, persistArtifacts...)
	} else if thinPackArtifact != nil {
		// No changes, but we had an input thin pack - pass through the SAME artifact
		// This avoids re-uploading; SWF can reuse the existing artifact
		outputArtifacts = append(outputArtifacts, thinPackArtifact)
	}
	// else: no input, no output - no artifacts to append

	// Build response using fullContext (which has updated hashes from Persist)
	parentRef := ""
	if fullContext.PersistHash == "" {
		parentRef = fullContext.BaseRef
	}

	return ActivityInvocationOutput{
		OpOutput: outputData,
		GitResult: contextual.GitCommitContext{
			PersistHash: fullContext.PersistHash,
			ParentHash:  fullContext.ParentHash,
			ParentRef:   parentRef,
		},
		NextTask: reg.NextTaskType,
	}, outputArtifacts, nil
}

// replaceSentinels recursively walks the input map and replaces sentinel values with actual worktree path
func replaceSentinels(input map[string]interface{}, replacements map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range input {
		result[k] = replaceSentinelValue(v, replacements)
	}
	return result
}

func replaceValue(val string, replacements map[string]string) string {
	if replacement, exists := replacements[val]; exists {
		return replacement
	}
	return val
}

// replaceSentinelValue handles different types recursively
func replaceSentinelValue(value interface{}, replacements map[string]string) interface{} {
	switch v := value.(type) {
	case string:
		return replaceValue(v, replacements)
	case map[string]interface{}:
		return replaceSentinels(v, replacements)
	case recipe.InputMap:
		// Convert to map[string]interface{} and process
		return recipe.InputMap(replaceSentinels(v, replacements))
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = replaceSentinelValue(item, replacements)
		}
		return result
	default:
		return v
	}
}
