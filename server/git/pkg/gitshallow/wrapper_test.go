package gitshallow

import (
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func TestGitShallowActivityWrapper(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "git-wrapper-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test git repository
	sourceDir := filepath.Join(tempDir, "source-repo")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user for the test repo
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to configure git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to configure git user name: %v", err)
	}

	// Create a commit
	testFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Test commit")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Get the commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = sourceDir
	commitHashBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get commit hash: %v", err)
	}
	commitHash := string(commitHashBytes[:len(commitHashBytes)-1]) // Remove newline

    t.Run("test metadata", func(t *testing.T) {
        wrapper := NewGitShallowActivity()
        metadata := wrapper.GetMetadata()

        if metadata.Type != "git_shallow_clone" {
            t.Errorf("Expected type 'git_shallow_clone', got '%s'", metadata.Type)
        }

        if metadata.Version != "1.0.0" {
            t.Errorf("Expected version '1.0.0', got '%s'", metadata.Version)
        }

        if metadata.Description == "" {
            t.Error("Expected description to be set")
        }
    })

	t.Run("test execute", func(t *testing.T) {
		wrapper := NewGitShallowActivity()
		targetDir := filepath.Join(tempDir, "target-wrapper")

        input := GitShallowInput{
            SourceDir:  sourceDir,
            TargetDir:  targetDir,
            CommitHash: commitHash,
        }

        output, err := wrapper.Execute(ctx, input)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if output.ClonedPath != targetDir {
			t.Errorf("Expected cloned path %s, got %s", targetDir, output.ClonedPath)
		}

		// Verify the clone exists
		if _, err := os.Stat(targetDir); err != nil {
			t.Errorf("Cloned directory not found: %v", err)
		}

		// Verify the test file exists in the clone
		clonedTestFile := filepath.Join(targetDir, "test.txt")
		if _, err := os.Stat(clonedTestFile); err != nil {
			t.Errorf("Test file not found in clone: %v", err)
		}
	})

	t.Run("test interface implementation", func(t *testing.T) {
        // This test ensures the wrapper properly implements the RegisterableOp interface
        wrapper := &GitShallowActivityWrapper{}

        // Test that we can call all interface methods
        _ = wrapper.GetMetadata()
        input := GitShallowInput{
            SourceDir:  sourceDir,
            TargetDir:  filepath.Join(tempDir, "interface-test"),
            CommitHash: commitHash,
        }

		_, err := wrapper.Execute(ctx, input)
		if err != nil {
			t.Fatalf("Interface method Execute failed: %v", err)
		}
	})
}
