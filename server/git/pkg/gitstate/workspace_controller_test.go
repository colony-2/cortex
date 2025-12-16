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

func newTaskContext(baseRepo, baseRef, worktree, blobStore, cell string) *GitTaskContext {
	return &GitTaskContext{
		BaseRepo:         baseRepo,
		BaseRef:          baseRef,
		ResolvedBaseHash: baseRef,
		PersistHash:      baseRef,
		ParentHash:       baseRef,
		WorktreePath:     worktree,
		BlobStoreURI:     "file://" + blobStore,
		CellName:         cell,
		TicketID:         "ticket-123",
		NodePath:         "node",
		InvokeSeq:        1,
	}
}

func TestControllerLifecycle(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, blobStore, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	require.NoError(t, controller.Restore(context.Background(), ctx))

	file := writeFile{Path: filepath.Join(worktree, "cells", "alpha", "hello.txt"), Content: "hello"}
	require.NoError(t, os.WriteFile(file.Path, []byte(file.Content), 0o644))

	output, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CommitHash)
	require.NotEqual(t, baseHash, output.CommitHash)

	ctx.PersistHash = output.CommitHash
	ctx.ParentHash = output.ParentHash
	ctx.ThinPackPath = output.ThinPackPath

	head := gitRevParse(t, worktree, "HEAD")
	require.True(t, strings.HasPrefix(head, output.CommitHash[:7]))

	relativePack, err := filepath.Rel(blobStore, output.ThinPackPath)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(filepath.ToSlash(relativePack), "git/thin-packs"))

	info, err := os.Stat(output.ThinPackPath)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))

	restoredWorktree := filepath.Join(t.TempDir(), "restore")
	restoredCtx := *ctx
	restoredCtx.WorktreePath = restoredWorktree

	require.NoError(t, controller.prepareWorkspace(context.Background(), &restoredCtx))
	require.NoError(t, controller.Restore(context.Background(), &restoredCtx))
	restoredHead := gitRevParse(t, restoredWorktree, "HEAD")
	require.True(t, strings.HasPrefix(restoredHead, output.CommitHash[:7]))
}

func TestControllerPersistCleansOutsideCell(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, blobStore, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	require.NoError(t, controller.Restore(context.Background(), ctx))

	inside := filepath.Join(worktree, "cells", "alpha", "alpha.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(inside), 0o755))
	require.NoError(t, os.WriteFile(inside, []byte("alpha"), 0o644))

	outside := filepath.Join(worktree, "rogue.txt")
	require.NoError(t, os.WriteFile(outside, []byte("rogue"), 0o644))

	output, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CommitHash)

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

	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := newTaskContext(baseRepo, baseHash, worktree, blobStore, "cells/alpha")

	controller := NewController(nil)
	require.NoError(t, controller.prepareWorkspace(context.Background(), ctx))
	require.NoError(t, controller.Restore(context.Background(), ctx))

	inside := filepath.Join(worktree, "cells", "alpha", "alpha.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(inside), 0o755))
	require.NoError(t, os.WriteFile(inside, []byte("alpha"), 0o644))

	output, err := controller.Persist(context.Background(), ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CommitHash)

	stray := filepath.Join(worktree, "stray.txt")
	require.NoError(t, os.WriteFile(stray, []byte("stray"), 0o644))
	statBefore, err := os.Stat(stray)
	require.NoError(t, err)
	require.False(t, statBefore.IsDir())

	ctx.PersistHash = output.CommitHash
	ctx.ParentHash = output.ParentHash
	ctx.ThinPackPath = output.ThinPackPath

	require.NoError(t, controller.Restore(context.Background(), ctx))

	_, err = os.Stat(stray)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	status := strings.TrimSpace(runGitOutput(t, worktree, "git", "status", "--porcelain"))
	require.Equal(t, "", status)
}

func TestBuildCommitMessage(t *testing.T) {
	ctx := &GitTaskContext{
		BaseRepo:         "/repo",
		BaseRef:          "main",
		ResolvedBaseHash: strings.Repeat("a", 40),
		ParentHash:       strings.Repeat("b", 40),
		PersistHash:      strings.Repeat("c", 40),
		BlobStoreURI:     "file:///blob",
		ThinPackPath:     "git/thin-packs/cb-pack.pack",
		TicketID:         "TICK-1",
		CellName:         "cells/alpha",
		NodePath:         "cells/alpha/op",
		InvokeSeq:        3,
	}
	message := buildCommitMessage(ctx, ctx.PersistHash, ctx.ThinPackPath)
	require.Contains(t, message, ctx.ResolvedBaseHash)
	require.Contains(t, message, ctx.ParentHash)
	require.Contains(t, message, ctx.PersistHash)
	require.Contains(t, message, ctx.BlobStoreURI)
	require.Contains(t, message, ctx.ThinPackPath)
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
