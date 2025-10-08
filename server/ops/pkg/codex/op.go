package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"go.temporal.io/sdk/activity"
	templog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
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
	Stderr              string       `json:"stderr"`
	StdoutBlobURI       string       `json:"stdoutBlobUri"`
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

func runCodexActivity(inv ops.Invocation, actx context.Context, input ExecOpInput) (ExecOpOutput, error) {
	start := time.Now()
	var logger templog.Logger
	if activity.IsActivity(actx) {
		logger = activity.GetLogger(actx)
	}

	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return ExecOpOutput{}, temporal.NewNonRetryableApplicationError("prompt is required", "INVALID_INPUT", nil)
	}

	contextMap := cloneMap(input.Context)
	if contextMap == nil {
		if raw, ok := input.Raw["context"]; ok {
			contextMap, _ = raw.(map[string]interface{})
		}
	}
	if contextMap == nil {
		return ExecOpOutput{}, temporal.NewNonRetryableApplicationError("context is required", "INVALID_CONTEXT", nil)
	}

	worktree := stringFromMap(contextMap, "worktree")
	if worktree == "" {
		if gitMap := mapFromInterface(contextMap["git"]); gitMap != nil {
			worktree = stringFromMap(gitMap, "worktree_path")
		}
	}
	if worktree == "" {
		return ExecOpOutput{}, temporal.NewNonRetryableApplicationError("context.worktree is required", "INVALID_CONTEXT", nil)
	}

	blobstoreURI := stringFromMap(contextMap, "blobstore")
	if blobstoreURI == "" {
		if gitMap := mapFromInterface(contextMap["git"]); gitMap != nil {
			blobstoreURI = stringFromMap(gitMap, "blob_store_uri")
		}
	}
	if blobstoreURI == "" {
		return ExecOpOutput{}, temporal.NewNonRetryableApplicationError("context.blobstore is required", "INVALID_CONTEXT", nil)
	}

	cellName := stringFromMap(contextMap, "cellname")
	if cellName == "" {
		if gitMap := mapFromInterface(contextMap["git"]); gitMap != nil {
			cellName = stringFromMap(gitMap, "cell_name")
		}
	}

	cellRelPath := resolveCellRelativePath(worktree, cellName)
	promptHash := digestPrompt(prompt)

	if logger != nil {
		logger.Info("codex.exec started", "prompt_hash", promptHash, "sessionId", input.SessionID, "model", input.Model, "workflow", inv.ID)
	}

	opts := Options{
		Prompt:           prompt,
		SessionID:        strings.TrimSpace(input.SessionID),
		Model:            strings.TrimSpace(input.Model),
		ExtraEnv:         input.Env,
		WorktreeRoot:     worktree,
		CellRelativePath: cellRelPath,
		BlobstoreURI:     blobstoreURI,
		WorkflowID:       inv.Hash(),
	}

	result, err := executeLibrary(actx, opts)
	if err != nil {
		return ExecOpOutput{}, err
	}

	output := ExecOpOutput{
		Status:              string(result.Status),
		SessionID:           safeString(result.SessionID),
		AssistantSummary:    safeString(result.AssistantSummary),
		IncompleteReason:    safeString(result.IncompleteReason),
		IncompleteCategory:  safeString(result.IncompleteCategory),
		PendingDependencies: copyDependencies(result.PendingDependencies),
		ErrorMessage:        safeString(result.ErrorMessage),
		Stderr:              safeString(result.Stderr),
		StdoutBlobURI:       safeString(result.StdoutBlobURI),
	}

	if logger != nil {
		logger.Info("codex.exec finished", "status", output.Status, "sessionId", output.SessionID, "stdoutBlobUri", output.StdoutBlobURI, "duration", time.Since(start))
	}
	return output, nil
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
