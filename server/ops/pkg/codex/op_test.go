package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type capture struct {
	options Options
}

type fakeOpDependencies struct {
	artifacts []swf.Artifact
}

func (f *fakeOpDependencies) AddOutputArtifact(artifact swf.Artifact) error {
	f.artifacts = append(f.artifacts, artifact)
	return nil
}

func (f *fakeOpDependencies) GetInputArtifacts() []swf.Artifact {
	return nil
}

func (f *fakeOpDependencies) GetOutputArtifacts() []swf.Artifact {
	return f.artifacts
}

func (f *fakeOpDependencies) Database() *gorm.DB {
	return nil
}

func (f *fakeOpDependencies) WorkflowControl() workflowctl.WorkflowControl {
	return nil
}

func TestRunCodexActivitySuccess(t *testing.T) {
	worktree := t.TempDir()
	cellDir := filepath.Join(worktree, "cells", "alpha")
	require.NoError(t, os.MkdirAll(cellDir, 0o755))

	// Create temp files for stdout and stderr
	tempDir := t.TempDir()
	stdoutPath := filepath.Join(tempDir, "stdout.jsonl")
	stderrPath := filepath.Join(tempDir, "stderr.txt")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("test output"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte("test errors"), 0o644))

	var cap capture
	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		cap.options = opts
		return Result{
			Status:              StatusCompleted,
			SessionID:           "sess-123",
			AssistantSummary:    "all good",
			PendingDependencies: []Dependency{},
		}, stdoutPath, stderrPath, tempDir, nil
	}
	defer func() { executeLibrary = Execute }()

	inv := &fakeOpDependencies{}
	input := ExecOpInput{
		Prompt:           "do something",
		Env:              map[string]string{"FOO": "BAR"},
		WorktreePath:     worktree,
		CellRelativePath: filepath.Join("cells", "alpha"),
	}

	out, err := runCodexActivity(inv, context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, StatusCompleted, Status(out.Status))
	require.Equal(t, "sess-123", out.SessionID)
	require.Equal(t, "all good", out.AssistantSummary)

	// Verify artifacts were created
	require.Len(t, inv.artifacts, 2)
	require.Equal(t, "stdout.jsonl", inv.artifacts[0].Name())
	require.Equal(t, "stderr.txt", inv.artifacts[1].Name())

	require.Equal(t, "do something", cap.options.Prompt)
	require.Equal(t, worktree, cap.options.WorktreeRoot)
	require.Equal(t, filepath.Join("cells", "alpha"), cap.options.CellRelativePath)
	require.Equal(t, "BAR", cap.options.ExtraEnv["FOO"])

	stdoutBytes, err := inv.artifacts[0].Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test output", string(stdoutBytes))

	stderrBytes, err := inv.artifacts[1].Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test errors", string(stderrBytes))
}

func TestRunCodexActivityArtifactsRemainUntilBothConsumed(t *testing.T) {
	worktree := t.TempDir()

	tempDir := t.TempDir()
	stdoutPath := filepath.Join(tempDir, "stdout.jsonl")
	stderrPath := filepath.Join(tempDir, "stderr.txt")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("test output"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte("test errors"), 0o644))

	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		return Result{
			Status:              StatusCompleted,
			SessionID:           "sess-123",
			AssistantSummary:    "all good",
			PendingDependencies: []Dependency{},
		}, stdoutPath, stderrPath, tempDir, nil
	}
	defer func() { executeLibrary = Execute }()

	inv := &fakeOpDependencies{}
	input := ExecOpInput{
		Prompt:           "do something",
		WorktreePath:     worktree,
		CellRelativePath: "cells/alpha",
	}

	_, err := runCodexActivity(inv, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, inv.artifacts, 2)

	var stdoutArt, stderrArt swf.Artifact
	for _, art := range inv.artifacts {
		if art.Name() == "stdout.jsonl" {
			stdoutArt = art
		} else if art.Name() == "stderr.txt" {
			stderrArt = art
		}
	}
	require.NotNil(t, stdoutArt)
	require.NotNil(t, stderrArt)

	stdoutBytes, err := stdoutArt.Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test output", string(stdoutBytes))
	require.NoError(t, stdoutArt.Cleanup())

	_, err = os.Stat(stdoutPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(stderrPath)
	require.NoError(t, err)

	stderrBytes, err := stderrArt.Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test errors", string(stderrBytes))
	require.NoError(t, stderrArt.Cleanup())

	_, err = os.Stat(stderrPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestRunCodexActivityMissingPrompt(t *testing.T) {
	input := ExecOpInput{Prompt: ""}
	_, err := runCodexActivity(nil, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityMissingWorktreePath(t *testing.T) {
	input := ExecOpInput{
		Prompt:           "ok",
		CellRelativePath: "cells/alpha",
	}
	_, err := runCodexActivity(nil, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityMissingCellRelativePath(t *testing.T) {
	input := ExecOpInput{
		Prompt:       "ok",
		WorktreePath: "/tmp",
	}
	_, err := runCodexActivity(nil, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityLibraryError(t *testing.T) {
	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		return Result{}, "", "", "", errors.New("boom")
	}
	defer func() { executeLibrary = Execute }()

	worktree := t.TempDir()
	input := ExecOpInput{
		Prompt:           "ok",
		WorktreePath:     worktree,
		CellRelativePath: "cells/alpha",
	}
	_, err := runCodexActivity(nil, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityRegistersArtifactsOnTimeout(t *testing.T) {
	worktree := t.TempDir()
	tempDir := t.TempDir()
	stdoutPath := filepath.Join(tempDir, "stdout.jsonl")
	stderrPath := filepath.Join(tempDir, "stderr.txt")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("partial output before timeout"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte("error logs before timeout"), 0o644))

	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		// Simulate timeout: return error but with valid stdout/stderr paths
		return Result{Status: StatusError}, stdoutPath, stderrPath, tempDir, errors.New("context deadline exceeded")
	}
	defer func() { executeLibrary = Execute }()

	inv := &fakeOpDependencies{}
	input := ExecOpInput{
		Prompt:           "do something",
		WorktreePath:     worktree,
		CellRelativePath: "cells/alpha",
	}

	// Activity should return error but still register artifacts
	_, err := runCodexActivity(inv, context.Background(), input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context deadline exceeded")

	// Verify artifacts were still registered despite the error
	require.Len(t, inv.artifacts, 2)
	require.Equal(t, "stdout.jsonl", inv.artifacts[0].Name())
	require.Equal(t, "stderr.txt", inv.artifacts[1].Name())

	// Verify artifact contents are accessible
	stdoutBytes, err := inv.artifacts[0].Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "partial output before timeout", string(stdoutBytes))

	stderrBytes, err := inv.artifacts[1].Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "error logs before timeout", string(stderrBytes))
}

func TestRunCodexActivityReturnsErrorOnStatusError(t *testing.T) {
	worktree := t.TempDir()
	tempDir := t.TempDir()
	stdoutPath := filepath.Join(tempDir, "stdout.jsonl")
	stderrPath := filepath.Join(tempDir, "stderr.txt")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("{\"status\":\"error\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte(""), 0o644))

	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		return Result{
			Status:       StatusError,
			ErrorMessage: "codex failed",
		}, stdoutPath, stderrPath, tempDir, nil
	}
	defer func() { executeLibrary = Execute }()

	input := ExecOpInput{
		Prompt:           "do something",
		WorktreePath:     worktree,
		CellRelativePath: "cells/alpha",
	}
	_, err := runCodexActivity(&fakeOpDependencies{}, context.Background(), input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "codex failed")
}

func TestRunCodexActivityAllowsIncomplete(t *testing.T) {
	worktree := t.TempDir()
	tempDir := t.TempDir()
	stdoutPath := filepath.Join(tempDir, "stdout.jsonl")
	stderrPath := filepath.Join(tempDir, "stderr.txt")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("{\"status\":\"incomplete\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte(""), 0o644))

	executeLibrary = func(ctx context.Context, opts Options) (Result, string, string, string, error) {
		return Result{
			Status:             StatusIncomplete,
			AssistantSummary:   "needs follow-up",
			IncompleteReason:   "blocked",
			IncompleteCategory: "dependency_blockers",
		}, stdoutPath, stderrPath, tempDir, nil
	}
	defer func() { executeLibrary = Execute }()

	input := ExecOpInput{
		Prompt:           "do something",
		WorktreePath:     worktree,
		CellRelativePath: "cells/alpha",
	}
	out, err := runCodexActivity(&fakeOpDependencies{}, context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, string(StatusIncomplete), out.Status)
	require.Equal(t, "needs follow-up", out.AssistantSummary)
}

func TestDigestPromptDeterministic(t *testing.T) {
	a := digestPrompt("hello")
	b := digestPrompt("hello")
	c := digestPrompt("world")
	require.Equal(t, a, b)
	require.NotEqual(t, a, c)
}
