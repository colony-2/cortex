package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	shai "github.com/divisive-ai/vibethis/server/container/pkg/shai"
	"github.com/stretchr/testify/require"
)

type fakeClock struct{ ts time.Time }

func (f fakeClock) Now() time.Time { return f.ts }

type blobCall struct {
	baseURI  string
	relative string
	source   string
}

type fakeBlobStore struct {
	calls []blobCall
}

func (f *fakeBlobStore) Put(ctx context.Context, baseURI, relativePath, sourcePath string) (string, error) {
	f.calls = append(f.calls, blobCall{baseURI: baseURI, relative: relativePath, source: sourcePath})
	return relativePath, nil
}

type fakeRunner struct {
	runFn  func(ctx context.Context) error
	closed bool
}

func (f *fakeRunner) Run(ctx context.Context) error {
	if f.runFn != nil {
		return f.runFn(ctx)
	}
	return nil
}

func (f *fakeRunner) Close() error {
	f.closed = true
	return nil
}

type runnerHarness struct {
	t      *testing.T
	lines  []string
	stderr []string
	runErr error
	config *shai.EphemeralConfig
	runner *fakeRunner
}

func (h *runnerHarness) factory(cfg *shai.EphemeralConfig) (Runner, error) {
	h.config = cfg
	h.runner = &fakeRunner{runFn: func(ctx context.Context) error {
		for _, line := range h.lines {
			cfg.Output.OnStdout([]byte(line))
		}
		for _, line := range h.stderr {
			cfg.Output.OnStderr([]byte(line))
		}
		return h.runErr
	}}
	return h.runner, nil
}

func TestExecuteCompleted(t *testing.T) {
	worktree := t.TempDir()
	cellRel := "cells/a"
	require.NoError(t, os.MkdirAll(filepath.Join(worktree, cellRel), 0o755))

	harness := &runnerHarness{
		lines: []string{
			`{"type":"session.created","session_id":"sess-123"}`,
			`{"type":"item.completed","item":{"id":"item_1","item_type":"assistant_message","text":"{\"status\":\"completed\",\"assistantSummary\":\"All done\",\"incompleteReason\":\"\",\"incompleteCategory\":\"\",\"pendingDependencies\":[],\"errorMessage\":\"\"}"}}`,
		},
	}

	blob := &fakeBlobStore{}
	opts := Options{
		Prompt:           "do the task",
		WorktreeRoot:     worktree,
		CellRelativePath: cellRel,
		BlobstoreURI:     "file://" + worktree,
		WorkflowID:       "wf:123",
		Clock:            fakeClock{ts: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		RunnerFactory:    harness.factory,
		BlobStore:        blob,
	}

	res, err := Execute(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, res.Status)
	require.Equal(t, "All done", res.AssistantSummary)
	require.Equal(t, "sess-123", res.SessionID)
	require.Empty(t, res.ErrorMessage)
	require.Equal(t, 0, len(res.PendingDependencies))
	require.NotEmpty(t, res.StdoutBlobURI)
	require.True(t, strings.HasPrefix(res.StdoutBlobURI, "file://"+worktree+"/codex/wf-123/20240102T030405Z/"))

	require.NotNil(t, harness.config)
	require.NotNil(t, harness.config.PostSetupExec)
	require.False(t, harness.config.PostSetupExec.UseTTY)
	require.Contains(t, harness.config.PostSetupExec.Command, "--experimental-json")

	// Verify schema path points into /src and prompt is final argument.
	cmd := harness.config.PostSetupExec.Command
	idx := indexOf(cmd, "--output-schema")
	require.NotEqual(t, -1, idx)
	require.True(t, strings.HasPrefix(cmd[idx+1], "/src/"))
	require.Equal(t, "do the task", cmd[len(cmd)-1])
	require.Contains(t, harness.config.PostSetupExec.Env, "CODEX_APPROVAL_POLICY")
	require.Equal(t, "never", harness.config.PostSetupExec.Env["CODEX_APPROVAL_POLICY"])
	require.Equal(t, []string{cellRel}, harness.config.ReadWritePaths)
	require.True(t, harness.runner.closed)

	require.Len(t, blob.calls, 1)
	call := blob.calls[0]
	require.Equal(t, "file://"+worktree, call.baseURI)
	require.True(t, strings.HasPrefix(call.relative, "codex/wf-123/20240102T030405Z/"))
	require.True(t, strings.HasSuffix(call.relative, ".jsonl"))
}

func TestExecuteIncompleteDependencies(t *testing.T) {
	worktree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(worktree, "cell"), 0o755))

	harness := &runnerHarness{
		lines: []string{
			`{"type":"session.created","session_id":"sess-456"}`,
			`{"type":"item.completed","item":{"id":"item_1","item_type":"assistant_message","text":"{\"status\":\"incomplete\",\"assistantSummary\":\"Needs follow-up\",\"incompleteReason\":\"update components\",\"incompleteCategory\":\"dependency_blockers\",\"pendingDependencies\":[{\"component\":\"server/git\",\"requestedChanges\":\"update refs\"}],\"errorMessage\":\"\" }"}}`,
		},
	}

	opts := Options{
		Prompt:           "analyze",
		WorktreeRoot:     worktree,
		CellRelativePath: "cell",
		BlobstoreURI:     "file://" + worktree,
		WorkflowID:       "wf",
		Clock:            fakeClock{ts: time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)},
		RunnerFactory:    harness.factory,
		BlobStore:        &fakeBlobStore{},
	}

	res, err := Execute(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, StatusIncomplete, res.Status)
	require.Equal(t, "Needs follow-up", res.AssistantSummary)
	require.Equal(t, "update components", res.IncompleteReason)
	require.Equal(t, "dependency_blockers", res.IncompleteCategory)
	require.Len(t, res.PendingDependencies, 1)
	require.Equal(t, "server/git", res.PendingDependencies[0].Component)
	require.Equal(t, "update refs", res.PendingDependencies[0].RequestedChanges)
}

func TestExecuteStructuredPayloadError(t *testing.T) {
	worktree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(worktree, "cell"), 0o755))

	harness := &runnerHarness{
		lines: []string{
			`{"type":"session.created","session_id":"sess-789"}`,
			`{"type":"item.completed","item":{"id":"item_1","item_type":"assistant_message","text":"{\"status\":\"completed\",\"pendingDependencies\":[],\"incompleteReason\":\"\",\"incompleteCategory\":\"\",\"errorMessage\":\"\"}"}}`,
		},
		stderr: []string{"warning: schema mismatch"},
	}

	blob := &fakeBlobStore{}
	opts := Options{
		Prompt:           "go",
		WorktreeRoot:     worktree,
		CellRelativePath: "cell",
		BlobstoreURI:     "file://" + worktree,
		WorkflowID:       "wf",
		Clock:            fakeClock{ts: time.Date(2024, 7, 8, 9, 10, 11, 0, time.UTC)},
		RunnerFactory:    harness.factory,
		BlobStore:        blob,
	}

	res, err := Execute(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, StatusError, res.Status)
	require.Contains(t, res.ErrorMessage, "assistant payload missing assistantSummary")
	require.Contains(t, res.Stderr, "warning: schema mismatch")
	require.Equal(t, "sess-789", res.SessionID)
	require.NotEmpty(t, res.StdoutBlobURI)
}

func TestExecuteRunError(t *testing.T) {
	worktree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(worktree, "cell"), 0o755))

	harness := &runnerHarness{
		lines: []string{
			`{"type":"session.created","session_id":"sess"}`,
			`{"type":"item.completed","item":{"id":"item_1","item_type":"assistant_message","text":"{\"status\":\"completed\",\"assistantSummary\":\"Done\",\"incompleteReason\":\"\",\"incompleteCategory\":\"\",\"pendingDependencies\":[],\"errorMessage\":\"\"}"}}`,
		},
		runErr: errors.New("codex exit 1"),
	}

	opts := Options{
		Prompt:           "task",
		WorktreeRoot:     worktree,
		CellRelativePath: "cell",
		BlobstoreURI:     "file://" + worktree,
		WorkflowID:       "wf",
		Clock:            fakeClock{ts: time.Date(2024, 9, 10, 11, 12, 13, 0, time.UTC)},
		RunnerFactory:    harness.factory,
		BlobStore:        &fakeBlobStore{},
	}

	res, err := Execute(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, StatusError, res.Status)
	require.Contains(t, res.ErrorMessage, "codex exit 1")
	require.Equal(t, "sess", res.SessionID)
}

func indexOf(haystack []string, needle string) int {
	for i, v := range haystack {
		if v == needle {
			return i
		}
	}
	return -1
}
