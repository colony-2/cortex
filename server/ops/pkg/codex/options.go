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
	if strings.TrimSpace(o.WorktreeRoot) == "" {
		return errMissingWorktree
	}
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
	if strings.TrimSpace(o.CellRelativePath) == "" {
		o.CellRelativePath = "."
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
	rel, err := filepath.Rel(o.WorktreeRoot, hostPath)
	if err != nil {
		return "", fmt.Errorf("compute container path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %s escapes worktree", hostPath)
	}
	return filepath.ToSlash(filepath.Join("/src", rel)), nil
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
