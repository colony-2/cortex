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

func TestControllerLifecycle(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	ctx := Context{
		BaseRepo:       baseRepo,
		BaseHash:       baseHash,
		PersistHash:    baseHash,
		PreviousHash:   baseHash,
		WorktreePath:   worktree,
		BlobStoreURI:   "file://" + blobStore,
		TicketID:       "ticket-123",
		CellName:       "alpha",
		RecipeID:       "recipe.test",
		RecipeNode:     "node",
		WorkflowID:     "wf",
		WorkflowRunID:  "run",
		InvocationID:   "inv",
		InvocationHash: "invhash",
	}

	controller := NewController(nil)
	require.NoError(t, controller.PrepareWorkspace(context.Background(), &ctx))
	require.NoError(t, controller.Restore(context.Background(), &ctx))

	file := writeFile{Path: filepath.Join(worktree, "hello.txt"), Content: "hello"}
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

func TestBuildCommitMessage(t *testing.T) {
	ctx := Context{
		BaseRepo:          "/repo",
		BaseHash:          strings.Repeat("a", 40),
		PreviousHash:      strings.Repeat("b", 40),
		PersistHash:       strings.Repeat("c", 40),
		BlobStoreURI:      "file:///blob",
		ThinPackPath:      "git/thin-packs/cb-pack.pack",
		TicketID:          "TICK-1",
		CellName:          "alpha",
		RecipeID:          "recipe",
		RecipeNode:        "node",
		InvocationHash:    "invhash",
		InvocationID:      "invoke",
		InvocationAttempt: 2,
		BoxID:             "box",
		ActivityID:        "activity",
		WorkflowID:        "wf",
		WorkflowRunID:     "run",
	}
	message := buildCommitMessage(ctx, ctx.PersistHash, ctx.ThinPackPath)
	require.Contains(t, message, "Recipe recipe node node")
	require.Contains(t, message, "persist_hash: "+ctx.PersistHash)
	require.Contains(t, message, "thin_pack_path: "+ctx.ThinPackPath)
	require.Contains(t, message, "workflow:\n  id: wf\n  run_id: run")
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
