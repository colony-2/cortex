package squashrebasemerge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunSquashRebaseMerge_PushesSquashedCommit(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	remotePath := filepath.Join(tempDir, "remote.git")
	initBareRemote(t, remotePath)

	// Seed the remote with an initial commit on main.
	seedRepo := filepath.Join(tempDir, "seed")
	runGit(t, tempDir, "git", "clone", remotePath, "seed")
	configureAuthor(t, seedRepo, "Seed User", "seed@example.com")
	writeFile(t, seedRepo, "README.md", "base\n")
	runGit(t, seedRepo, "git", "add", "README.md")
	runGit(t, seedRepo, "git", "commit", "-m", "seed commit")
	runGit(t, seedRepo, "git", "checkout", "-B", "main")
	runGit(t, seedRepo, "git", "push", "origin", "HEAD:main")

	workspace := filepath.Join(tempDir, "workspace")
	runGit(t, tempDir, "git", "clone", remotePath, "workspace")
	configureAuthor(t, workspace, "Feature Dev", "feature@example.com")

	baseHash := revParse(t, workspace, "HEAD")

	writeFile(t, workspace, "feature.txt", "one\n")
	runGit(t, workspace, "git", "add", "feature.txt")
	runGit(t, workspace, "git", "commit", "-m", "feature part 1")

	writeFile(t, workspace, "feature.txt", "one\ntwo\n")
	runGit(t, workspace, "git", "add", "feature.txt")
	runGit(t, workspace, "git", "commit", "-m", "feature part 2")

	persistHash := revParse(t, workspace, "HEAD")

	input := SquashRebaseMergeInput{
		RepoPath:       workspace,
		TargetBranch:   "refs/heads/main",
		UpstreamRemote: "origin",
		Context: map[string]interface{}{
			"worktree": workspace,
			"git": map[string]interface{}{
				"base_hash":      baseHash,
				"persist_hash":   persistHash,
				"base_repo":      remotePath,
				"worktree_path":  workspace,
				"blob_store_uri": "file://" + filepath.ToSlash(tempDir),
				"git_author":     "Workflow Bot <bot@example.com>",
			},
		},
	}

	output, err := Run(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	require.Equal(t, "refs/heads/main", output.TargetBranch)
	require.NotEmpty(t, output.MergedHash)
	require.Equal(t, persistHash, output.SquashedCommits.PersistHash)
	require.Equal(t, baseHash, output.SquashedCommits.BaseHash)
	require.Equal(t, output.MergedHash, output.GitContextPatch["base_hash"])
	require.Equal(t, output.MergedHash, output.GitContextPatch["persist_hash"])

	// Only one commit should exist after the squash.
	commitCount := strings.TrimSpace(runGitOutput(t, workspace, "git", "rev-list", "--count", fmt.Sprintf("%s..HEAD", baseHash)))
	require.Equal(t, "1", commitCount)

	// Remote main should point to the merged hash.
	runGit(t, workspace, "git", "fetch", "origin", "main")
	remoteHead := revParse(t, workspace, "refs/remotes/origin/main")
	require.Equal(t, output.MergedHash, remoteHead)
}

func TestRunSquashRebaseMerge_NoChangesFastForwards(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	remotePath := filepath.Join(tempDir, "remote.git")
	initBareRemote(t, remotePath)

	seedRepo := filepath.Join(tempDir, "seed")
	runGit(t, tempDir, "git", "clone", remotePath, "seed")
	configureAuthor(t, seedRepo, "Seed", "seed@example.com")
	writeFile(t, seedRepo, "README.md", "base\n")
	runGit(t, seedRepo, "git", "add", "README.md")
	runGit(t, seedRepo, "git", "commit", "-m", "seed")
	runGit(t, seedRepo, "git", "checkout", "-B", "main")
	runGit(t, seedRepo, "git", "push", "origin", "HEAD:main")

	workspace := filepath.Join(tempDir, "workspace")
	runGit(t, tempDir, "git", "clone", remotePath, "workspace")
	configureAuthor(t, workspace, "Dev", "dev@example.com")

	baseHash := revParse(t, workspace, "HEAD")

	// Advance remote without local changes
	writeFile(t, seedRepo, "README.md", "base\nremote\n")
	runGit(t, seedRepo, "git", "add", "README.md")
	runGit(t, seedRepo, "git", "commit", "-m", "remote advance")
	runGit(t, seedRepo, "git", "push", "origin", "HEAD:main")
	remoteHead := revParse(t, seedRepo, "HEAD")

	input := SquashRebaseMergeInput{
		RepoPath:       workspace,
		TargetBranch:   "refs/heads/main",
		UpstreamRemote: "origin",
		Context: map[string]interface{}{
			"worktree": workspace,
			"git": map[string]interface{}{
				"base_hash":      baseHash,
				"persist_hash":   baseHash,
				"base_repo":      remotePath,
				"worktree_path":  workspace,
				"blob_store_uri": "file://" + filepath.ToSlash(tempDir),
			},
		},
	}

	output, err := Run(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, remoteHead, output.MergedHash)
	require.Equal(t, remoteHead, revParse(t, workspace, "HEAD"))
	require.Equal(t, remoteHead, output.GitContextPatch["base_hash"])
	require.Equal(t, remoteHead, output.GitContextPatch["persist_hash"])
}

func TestRunSquashRebaseMerge_RebaseConflict(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	remotePath := filepath.Join(tempDir, "remote.git")
	initBareRemote(t, remotePath)

	seedRepo := filepath.Join(tempDir, "seed")
	runGit(t, tempDir, "git", "clone", remotePath, "seed")
	configureAuthor(t, seedRepo, "Seed", "seed@example.com")
	writeFile(t, seedRepo, "shared.txt", "hello\n")
	runGit(t, seedRepo, "git", "add", "shared.txt")
	runGit(t, seedRepo, "git", "commit", "-m", "seed")
	runGit(t, seedRepo, "git", "checkout", "-B", "main")
	runGit(t, seedRepo, "git", "push", "origin", "HEAD:main")

	workspace := filepath.Join(tempDir, "workspace")
	runGit(t, tempDir, "git", "clone", remotePath, "workspace")
	configureAuthor(t, workspace, "Dev", "dev@example.com")

	baseHash := revParse(t, workspace, "HEAD")

	writeFile(t, workspace, "shared.txt", "hello\nlocal\n")
	runGit(t, workspace, "git", "add", "shared.txt")
	runGit(t, workspace, "git", "commit", "-m", "local change")
	persistHash := revParse(t, workspace, "HEAD")

	// Remote conflicting change
	writeFile(t, seedRepo, "shared.txt", "hello\nremote\n")
	runGit(t, seedRepo, "git", "add", "shared.txt")
	runGit(t, seedRepo, "git", "commit", "-m", "remote change")
	runGit(t, seedRepo, "git", "push", "origin", "main")

	input := SquashRebaseMergeInput{
		RepoPath:       workspace,
		TargetBranch:   "refs/heads/main",
		UpstreamRemote: "origin",
		Context: map[string]interface{}{
			"worktree": workspace,
			"git": map[string]interface{}{
				"base_hash":      baseHash,
				"persist_hash":   persistHash,
				"base_repo":      remotePath,
				"worktree_path":  workspace,
				"blob_store_uri": "file://" + filepath.ToSlash(tempDir),
			},
		},
	}

	_, err := Run(context.Background(), input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "git rebase failed")
}

func initBareRemote(t *testing.T, path string) {
	t.Helper()
	runGit(t, filepath.Dir(path), "git", "init", "--bare", filepath.Base(path))
	runGit(t, path, "git", "symbolic-ref", "HEAD", "refs/heads/main")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func configureAuthor(t *testing.T, dir, name, email string) {
	t.Helper()
	runGit(t, dir, "git", "config", "user.name", name)
	runGit(t, dir, "git", "config", "user.email", email)
}

func runGit(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, output)
	}
}

func runGitOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, output)
	}
	return string(output)
}

func revParse(t *testing.T, dir, ref string) string {
	return strings.TrimSpace(runGitOutput(t, dir, "git", "rev-parse", ref))
}
