package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
)

type capture struct {
	options Options
}

func TestRunCodexActivitySuccess(t *testing.T) {
	worktree := t.TempDir()
	cellDir := filepath.Join(worktree, "cells", "alpha")
	require.NoError(t, os.MkdirAll(cellDir, 0o755))

	var cap capture
	executeLibrary = func(ctx context.Context, opts Options) (Result, error) {
		cap.options = opts
		return Result{
			Status:              StatusCompleted,
			SessionID:           "sess-123",
			AssistantSummary:    "all good",
			PendingDependencies: []Dependency{},
			StdoutBlobURI:       "file://blob/codex/output",
		}, nil
	}
	defer func() { executeLibrary = Execute }()

	input := ExecOpInput{
		Prompt: "do something",
		Env:    map[string]string{"FOO": "BAR"},
		Context: map[string]interface{}{
			"worktree":  worktree,
			"blobstore": "file:///blob",
			"cellname":  "alpha",
		},
	}

	inv := ops.Invocation{RecipeID: "recipe", NodePath: "n/a", InvokeSeq: 1, BoxID: "box", ActivityID: "activity", ID: "inv-1"}

	out, err := runCodexActivity(inv, context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, StatusCompleted, Status(out.Status))
	require.Equal(t, "sess-123", out.SessionID)
	require.Equal(t, "all good", out.AssistantSummary)
	require.Equal(t, "file://blob/codex/output", out.StdoutBlobURI)

	require.Equal(t, "do something", cap.options.Prompt)
	require.Equal(t, "file:///blob", cap.options.BlobstoreURI)
	require.Equal(t, worktree, cap.options.WorktreeRoot)
	require.Equal(t, filepath.Join("cells", "alpha"), cap.options.CellRelativePath)
	require.Equal(t, "BAR", cap.options.ExtraEnv["FOO"])
	require.Equal(t, inv.Hash(), cap.options.WorkflowID)
}

func TestRunCodexActivityMissingPrompt(t *testing.T) {
	input := ExecOpInput{Prompt: ""}
	_, err := runCodexActivity(ops.Invocation{}, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityMissingContext(t *testing.T) {
	input := ExecOpInput{Prompt: "ok"}
	_, err := runCodexActivity(ops.Invocation{}, context.Background(), input)
	require.Error(t, err)
}

func TestRunCodexActivityLibraryError(t *testing.T) {
	executeLibrary = func(ctx context.Context, opts Options) (Result, error) {
		return Result{}, errors.New("boom")
	}
	defer func() { executeLibrary = Execute }()

	worktree := t.TempDir()
	input := ExecOpInput{
		Prompt: "ok",
		Context: map[string]interface{}{
			"worktree":  worktree,
			"blobstore": "file:///blob",
		},
	}
	_, err := runCodexActivity(ops.Invocation{}, context.Background(), input)
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
