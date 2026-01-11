package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type writeFile struct {
	Path    string
	Content string
}

func newTaskContext(baseRepo, baseRef, worktree, cell string) *GitTaskContext {
	return &GitTaskContext{
		BaseRepo:         baseRepo,
		BaseRef:          baseRef,
		ResolvedBaseHash: baseRef,
		PersistHash:      baseRef,
		ParentHash:       baseRef,
		WorktreePath:     worktree,
		CellName:         cell,
		CellPath:         cell,
		TicketID:         "ticket-123",
		NodePath:         "node",
		InvokeSeq:        1,
	}
}

func TestControllerLifecycle(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	// Initial restore with no thin pack artifact
	require.NoError(t, controller.Restore(context.Background(), ctx, nil))

	file := writeFile{Path: filepath.Join(worktree, "cells", "alpha", "hello.txt"), Content: "hello"}
	require.NoError(t, os.WriteFile(file.Path, []byte(file.Content), 0o644))

	// Persist returns output metadata and artifact
	output, artifact, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, artifact)
	require.Equal(t, "__git_state_thin_pack__", artifact.Name())

	// Check output metadata
	require.True(t, output.HasChanges)
	require.NotEmpty(t, output.CommitHash)
	require.NotEqual(t, baseHash, output.CommitHash)
	require.NotEmpty(t, output.ParentHash)
	require.NotEmpty(t, output.ThinPackPath)

	// Check that context was updated
	require.NotEmpty(t, ctx.PersistHash)
	require.NotEqual(t, baseHash, ctx.PersistHash)
	require.Equal(t, output.CommitHash, ctx.PersistHash)
	require.Equal(t, output.ParentHash, ctx.ParentHash)

	head := gitRevParse(t, worktree, "HEAD")
	require.True(t, strings.HasPrefix(head, ctx.PersistHash[:7]))

	// Create a new worktree and restore using the artifact
	restoredWorktree := filepath.Join(t.TempDir(), "restore")
	restoredCtx := *ctx
	restoredCtx.WorktreePath = restoredWorktree
	// Reset ResolvedBaseHash since the new worktree will be cloned fresh at base
	restoredCtx.ResolvedBaseHash = baseHash

	require.NoError(t, controller.prepareWorkspace(context.Background(), &restoredCtx))
	// Restore with the thin pack artifact
	require.NoError(t, controller.Restore(context.Background(), &restoredCtx, artifact))
	restoredHead := gitRevParse(t, restoredWorktree, "HEAD")
	require.True(t, strings.HasPrefix(restoredHead, ctx.PersistHash[:7]))
}

func TestControllerPersistCleansOutsideCell(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	require.NoError(t, controller.Restore(context.Background(), ctx, nil))

	inside := filepath.Join(worktree, "cells", "alpha", "alpha.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(inside), 0o755))
	require.NoError(t, os.WriteFile(inside, []byte("alpha"), 0o644))

	outside := filepath.Join(worktree, "rogue.txt")
	require.NoError(t, os.WriteFile(outside, []byte("rogue"), 0o644))

	output, artifact, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, artifact)

	// Check output metadata
	require.True(t, output.HasChanges)
	require.NotEmpty(t, output.CommitHash)
	require.NotEmpty(t, output.ParentHash)
	require.NotEmpty(t, output.ThinPackPath)

	// Check that context was updated
	require.NotEmpty(t, ctx.PersistHash)
	require.Equal(t, output.CommitHash, ctx.PersistHash)
	require.Equal(t, output.ParentHash, ctx.ParentHash)

	_, err = os.Stat(outside)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	status := strings.TrimSpace(runGitOutput(t, worktree, "git", "status", "--porcelain"))
	require.Equal(t, "", status)

	show := runGitOutput(t, worktree, "git", "show", "--name-only", "--pretty=format:")
	require.Contains(t, show, "cells/alpha/alpha.txt")
	require.NotContains(t, show, "rogue.txt")
}

func TestControllerRestoreCleansOutsideCell(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	require.NoError(t, controller.Restore(context.Background(), ctx, nil))

	inside := filepath.Join(worktree, "cells", "alpha", "alpha.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(inside), 0o755))
	require.NoError(t, os.WriteFile(inside, []byte("alpha"), 0o644))

	output, artifact, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, artifact)

	// Check output metadata
	require.True(t, output.HasChanges)
	require.NotEmpty(t, output.CommitHash)
	require.NotEmpty(t, output.ParentHash)
	require.NotEmpty(t, output.ThinPackPath)

	// Check that context was updated
	require.NotEmpty(t, ctx.PersistHash)
	require.Equal(t, output.CommitHash, ctx.PersistHash)
	require.Equal(t, output.ParentHash, ctx.ParentHash)

	stray := filepath.Join(worktree, "stray.txt")
	require.NoError(t, os.WriteFile(stray, []byte("stray"), 0o644))
	statBefore, err := os.Stat(stray)
	require.NoError(t, err)
	require.False(t, statBefore.IsDir())

	// Restore with the thin pack artifact
	require.NoError(t, controller.Restore(context.Background(), ctx, artifact))

	_, err = os.Stat(stray)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	status := strings.TrimSpace(runGitOutput(t, worktree, "git", "status", "--porcelain"))
	require.Equal(t, "", status)
}

func TestPersistReturnsNilWhenNoChanges(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	require.NoError(t, controller.Restore(context.Background(), ctx, nil))

	// Don't make any changes

	output, artifact, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output, "output should not be nil even when there are no changes")
	require.False(t, output.HasChanges, "output should indicate no changes")
	require.Nil(t, artifact, "artifact should be nil when there are no changes")
	require.Empty(t, ctx.PersistHash, "persist hash should be empty when there are no changes")
}

func TestRestore_WorkspaceAlreadyAtTarget(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, "cells/alpha")
	ctx.PersistHash = baseHash // Set target to current state

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))

	// Even though we pass nil artifact, restore should succeed because we're already at target
	require.NoError(t, controller.Restore(context.Background(), ctx, nil))

	head := gitRevParse(t, worktree, "HEAD")
	require.True(t, strings.HasPrefix(head, baseHash[:7]))
}

func TestRestore_NilThinPackArtifactError(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, "cells/alpha")
	// Set persist hash to a different commit (that doesn't exist yet)
	ctx.PersistHash = "1234567890abcdef"

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))

	// Restore should fail because we need to restore but have no artifact
	err := controller.Restore(context.Background(), ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "thin pack artifact required")
}

func TestBuildCommitMessage(t *testing.T) {
	ctx := &GitTaskContext{
		BaseRepo:         "/repo",
		BaseRef:          "main",
		ResolvedBaseHash: strings.Repeat("a", 40),
		ParentHash:       strings.Repeat("b", 40),
		PersistHash:      strings.Repeat("c", 40),
		ThinPackPath:     "git/thin-packs/cb-pack.pack",
		TicketID:         "TICK-1",
		CellName:         "cells/alpha",
		CellPath:         "cells/alpha",
		NodePath:         "cells/alpha/op",
		InvokeSeq:        3,
	}
	message := buildCommitMessage(ctx, ctx.PersistHash, ctx.ThinPackPath)
	require.Contains(t, message, ctx.ResolvedBaseHash)
	require.Contains(t, message, ctx.ParentHash)
	require.Contains(t, message, ctx.PersistHash)
	require.Contains(t, message, ctx.TicketID)
	require.Contains(t, message, ctx.CellName)
	require.Contains(t, message, ctx.NodePath)
}

func setupGitRepo(t *testing.T) (string, string, func()) {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "base")
	require.NoError(t, os.MkdirAll(repoPath, 0o755))
	runGit(t, dir, "git", "init", "base")
	runGit(t, repoPath, "git", "config", "user.email", "test@example.com")
	runGit(t, repoPath, "git", "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("initial\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "cells", "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "cells", "beta"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "cells", "alpha", "README.md"), []byte("alpha\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "cells", "beta", "README.md"), []byte("beta\n"), 0o644))
	runGit(t, repoPath, "git", "add", ".")
	runGit(t, repoPath, "git", "commit", "-m", "init")
	head := gitRevParse(t, repoPath, "HEAD")
	return repoPath, head, func() { os.RemoveAll(dir) }
}

func gitRevParse(t *testing.T, dir string, ref string) string {
	out := runGitOutput(t, dir, "git", "rev-parse", ref)
	return strings.TrimSpace(out)
}

func runGit(t *testing.T, dir string, name string, args ...string) {
	runGitOutput(t, dir, name, args...)
}

func runGitOutput(t *testing.T, dir string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, output)
	}
	return string(output)
}

func TestResolveScopePath_RequiresCellPath(t *testing.T) {
	ctrl := &Controller{}
	ctx := context.Background()

	t.Run("empty CellPath returns error", func(t *testing.T) {
		task := &GitTaskContext{CellPath: ""}
		_, err := ctrl.resolveScopePath(ctx, task)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cell_path is required")
	})

	t.Run("valid CellPath returns sanitized path", func(t *testing.T) {
		task := &GitTaskContext{CellPath: "cells/my-cell"}
		scope, err := ctrl.resolveScopePath(ctx, task)
		require.NoError(t, err)
		require.Equal(t, "cells/my-cell", scope)
	})

	t.Run("path with trailing slash is sanitized", func(t *testing.T) {
		task := &GitTaskContext{CellPath: "cells/my-cell/"}
		scope, err := ctrl.resolveScopePath(ctx, task)
		require.NoError(t, err)
		require.Equal(t, "cells/my-cell", scope)
	})

	t.Run("path traversal is rejected", func(t *testing.T) {
		task := &GitTaskContext{CellPath: "cells/../other"}
		_, err := ctrl.resolveScopePath(ctx, task)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cell path")
	})

	t.Run("absolute path with slash is rejected", func(t *testing.T) {
		task := &GitTaskContext{CellPath: "/cells/my-cell"}
		_, err := ctrl.resolveScopePath(ctx, task)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cell path")
	})

	t.Run("only slashes is rejected as absolute", func(t *testing.T) {
		task := &GitTaskContext{CellPath: "///"}
		_, err := ctrl.resolveScopePath(ctx, task)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cell path")
	})
}
