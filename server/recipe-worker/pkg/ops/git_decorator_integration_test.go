package ops_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/contextual"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

func TestGitDecoratorPersistsAcrossActivities(t *testing.T) {
	yamlSpec := `id: git_persist
version: "1.0"
sequence:
  - id: write
    op: command_execution
    inputs:
      run: |
        mkdir -p cells/test-cell
        echo first >> cells/test-cell/README.md
      working_directory: '{{ context.git.worktree_path }}'
  - id: append
    op: command_execution
    inputs:
      run: |
        mkdir -p cells/test-cell
        echo second >> cells/test-cell/README.md
      working_directory: '{{ context.git.worktree_path }}'
outputs:
  first_hash: '{{ sequence.write.outputs.git_persist_hash }}'
  second_hash: '{{ sequence.append.outputs.git_persist_hash }}'
  context_hash: '{{ sequence.append.outputs.context.git.persist_hash }}'
`

	var r recipe.Recipe
	if err := yaml.Unmarshal([]byte(yamlSpec), &r); err != nil {
		t.Fatalf("failed to parse recipe: %v", err)
	}

	repoPath, baseHash, cleanup := createTempRepo(t)
	defer cleanup()

	blobStore := t.TempDir()
	blobStoreURI := "file://" + filepath.ToSlash(blobStore)
	persistWorktree := filepath.Join(t.TempDir(), "persist-worktree")

	inputs := map[string]interface{}{
		"basegitrepo": repoPath,
		"basegithash": baseHash,
		"ticketid":    "TEST-TICKET",
		"cellname":    "cells/test-cell",
		"ticket_id":   "TEST-TICKET",
		"cell_name":   "cells/test-cell",
	}
	actor := contextual.ActorContext{
		TicketID: "TEST-TICKET",
		CellName: "cells/test-cell",
	}
	environment := contextual.EnvironmentContext{
		WorktreePath: persistWorktree,
		BlobStoreURI: blobStoreURI,
	}
	gitCtx := gitstate.Context{
		ActorContext:       actor,
		EnvironmentContext: environment,
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:    repoPath,
			BaseHash:    baseHash,
			PersistHash: baseHash,
		},
	}
	execCtx := compiler.ExecutionContext{
		Actor:       actor,
		Environment: environment,
		Git:         gitCtx,
	}

	registry, err := ops.NewActivityRegistry()
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	exec, err := executor.NewStandaloneExecutor(registry, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	outputs, err := exec.Execute(context.Background(), r, inputs, execCtx)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	firstHash, _ := outputs["first_hash"].(string)
	secondHash, _ := outputs["second_hash"].(string)
	contextHash, _ := outputs["context_hash"].(string)

	if firstHash == "" {
		t.Fatalf("expected first_hash to be set")
	}
	if secondHash == "" {
		t.Fatalf("expected second_hash to be set")
	}
	if contextHash != secondHash {
		t.Fatalf("expected context hash to match second hash; got %q vs %q", contextHash, secondHash)
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
	command := exec.Command(cmd, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v (%s)", append([]string{cmd}, args...), err, output)
	}
}

func runGitOutput(t *testing.T, dir string, cmd string, args ...string) string {
	command := exec.Command(cmd, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", append([]string{cmd}, args...), err, output)
	}
	return string(output)
}
