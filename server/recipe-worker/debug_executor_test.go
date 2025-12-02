package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

var (
	debugRepoOnce sync.Once
	debugRepoPath string
	debugRepoHash string
)

func ensureDebugRepo() (string, string) {
	debugRepoOnce.Do(func() {
		dir, err := os.MkdirTemp("", "debug-repo-*")
		if err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "init"); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "config", "user.email", "test@example.com"); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "config", "user.name", "Test User"); err != nil {
			panic(err)
		}
		readme := filepath.Join(dir, "README.md")
		if err := os.WriteFile(readme, []byte("initial\n"), 0o644); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "add", "."); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "commit", "-m", "init"); err != nil {
			panic(err)
		}
		output, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
		if err != nil {
			panic(fmt.Errorf("rev-parse HEAD failed: %w (%s)", err, output))
		}
		debugRepoPath = dir
		debugRepoHash = strings.TrimSpace(string(output))
	})
	return debugRepoPath, debugRepoHash
}

func runGit(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v failed: %w (%s)", args, err, out)
	}
	return nil
}

func TestDebugExecutor(t *testing.T) {
	yamlStr := `id: hello_world
desc: A minimal recipe that demonstrates basic structure  
version: "1.0"
op: echo_activity
inputs:
  message: "Hello, World!"`

	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(yamlStr), &r)
	if err != nil {
		t.Fatalf("Parse Error: %v", err)
	}

	t.Logf("Recipe ID: %s", r.GetMetdata().ID)

	// Create executor
	logger := zaptest.NewLogger(t)
	a, err := ops.NewActivityRegistry()
	if err != nil {
		t.Fatalf("Registry Error: %v", err)
	}

	exec, err := executor.NewStandaloneExecutor(a, logger)
	if err != nil {
		t.Fatalf("Executor Error: %v", err)
	}

	repo, hash := ensureDebugRepo()
	worktree := filepath.Join(os.TempDir(), "debug-executor-worktree")
	blobStore := "file://" + filepath.Join(os.TempDir(), "debug-executor-blobstore")
	actor := contextual.ActorContext{
		TicketID: "TEST-TICKET",
		CellName: "cells/test-cell",
	}
	envCtx := contextual.EnvironmentContext{
		WorktreePath: worktree,
		BlobStoreURI: blobStore,
	}
	gitCtx := gitstate.Context{
		ActorContext:       actor,
		EnvironmentContext: envCtx,
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:    repo,
			BaseHash:    hash,
			PersistHash: hash,
		},
	}
	execCtx := compiler.ExecutionContext{
		Actor:       actor,
		Environment: envCtx,
		Git:         gitCtx,
	}

	// Execute recipe
	result, err := exec.Execute(
		context.Background(),
		r,
		map[string]interface{}{
			"name":        "Test",
			"basegitrepo": repo,
			"basegithash": hash,
			"ticketid":    "TEST-TICKET",
			"cellname":    "cells/test-cell",
		},
		execCtx,
	)

	if err != nil {
		t.Fatalf("Execution Error: %v", err)
	}

	t.Logf("Result: %v", result)
}
