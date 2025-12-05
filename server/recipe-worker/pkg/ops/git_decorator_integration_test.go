package ops_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

func TestGitDecoratorPersistsAcrossActivities(t *testing.T) {
	repoPath, baseHash, cleanup := createTempRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	blobStoreURI := "file://" + filepath.ToSlash(blobStore)
	persistWorktree := filepath.Join(t.TempDir(), "persist-worktree")

	yamlSpec := fmt.Sprintf(`id: git_persist
version: "1.0"
sequence:
  - id: write
    op: command_execution
    inputs:
      run: |
        mkdir -p cells/test-cell
        echo first >> cells/test-cell/README.md
      working_directory: %q
  - id: append
    op: command_execution
    inputs:
      run: |
        mkdir -p cells/test-cell
        echo second >> cells/test-cell/README.md
      working_directory: %q
outputs:
  status: ok
`, persistWorktree, persistWorktree)

	var r recipe.Recipe
	if err := yaml.Unmarshal([]byte(yamlSpec), &r); err != nil {
		t.Fatalf("failed to parse recipe: %v", err)
	}

	inputs := map[string]interface{}{}
	jobCtx := contextual.JobContext{
		Actor: contextual.ActorContext{
			TicketID:   "TEST-TICKET",
			ActorName:  "test-actor",
			ActorEmail: "test-actor@vibethis",
		},
		Environment: contextual.EnvironmentContext{
			WorktreePath: persistWorktree,
			BlobStoreURI: blobStoreURI,
		},
		Workflow: contextual.WorkflowContext{
			CellName: "",
			JobID:    "git-decorator-test",
		},
		GitBase: contextual.GitBaseContext{
			BaseRepo: repoPath,
			BaseHash: baseHash,
			GitAuthor: "Test User <test@example.com>",
		},
	}
	gitCtx := contextual.GitCommitContext{
		PersistHash: "",
		ParentHash:  baseHash,
	}

	registry, err := ops.NewActivityRegistry()
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	exec, err := executor.NewStandaloneExecutor(coreops.NewServiceDepsBuilder().Build(), registry, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	_, err = exec.Execute(context.Background(), r, inputs, jobCtx, gitCtx)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	head := strings.TrimSpace(runGitOutput(t, persistWorktree, "git", "rev-parse", "HEAD"))
	if head == "" || head == baseHash {
		t.Fatalf("expected HEAD to advance, got %q", head)
	}
	data := strings.TrimSpace(runGitOutput(t, persistWorktree, "cat", "cells/test-cell/README.md"))
	if !strings.Contains(data, "first") || !strings.Contains(data, "second") {
		t.Fatalf("expected appended data in README, got %q", data)
	}
}

func createTempRepo(t *testing.T) (string, string, func()) {
	t.Helper()
	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	runGit(t, baseDir, "git", "init", "repo")
	runGit(t, repoDir, "git", "config", "user.email", "test@example.com")
	runGit(t, repoDir, "git", "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "cells", "test-cell"), 0o755); err != nil {
		t.Fatalf("create cell dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "cells", "test-cell", "README.md"), []byte("cell\n"), 0o644); err != nil {
		t.Fatalf("write cell seed: %v", err)
	}
	runGit(t, repoDir, "git", "add", ".")
	runGit(t, repoDir, "git", "commit", "-m", "init")

	baseHash := strings.TrimSpace(runGitOutput(t, repoDir, "git", "rev-parse", "HEAD"))
	return repoDir, baseHash, func() { os.RemoveAll(baseDir) }
}

func runGit(t *testing.T, dir string, cmd string, args ...string) {
	t.Helper()
	command := exec.Command(cmd, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git command failed: %v, output: %s", err, out)
	}
}

func runGitOutput(t *testing.T, dir string, cmd string, args ...string) string {
	t.Helper()
	command := exec.Command(cmd, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git command failed: %v, output: %s", err, out)
	}
	return string(out)
}
