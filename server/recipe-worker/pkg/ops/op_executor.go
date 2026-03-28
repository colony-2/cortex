package ops

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/core/pkg/logutil"
	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type opExecutor struct {
	deps       ops.ServiceDependencies2
	reg        ActivityRegistration
	controller *gitstate.Controller
}

type nextTaskOverride interface {
	NextTaskType() (string, bool)
}

func unchangedGitResult(input gitstate.GlobalGitTaskContext) contextual.GitCommitContext {
	if input.PersistHash != "" {
		return contextual.GitCommitContext{
			PersistHash: input.PersistHash,
			ParentHash:  input.ParentHash,
		}
	}
	return contextual.GitCommitContext{
		ParentRef: input.BaseRef,
	}
}

func currentGitResult(task *gitstate.GitTaskContext) contextual.GitCommitContext {
	parentRef := ""
	if task.PersistHash == "" {
		parentRef = task.BaseRef
	}
	return contextual.GitCommitContext{
		PersistHash: task.PersistHash,
		ParentHash:  task.ParentHash,
		ParentRef:   parentRef,
	}
}

func (t opExecutor) do(ctx context.Context, jobTool ops.JobTool, req ActivityInvocationRequest, inputArtifacts []swf.Artifact) (output ActivityInvocationOutput, outputArtifacts []swf.Artifact, err error) {
	deps := t.deps
	controller := t.controller
	reg := t.reg

	var zero ActivityInvocationOutput

	logger := slog.Default().With(
		"task_type", reg.TaskType,
		"op_type", reg.Metadata.Type,
		"step", reg.Step.Name,
		"step_index", reg.StepIndex,
		"ticket_id", req.GitTaskContext.TicketID,
		"cell_name", req.GitTaskContext.CellName,
		"cell_path", req.GitTaskContext.CellPath,
		"node_path", req.GitTaskContext.NodePath,
		"invoke_seq", req.GitTaskContext.InvokeSeq,
		"invoke_hash", req.GitTaskContext.InvokeHash,
		"input_artifact_count", len(inputArtifacts),
		"artifact_key_count", len(req.ArtifactKeys),
		"artifact_binding_count", len(req.Artifacts),
	)
	if jobTool != nil {
		key := jobTool.GetJobKey()
		logger = logger.With("tenant_id", key.TenantId, "job_id", key.JobId)
	}
	start := time.Now()

	// Create temporary worktree directory for this invocation
	workDir, err := createWorkDir()
	if err != nil {
		return zero, nil, fmt.Errorf("create temp worktree: %w", err)
	}
	worktreePath := filepath.Join(workDir, "worktree")

	defer func() {
		if err == nil {
			return
		}
		logger.Error("opExecutor.do failed",
			"duration_ms", time.Since(start).Milliseconds(),
			"work_dir", workDir,
			"worktree_path", worktreePath,
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(6),
		)
	}()

	inbox := filepath.Join(workDir, "inbox")
	err = os.Mkdir(inbox, 0o755)
	if err != nil {
		return zero, nil, err
	}
	outbox := filepath.Join(workDir, "outbox")
	err = os.Mkdir(outbox, 0o755)
	if err != nil {
		return zero, nil, err
	}

	if jobTool == nil {
		return zero, nil, fmt.Errorf("job tool is required")
	}
	replacements := map[string]string{
		contextual.WorktreePathSentinel:   worktreePath,
		contextual.WorkdirPathSentinel:    workDir,
		contextual.ArtifactInboxSentinel:  inbox,
		contextual.ArtifactOutboxSentinel: outbox,
		contextual.JobIdSentinel:          jobTool.GetJobKey().JobId,
	}

	defer removeWorkDir(worktreePath)

	incomingGitContext := req.GitTaskContext

	// Build full GitTaskContext for controller from global context + local worktree path
	fullContext := &gitstate.GitTaskContext{
		GlobalGitTaskContext: &req.GitTaskContext,
		WorktreePath:         worktreePath,
	}
	if fullContext.GlobalGitTaskContext == nil {
		return zero, nil, fmt.Errorf("git task context is required")
	}

	// Rehydrate referenced artifacts from keys.
	if len(req.ArtifactKeys) > 0 {

		ctl := deps.WorkflowControl()
		if ctl == nil {
			return zero, nil, fmt.Errorf("workflow control is required for artifact resolution")
		}
		rehydrated := make([]swf.Artifact, 0, len(req.ArtifactKeys))
		for _, key := range req.ArtifactKeys {
			artifact := ctl.GetArtifactLazy(ctx, jobTool.GetJobKey().TenantId, key)
			rehydrated = append(rehydrated, artifact)
		}
		inputArtifacts = append(inputArtifacts, rehydrated...)
	}

	artifactByKey := indexArtifactsByKey(inputArtifacts)
	if len(req.Artifacts) > 0 {
		if err := materializeArtifactBindings(ctx, inbox, req.Artifacts, artifactByKey); err != nil {
			return zero, nil, err
		}
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
		WithJobTool(jobTool).
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
	nextTask := reg.NextTaskType
	if override, ok := opDeps.(nextTaskOverride); ok {
		if overrideValue, set := override.NextTaskType(); set {
			nextTask = overrideValue
		}
	}
	if err := filepath.WalkDir(outbox, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(outbox, path)
		if err != nil {
			return err
		}
		artifact, err := swf.NewArtifactFromFile(rel, path)
		if err != nil {
			return err
		}
		outputArtifacts = append(outputArtifacts, artifact)
		return nil
	}); err != nil {
		return zero, outputArtifacts, err
	}

	if req.Const {
		if thinPackArtifact != nil {
			outputArtifacts = append(outputArtifacts, thinPackArtifact)
		}
		return ActivityInvocationOutput{
			OpOutput:  outputData,
			GitResult: unchangedGitResult(incomingGitContext),
			NextTask:  nextTask,
		}, outputArtifacts, nil
	}

	// Call PersistWithDiffs with full context
	persistOutput, persistArtifacts, err := controller.PersistWithDiffs(context.Background(), fullContext)
	if err != nil {
		return zero, outputArtifacts, err
	}

	gitResult := currentGitResult(fullContext)

	// Handle artifact pass-through logic
	if persistOutput != nil && persistOutput.HasChanges {
		// PersistWithDiffs created artifacts (changes were made)
		// Append all artifacts (thin pack + diffs) to output
		outputArtifacts = append(outputArtifacts, persistArtifacts...)
	} else {
		// Preserve input git state for unchanged tasks so hash-mode never regresses to ref-mode.
		gitResult = unchangedGitResult(incomingGitContext)
		if thinPackArtifact != nil {
			// No changes, but we had an input thin pack - pass through the SAME artifact
			// This avoids re-uploading; SWF can reuse the existing artifact
			outputArtifacts = append(outputArtifacts, thinPackArtifact)
		}
	}

	return ActivityInvocationOutput{
		OpOutput:  outputData,
		GitResult: gitResult,
		NextTask:  nextTask,
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
	for sentinel, replacement := range replacements {
		if strings.Contains(val, sentinel) {
			val = strings.ReplaceAll(val, sentinel, replacement)
		}
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

func indexArtifactsByKey(artifacts []swf.Artifact) map[string]swf.Artifact {
	index := make(map[string]swf.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		key, err := artifact.ArtifactKey()
		if err != nil {
			continue
		}
		index[artifactKeyIdentity(key)] = artifact
	}
	return index
}

func materializeArtifactBindings(ctx context.Context, inbox string, bindings map[string]swf.ArtifactKey, artifactsByKey map[string]swf.Artifact) error {
	for name, key := range bindings {
		if err := validateBindingName(name); err != nil {
			return fmt.Errorf("invalid artifact binding %q: %w", name, err)
		}
		artifact, ok := artifactsByKey[artifactKeyIdentity(key)]
		if !ok {
			return fmt.Errorf("artifact binding %q refers to missing artifact %s", name, artifactKeyIdentity(key))
		}

		destPath, err := bindingDestination(inbox, name, artifact.Name())
		if err != nil {
			return fmt.Errorf("artifact binding %q invalid destination: %w", name, err)
		}
		if _, err := os.Stat(destPath); err == nil {
			return fmt.Errorf("artifact binding %q would overwrite %s", name, destPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("artifact binding %q stat failed: %w", name, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("artifact binding %q mkdir failed: %w", name, err)
		}
		if err := artifact.SaveToFile(ctx, destPath); err != nil {
			return fmt.Errorf("artifact binding %q save failed: %w", name, err)
		}
	}
	return nil
}

func validateBindingName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("name must be relative")
	}
	for _, segment := range splitPathSegments(name) {
		if segment == ".." {
			return fmt.Errorf("name must not contain '..' segments")
		}
	}
	return nil
}

func bindingDestination(inbox, name, artifactName string) (string, error) {
	if hasTrailingSlash(name) {
		trimmed := strings.TrimRight(name, "/\\")
		if trimmed == "" {
			return "", fmt.Errorf("name cannot be root")
		}
		name = filepath.Join(trimmed, artifactName)
	}
	destPath := filepath.Clean(filepath.Join(inbox, name))
	if !strings.HasPrefix(destPath, inbox+string(filepath.Separator)) && destPath != inbox {
		return "", fmt.Errorf("destination escapes inbox")
	}
	return destPath, nil
}

func splitPathSegments(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return r == '/' || r == '\\'
	})
}

func hasTrailingSlash(name string) bool {
	return strings.HasSuffix(name, "/") || strings.HasSuffix(name, "\\")
}

func artifactKeyIdentity(key swf.ArtifactKey) string {
	return fmt.Sprintf("%s:%d:%s", key.JobId, key.TaskOrdinal, key.Name)
}
