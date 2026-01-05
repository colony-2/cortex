package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/google/uuid"
)

// ExecOpInput defines the codex.exec activity inputs expected from recipe-worker.
type ExecOpInput struct {
	Prompt    string            `json:"prompt"`
	SessionID string            `json:"sessionId,omitempty"`
	Model     string            `json:"model,omitempty"`
	Env       map[string]string `json:"env,omitempty"`

	Context map[string]interface{} `json:"context,omitempty"`
	Raw     map[string]interface{} `json:"-" mapstructure:",remain"`
}

// ExecOpOutput mirrors the structured response surfaced by the codex library.
type ExecOpOutput struct {
	Status              string       `json:"status"`
	SessionID           string       `json:"sessionId"`
	AssistantSummary    string       `json:"assistantSummary"`
	IncompleteReason    string       `json:"incompleteReason"`
	IncompleteCategory  string       `json:"incompleteCategory"`
	PendingDependencies []Dependency `json:"pendingDependencies"`
	ErrorMessage        string       `json:"errorMessage"`
}

// executeLibrary is replaceable for tests.
var executeLibrary = Execute

// GetOp exposes the codex.exec activity as a RegisterableOp.
func GetOp() ops.RegisterableOp {
	return ops.NewActivityMappedOpV2[ExecOpInput, ExecOpOutput](ops.OpMetadata{
		Type:           "codex.exec",
		Description:    "Runs Codex CLI in non-interactive mode with structured output capture",
		Version:        "1.0.0",
		DefaultTimeout: 30 * time.Minute,
	}, runCodexActivity)
}

func runCodexActivity(inv ops.OpDependencies, actx context.Context, input ExecOpInput) (ExecOpOutput, error) {

	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return ExecOpOutput{}, workflow.NewNonRetryableApplicationError("prompt is required")
	}

	contextMap := cloneMap(input.Context)
	if contextMap == nil {
		if raw, ok := input.Raw["context"]; ok {
			contextMap, _ = raw.(map[string]interface{})
		}
	}
	if contextMap == nil {
		return ExecOpOutput{}, workflow.NewNonRetryableApplicationError("context is required")
	}

	worktree := stringFromMap(contextMap, "worktree")
	if worktree == "" {
		if gitMap := mapFromInterface(contextMap["git"]); gitMap != nil {
			worktree = stringFromMap(gitMap, "worktree_path")
		}
	}
	if worktree == "" {
		return ExecOpOutput{}, workflow.NewNonRetryableApplicationError("context.worktree is required")
	}

	cellName := stringFromMap(contextMap, "cellname")
	if cellName == "" {
		if gitMap := mapFromInterface(contextMap["git"]); gitMap != nil {
			cellName = stringFromMap(gitMap, "cell_name")
		}
	}

	cellRelPath := resolveCellRelativePath(worktree, cellName)

	opts := Options{
		Prompt:           prompt,
		SessionID:        strings.TrimSpace(input.SessionID),
		Model:            strings.TrimSpace(input.Model),
		ExtraEnv:         input.Env,
		WorktreeRoot:     worktree,
		CellRelativePath: cellRelPath,
	}

	result, stdoutPath, stderrPath, tempDir, err := executeLibrary(actx, opts)
	if err != nil {
		// Clean up immediately on error
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		return ExecOpOutput{}, err
	}

	// Create timestamp and ID for artifact naming (same for both artifacts)
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	executionID := uuid.NewString()

	// Track cleanup state - only cleanup once when artifacts are consumed
	cleanupCalled := &atomic.Bool{}

	cleanupFunc := func() error {
		if cleanupCalled.CompareAndSwap(false, true) {
			return os.RemoveAll(tempDir)
		}
		return nil
	}

	// Create stdout artifact
	stdoutArtifact := swf.NewArtifact(
		fmt.Sprintf("codex_stdout_%s_%s.jsonl", timestamp, executionID),
		func() (io.ReadCloser, int64, error) {
			f, err := os.Open(stdoutPath)
			if err != nil {
				return nil, 0, fmt.Errorf("open stdout: %w", err)
			}
			info, err := f.Stat()
			if err != nil {
				f.Close()
				return nil, 0, fmt.Errorf("stat stdout: %w", err)
			}
			return f, info.Size(), nil
		},
		cleanupFunc,
	)

	// Create stderr artifact
	stderrArtifact := swf.NewArtifact(
		fmt.Sprintf("codex_stderr_%s_%s.txt", timestamp, executionID),
		func() (io.ReadCloser, int64, error) {
			f, err := os.Open(stderrPath)
			if err != nil {
				return nil, 0, fmt.Errorf("open stderr: %w", err)
			}
			info, err := f.Stat()
			if err != nil {
				f.Close()
				return nil, 0, fmt.Errorf("stat stderr: %w", err)
			}
			return f, info.Size(), nil
		},
		cleanupFunc,
	)

	// Add both artifacts to output
	if err := inv.AddOutputArtifact(stdoutArtifact); err != nil {
		os.RemoveAll(tempDir) // Clean up on artifact error
		return ExecOpOutput{}, fmt.Errorf("add stdout artifact: %w", err)
	}
	if err := inv.AddOutputArtifact(stderrArtifact); err != nil {
		os.RemoveAll(tempDir) // Clean up on artifact error
		return ExecOpOutput{}, fmt.Errorf("add stderr artifact: %w", err)
	}

	output := ExecOpOutput{
		Status:              string(result.Status),
		SessionID:           safeString(result.SessionID),
		AssistantSummary:    safeString(result.AssistantSummary),
		IncompleteReason:    safeString(result.IncompleteReason),
		IncompleteCategory:  safeString(result.IncompleteCategory),
		PendingDependencies: copyDependencies(result.PendingDependencies),
		ErrorMessage:        safeString(result.ErrorMessage),
		// StdoutBlobURI field removed - use output artifacts
		// Stderr field removed - use output artifacts
	}
	return output, nil
	// Temp dir cleaned up by artifact cleanup callback (after both artifacts consumed)
}

func copyDependencies(in []Dependency) []Dependency {
	if len(in) == 0 {
		return []Dependency{}
	}
	out := make([]Dependency, len(in))
	copy(out, in)
	return out
}

func safeString(s string) string {
	if s == "" {
		return ""
	}
	return s
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func mapFromInterface(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func resolveCellRelativePath(worktree, cellName string) string {
	name := strings.TrimSpace(cellName)
	if name == "" {
		return "."
	}
	cleaned := filepath.Clean(name)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "."
	}

	candidates := []string{
		filepath.Join("cells", cleaned),
		cleaned,
	}
	for _, candidate := range candidates {
		full := filepath.Join(worktree, candidate)
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			return candidate
		}
	}

	// Fall back to default cells/<cell> path and allow downstream MkdirAll to create it.
	preferred := filepath.Join("cells", cleaned)
	if strings.HasPrefix(preferred, "..") {
		return "."
	}
	return preferred
}

func digestPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:8])
}
