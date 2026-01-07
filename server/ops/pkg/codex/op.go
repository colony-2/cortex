package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/google/uuid"
)

// ExecOpInput defines the codex.exec activity inputs expected from recipe-worker.
type ExecOpInput struct {
	Prompt           string            `json:"prompt"`
	SessionID        string            `json:"sessionId,omitempty"`
	Model            string            `json:"model,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	WorktreePath     string            `json:"worktree_path"`
	CellRelativePath string            `json:"cell_relative_path"`
}

// ExecOpOutput mirrors the structured response surfaced by the codex library.
type ExecOpOutput struct {
	Status              string       `json:"status"`
	SessionID           string       `json:"sessionId"`
	AssistantSummary    string       `json:"assistantSummary"`
	IncompleteReason    string       `json:"incompleteReason"`
	IncompleteCategory  string       `json:"incompleteCategory"`
	PendingDependencies []Dependency `json:"pendingDependencies"`
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

	worktree := strings.TrimSpace(input.WorktreePath)
	if worktree == "" {
		return ExecOpOutput{}, workflow.NewNonRetryableApplicationError("worktree_path is required")
	}
	cellRelPath := strings.TrimSpace(input.CellRelativePath)
	if cellRelPath == "" {
		return ExecOpOutput{}, workflow.NewNonRetryableApplicationError("cell_relative_path is required")
	}

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

	executionID := uuid.NewString()
	debugArtifactf("execution_id=%s stdout=%q stderr=%q temp_dir=%q", executionID, stdoutPath, stderrPath, tempDir)

	stdoutCleanup := func() error {
		if stdoutPath != "" {
			err := os.Remove(stdoutPath)
			debugArtifactf("cleanup stdout path=%q err=%v\nstack=%s", stdoutPath, err, debug.Stack())
			return err
		}
		return nil
	}
	stderrCleanup := func() error {
		if stderrPath != "" {
			err := os.Remove(stderrPath)
			debugArtifactf("cleanup stderr path=%q err=%v\nstack=%s", stderrPath, err, debug.Stack())
			return err
		}
		return nil
	}

	// Create stdout artifact
	stdoutArtifact := swf.NewArtifact(
		"stdout.jsonl",
		func() (io.ReadCloser, int64, error) {
			debugArtifactStat("stdout", stdoutPath)
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
		stdoutCleanup,
	)

	// Create stderr artifact
	stderrArtifact := swf.NewArtifact(
		"stderr.txt",
		func() (io.ReadCloser, int64, error) {
			debugArtifactStat("stderr", stderrPath)
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
		stderrCleanup,
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

	debugArtifactStat("stdout-before-return", stdoutPath)
	debugArtifactStat("stderr-before-return", stderrPath)

	output := ExecOpOutput{
		Status:              string(result.Status),
		SessionID:           safeString(result.SessionID),
		AssistantSummary:    safeString(result.AssistantSummary),
		IncompleteReason:    safeString(result.IncompleteReason),
		IncompleteCategory:  safeString(result.IncompleteCategory),
		PendingDependencies: copyDependencies(result.PendingDependencies),
		// StdoutBlobURI field removed - use output artifacts
		// Stderr field removed - use output artifacts
	}
	if result.Status == StatusError {
		msg := strings.TrimSpace(result.ErrorMessage)
		if msg == "" {
			msg = "codex reported an error"
		}
		return ExecOpOutput{}, fmt.Errorf("codex error: %s", msg)
	}
	return output, nil
	// Temp dir cleaned up by artifact cleanup callback (after both artifacts consumed)
}

var artifactDebugOnce sync.Once
var artifactDebugEnabled bool

func artifactDebug() bool {
	artifactDebugOnce.Do(func() {
		val := strings.TrimSpace(os.Getenv("VIBETHIS_CODEX_ARTIFACT_DEBUG"))
		if val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "yes") {
			artifactDebugEnabled = true
		}
	})
	return artifactDebugEnabled
}

func debugArtifactf(format string, args ...interface{}) {
	if artifactDebug() {
		log.Printf("codex.exec artifacts: "+format, args...)
	}
}

func debugArtifactStat(label, path string) {
	if !artifactDebug() {
		return
	}
	if path == "" {
		log.Printf("codex.exec artifacts: %s path empty", label)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		log.Printf("codex.exec artifacts: %s stat path=%q err=%v", label, path, err)
		return
	}
	log.Printf("codex.exec artifacts: %s stat path=%q size=%d", label, path, info.Size())
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

func digestPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:8])
}
