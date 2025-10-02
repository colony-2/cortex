package shared

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requiredWorkflowInputs(t *testing.T) map[string]interface{} {
	t.Helper()

	repoDir := t.TempDir()

	run := func(args ...string) {
		t.Helper()

		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_TERMINAL_PROMPT=0",
		)

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v (%s)", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test User")

	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("test repo\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = repoDir
	hashCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	hashOut, err := hashCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v (%s)", err, hashOut)
	}

	baseHash := string(hashOut)
	if len(baseHash) > 0 && baseHash[len(baseHash)-1] == '\n' {
		baseHash = baseHash[:len(baseHash)-1]
	}

	return map[string]interface{}{
		"basegitrepo": repoDir,
		"basegithash": baseHash,
		"ticketid":    "TEST-TICKET",
		"cellname":    "test-cell",
	}
}
