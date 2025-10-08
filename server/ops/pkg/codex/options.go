package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	errEmptyPrompt       = errors.New("codex: prompt is required")
	errMissingWorktree   = errors.New("codex: worktree root is required")
	errMissingBlobstore  = errors.New("codex: blobstore URI is required")
	errMissingWorkflowID = errors.New("codex: workflow ID is required")
)

func (o *Options) validate() error {
	if strings.TrimSpace(o.Prompt) == "" {
		return errEmptyPrompt
	}
	if strings.TrimSpace(o.WorktreeRoot) == "" {
		return errMissingWorktree
	}
	if strings.TrimSpace(o.BlobstoreURI) == "" {
		return errMissingBlobstore
	}
	if strings.TrimSpace(o.WorkflowID) == "" {
		return errMissingWorkflowID
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
	if o.BlobStore == nil {
		o.BlobStore = fileBlobStore{}
	}
	if len(o.StructuredSchema) == 0 {
		o.StructuredSchema = structuredOutputSchema
	}
	if strings.TrimSpace(o.CellRelativePath) == "" {
		o.CellRelativePath = "."
	}
	return nil
}

func (o Options) tempDir() (string, error) {
	rwPath := filepath.Join(o.WorktreeRoot, o.CellRelativePath)
	if err := os.MkdirAll(rwPath, 0o755); err != nil {
		return "", fmt.Errorf("create cell path: %w", err)
	}
	dir, err := os.MkdirTemp(rwPath, "codex-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return dir, nil
}

func (o Options) schemaPath(dir string) string {
	return filepath.Join(dir, "schema.json")
}

func (o Options) stdoutPath(dir string) string {
	return filepath.Join(dir, "stdout.jsonl")
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

func (o Options) sanitizeWorkflowID() string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			fallthrough
		case r >= 'A' && r <= 'Z':
			fallthrough
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, o.WorkflowID)
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		sanitized = "workflow"
	}
	return sanitized
}

func (o Options) stdoutRelativePath(ts string, id string) string {
	wf := o.sanitizeWorkflowID()
	return filepath.ToSlash(filepath.Join("codex", wf, ts, id+".jsonl"))
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
