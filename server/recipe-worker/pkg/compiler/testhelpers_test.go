package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/divisive-ai/vibethis/server/git/pkg/gitstate"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
)

var (
	testRepoOnce sync.Once
	testRepoPath string
	testRepoHash string
)

func ensureTestRepo() (string, string) {
	testRepoOnce.Do(func() {
		dir, err := os.MkdirTemp("", "recipe-worker-test-repo-*")
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
		cells := []string{"cells/test-cell", "cells/alpha", "cells/beta", "cells/cell-a"}
		for _, rel := range cells {
			full := filepath.Join(dir, rel)
			if err := os.MkdirAll(full, 0o755); err != nil {
				panic(err)
			}
			seed := filepath.Join(full, "README.md")
			if err := os.WriteFile(seed, []byte(rel+"\n"), 0o644); err != nil {
				panic(err)
			}
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
		testRepoPath = dir
		testRepoHash = strings.TrimSpace(string(output))
	})
	return testRepoPath, testRepoHash
}

// withRequiredGitInputs seeds both the legacy input map and a typed execution context for tests.
func withRequiredGitInputs(inputs map[string]interface{}) (map[string]interface{}, ExecutionContext) {
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	baseRepo, baseHash := ensureTestRepo()
	if _, ok := inputs["basegitrepo"]; !ok {
		inputs["basegitrepo"] = baseRepo
	}
	if _, ok := inputs["basegithash"]; !ok {
		inputs["basegithash"] = baseHash
	}
	if _, ok := inputs["ticketid"]; !ok {
		inputs["ticketid"] = "TEST-TICKET"
	}
	if _, ok := inputs["cellname"]; !ok {
		inputs["cellname"] = "cells/test-cell"
	}
	worktree, err := os.MkdirTemp("", "vibethis-worktree-*")
	if err != nil {
		panic(err)
	}
	blobDir, err := os.MkdirTemp("", "vibethis-blobstore-*")
	if err != nil {
		panic(err)
	}
	blobStoreURI := "file://" + blobDir

	actor := contextual.ActorContext{
		TicketID: "TEST-TICKET",
		CellName: "cells/test-cell",
	}
	environment := contextual.EnvironmentContext{
		WorktreePath: worktree,
		BlobStoreURI: blobStoreURI,
	}
	gitCtx := gitstate.Context{
		ActorContext:       actor,
		EnvironmentContext: environment,
		GitSnapshotContext: contextual.GitSnapshotContext{
			BaseRepo:    baseRepo,
			BaseHash:    baseHash,
			PersistHash: baseHash,
		},
	}

	execCtx := ExecutionContext{
		Invocation:  contextual.InvocationContext{},
		Actor:       actor,
		Environment: environment,
		Git:         gitCtx,
		Extra:       nil,
	}
	return inputs, execCtx
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
