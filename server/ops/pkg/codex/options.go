package codex

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	errEmptyPrompt     = errors.New("codex: prompt is required")
	errMissingWorktree = errors.New("codex: worktree root is required")
)

func (o *Options) validate() error {
	if strings.TrimSpace(o.Prompt) == "" {
		return errEmptyPrompt
	}
	o.WorktreeRoot = strings.TrimSpace(o.WorktreeRoot)
	if o.WorktreeRoot == "" {
		return errMissingWorktree
	}
	o.WorktreeRoot = filepath.Clean(o.WorktreeRoot)
	o.WorkDirRoot = strings.TrimSpace(o.WorkDirRoot)
	if o.WorkDirRoot == "" {
		// Backward-compatible fallback for older callers.
		o.WorkDirRoot = o.WorktreeRoot
	}
	o.WorkDirRoot = filepath.Clean(o.WorkDirRoot)
	o.ArtifactInbox = strings.TrimSpace(o.ArtifactInbox)
	if o.ArtifactInbox == "" {
		o.ArtifactInbox = filepath.Join(o.WorkDirRoot, "inbox")
	}
	o.ArtifactInbox = filepath.Clean(o.ArtifactInbox)
	o.ArtifactOutbox = strings.TrimSpace(o.ArtifactOutbox)
	if o.ArtifactOutbox == "" {
		o.ArtifactOutbox = filepath.Join(o.WorkDirRoot, "outbox")
	}
	o.ArtifactOutbox = filepath.Clean(o.ArtifactOutbox)
	if o.ExtraEnv == nil {
		o.ExtraEnv = map[string]string{}
	}
	if o.Clock == nil {
		o.Clock = realClock{}
	}
	if o.RunnerFactory == nil {
		o.RunnerFactory = defaultRunnerFactory
	}
	if len(o.StructuredSchema) == 0 {
		o.StructuredSchema = structuredOutputSchema
	}
	o.CellRelativePath = strings.TrimSpace(o.CellRelativePath)
	if o.CellRelativePath == "" {
		o.CellRelativePath = "."
	}
	o.CellRelativePath = filepath.Clean(o.CellRelativePath)
	if o.CellRelativePath == "." {
		o.CellRelativePath = "."
	} else if filepath.IsAbs(o.CellRelativePath) || o.CellRelativePath == ".." || strings.HasPrefix(o.CellRelativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("codex: cell relative path %q escapes worktree", o.CellRelativePath)
	}
	if err := ensureDescendantPath(o.WorkDirRoot, o.WorktreeRoot, "worktree root"); err != nil {
		return err
	}
	if err := ensureDescendantPath(o.WorkDirRoot, o.ArtifactInbox, "artifact inbox"); err != nil {
		return err
	}
	if err := ensureDescendantPath(o.WorkDirRoot, o.ArtifactOutbox, "artifact outbox"); err != nil {
		return err
	}
	return nil
}

func (o Options) stdoutPath(dir string) string {
	return filepath.Join(dir, "stdout.jsonl")
}

func (o Options) stderrPath(dir string) string {
	return filepath.Join(dir, "stderr.txt")
}

func (o Options) containerPath(hostPath string) (string, error) {
	rel, err := filepath.Rel(o.WorkDirRoot, hostPath)
	if err != nil {
		return "", fmt.Errorf("compute container path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes workdir", hostPath)
	}
	return filepath.ToSlash(filepath.Join("/src", rel)), nil
}

func (o Options) relativeToWorkdir(hostPath string) (string, error) {
	rel, err := filepath.Rel(o.WorkDirRoot, hostPath)
	if err != nil {
		return "", fmt.Errorf("compute workdir relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes workdir", hostPath)
	}
	return rel, nil
}

func (o Options) stdoutRelativePath(ts string, id string) string {
	return filepath.ToSlash(filepath.Join("codex", ts, id+".jsonl"))
}

func (o Options) copyEnv() map[string]string {
	if len(o.ExtraEnv) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(o.ExtraEnv))
	for k, v := range o.ExtraEnv {
		cloned[k] = v
	}
	return cloned
}

func ensureDescendantPath(basePath string, targetPath string, label string) error {
	rel, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return fmt.Errorf("codex: resolve %s path: %w", label, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("codex: %s %q escapes workdir %q", label, targetPath, basePath)
	}
	return nil
}
