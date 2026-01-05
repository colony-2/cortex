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
		Prompt: "do something",
		Env:    map[string]string{"FOO": "BAR"},
		Context: map[string]interface{}{
			"worktree": worktree,
			"cellname": "alpha",
		},
	}

	out, err := runCodexActivity(inv, context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, StatusCompleted, Status(out.Status))
	require.Equal(t, "sess-123", out.SessionID)
	require.Equal(t, "all good", out.AssistantSummary)

	// Verify artifacts were created
	require.Len(t, inv.artifacts, 2)
	require.Contains(t, inv.artifacts[0].Name(), "codex_stdout_")
	require.Contains(t, inv.artifacts[0].Name(), ".jsonl")
	require.Contains(t, inv.artifacts[1].Name(), "codex_stderr_")
	require.Contains(t, inv.artifacts[1].Name(), ".txt")

	require.Equal(t, "do something", cap.options.Prompt)
	require.Equal(t, worktree, cap.options.WorktreeRoot)
	require.Equal(t, filepath.Join("cells", "alpha"), cap.options.CellRelativePath)
	require.Equal(t, "BAR", cap.options.ExtraEnv["FOO"])
}

func TestRunCodexActivityMissingPrompt(t *testing.T) {
	input := ExecOpInput{Prompt: ""}
	_, err := runCodexActivity(nil, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityMissingContext(t *testing.T) {
	input := ExecOpInput{Prompt: "ok"}
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
		Prompt: "ok",
		Context: map[string]interface{}{
			"worktree": worktree,
		},
	}
	_, err := runCodexActivity(nil, context.Background(), input)
	require.Error(t, err)
}

func TestResolveCellRelativePath(t *testing.T) {
	worktree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(worktree, "cells", "alpha"), 0o755))

	path := resolveCellRelativePath(worktree, "alpha")
	require.Equal(t, filepath.Join("cells", "alpha"), path)

	path = resolveCellRelativePath(worktree, "")
	require.Equal(t, ".", path)

	require.NoError(t, os.MkdirAll(filepath.Join(worktree, "beta"), 0o755))
	path = resolveCellRelativePath(worktree, "beta")
	require.Equal(t, "beta", path)
}

func TestDigestPromptDeterministic(t *testing.T) {
	a := digestPrompt("hello")
	b := digestPrompt("hello")
	c := digestPrompt("world")
	require.Equal(t, a, b)
	require.NotEqual(t, a, c)
}
