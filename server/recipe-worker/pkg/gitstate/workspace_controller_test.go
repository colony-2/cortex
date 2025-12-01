package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/contextual"
	"github.com/stretchr/testify/require"
)

type writeFile struct {
	Path    string
	Content string
}

func TestControllerLifecycle(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := Context{
		InvocationContext: contextual.InvocationContext{
			RecipeID:       "recipe.test",
			NodePath:       "node",
			InvocationID:   "inv",
			InvocationHash: "invhash",
		},
		ActorContext: contextual.ActorContext{
			TicketID: "ticket-123",
			CellName: "cells/alpha",
		},
		EnvironmentContext: contextual.EnvironmentContext{
			WorktreePath: worktree,
			BlobStoreURI: "file://" + blobStore,
		},
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:     baseRepo,
			BaseHash:     baseHash,
			PersistHash:  baseHash,
			PreviousHash: baseHash,
		},
		Workflow: contextual.WorkflowEnvelope{JobID: swf.JobId("job-1")},
	}

	controller := NewController(nil)
	require.NoError(t, controller.PrepareWorkspace(context.Background(), &ctx))
	require.NoError(t, controller.Restore(context.Background(), &ctx))

	file := writeFile{Path: filepath.Join(worktree, "cells", "alpha", "hello.txt"), Content: "hello"}
	require.NoError(t, os.WriteFile(file.Path, []byte(file.Content), 0o644))

	newHash, updatedCtx, err := controller.Persist(context.Background(), &ctx)
	require.NoError(t, err)
	require.NotEmpty(t, newHash)
	require.NotEqual(t, baseHash, newHash)

	outputs := map[string]interface{}{}
	InjectPersistResult(outputs, newHash, updatedCtx)

	gitMap := outputs["context"].(map[string]interface{})["git"].(map[string]interface{})
	require.Equal(t, newHash, gitMap["persist_hash"].(string))

	head := gitRevParse(t, worktree, "HEAD")
	require.True(t, strings.HasPrefix(head, newHash[:7]))

	thinPackPath := gitMap["thin_pack_path"].(string)
	require.True(t, strings.HasPrefix(thinPackPath, "git/thin-packs"))
	packAbs := filepath.Join(blobStore, filepath.FromSlash(thinPackPath))
	info, err := os.Stat(packAbs)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))

	restoredWorktree := filepath.Join(t.TempDir(), "restore")
	restoredCtx := updatedCtx
	restoredCtx.WorktreePath = restoredWorktree
	restoredCtx.WorkspacePrepared = false

	require.NoError(t, controller.PrepareWorkspace(context.Background(), &restoredCtx))
	require.NoError(t, controller.Restore(context.Background(), &restoredCtx))
	restoredHead := gitRevParse(t, restoredWorktree, "HEAD")
	require.True(t, strings.HasPrefix(restoredHead, newHash[:7]))
}

func TestControllerPersistCleansOutsideCell(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := Context{
		EnvironmentContext: contextual.EnvironmentContext{
			WorktreePath: worktree,
			BlobStoreURI: "file://" + blobStore,
		},
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:     baseRepo,
			BaseHash:     baseHash,
			PersistHash:  baseHash,
			PreviousHash: baseHash,
		},
		ActorContext: contextual.ActorContext{
			CellName: "cells/alpha",
		},
	}

	controller := NewController(nil)
	require.NoError(t, controller.PrepareWorkspace(context.Background(), &ctx))
	require.NoError(t, controller.Restore(context.Background(), &ctx))

	inside := filepath.Join(worktree, "cells", "alpha", "alpha.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(inside), 0o755))
	require.NoError(t, os.WriteFile(inside, []byte("alpha"), 0o644))

	outside := filepath.Join(worktree, "rogue.txt")
	require.NoError(t, os.WriteFile(outside, []byte("rogue"), 0o644))

	newHash, _, err := controller.Persist(context.Background(), &ctx)
	require.NoError(t, err)
	require.NotEmpty(t, newHash)

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

	ctx := Context{
		EnvironmentContext: contextual.EnvironmentContext{
			WorktreePath: worktree,
			BlobStoreURI: "file://" + blobStore,
		},
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:     baseRepo,
			BaseHash:     baseHash,
			PersistHash:  baseHash,
			PreviousHash: baseHash,
		},
		ActorContext: contextual.ActorContext{
			CellName: "cells/alpha",
		},
	}

	controller := NewController(nil)
	require.NoError(t, controller.PrepareWorkspace(context.Background(), &ctx))
	require.NoError(t, controller.Restore(context.Background(), &ctx))

	inside := filepath.Join(worktree, "cells", "alpha", "alpha.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(inside), 0o755))
	require.NoError(t, os.WriteFile(inside, []byte("alpha"), 0o644))

	newHash, updatedCtx, err := controller.Persist(context.Background(), &ctx)
	require.NoError(t, err)
	require.NotEmpty(t, newHash)

	stray := filepath.Join(worktree, "stray.txt")
	require.NoError(t, os.WriteFile(stray, []byte("stray"), 0o644))
	statBefore, err := os.Stat(stray)
	require.NoError(t, err)
	require.False(t, statBefore.IsDir())

	updatedCtx.PersistHash = newHash
	require.NoError(t, controller.Restore(context.Background(), &updatedCtx))

	_, err = os.Stat(stray)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	status := strings.TrimSpace(runGitOutput(t, worktree, "git", "status", "--porcelain"))
	require.Equal(t, "", status)
}

func TestBuildCommitMessage(t *testing.T) {
	ctx := Context{
		InvocationContext: contextual.InvocationContext{
			RecipeID:          "recipe",
			NodePath:          "node",
			InvocationHash:    "invhash",
			InvocationID:      "invoke",
			InvocationAttempt: 2,
			BoxID:             "box",
			ActivityID:        "activity",
			JobID:             swf.JobId("job-123"),
		},
		ActorContext: contextual.ActorContext{
			TicketID: "TICK-1",
			CellName: "cells/alpha",
		},
		EnvironmentContext: contextual.EnvironmentContext{
			BlobStoreURI: "file:///blob",
			ThinPackPath: "git/thin-packs/cb-pack.pack",
		},
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:     "/repo",
			BaseHash:     strings.Repeat("a", 40),
			PreviousHash: strings.Repeat("b", 40),
			PersistHash:  strings.Repeat("c", 40),
		},
	}
	message := buildCommitMessage(ctx, ctx.PersistHash, ctx.ThinPackPath)
	require.Contains(t, message, "Recipe recipe node node")
	require.Contains(t, message, "persist_hash: "+ctx.PersistHash)
	require.Contains(t, message, "thin_pack_path: "+ctx.ThinPackPath)
	require.Contains(t, message, "job:\n  id: job-123")
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
