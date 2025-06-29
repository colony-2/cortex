package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitStatus(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test node directory
	nodeDir := filepath.Join(tempDir, "test-node")
	err = os.Mkdir(nodeDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = nodeDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = nodeDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = nodeDir
	cmd.Run()

	// Create repository instance
	repo := New("test@example.com", "Test User")
	ctx := context.Background()

	// Test clean repository
	status, err := repo.GetStatus(ctx, nodeDir)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if !status.Clean {
		t.Error("Expected clean repository")
	}

	if len(status.Files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(status.Files))
	}

	// Create a test file
	testFile := filepath.Join(nodeDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test with untracked file
	status, err = repo.GetStatus(ctx, nodeDir)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if status.Clean {
		t.Error("Expected repository to not be clean")
	}

	if len(status.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(status.Files))
	}

	if len(status.Files) > 0 && status.Files[0].Status != "?" {
		t.Errorf("Expected untracked status '?', got %s", status.Files[0].Status)
	}

	// Stage the file
	err = repo.StageFiles(ctx, nodeDir, []string{"test.txt"})
	if err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}

	// Test with staged file
	status, err = repo.GetStatus(ctx, nodeDir)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if len(status.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(status.Files))
	}

	if len(status.Files) > 0 && status.Files[0].Status != "A" {
		t.Errorf("Expected added status 'A', got %s", status.Files[0].Status)
	}
}

func TestGitCommit(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test node directory
	nodeDir := filepath.Join(tempDir, "test-node")
	err = os.Mkdir(nodeDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = nodeDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = nodeDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = nodeDir
	cmd.Run()

	// Create and stage a test file
	testFile := filepath.Join(nodeDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create repository instance
	repo := New("test@example.com", "Test User")
	ctx := context.Background()

	// Stage the file
	err = repo.StageFiles(ctx, nodeDir, []string{"test.txt"})
	if err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}

	// Create commit
	err = repo.CreateCommit(ctx, nodeDir, "Test commit")
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	// Verify commit was created
	history, err := repo.GetHistory(ctx, nodeDir, 1)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("Expected 1 commit, got %d", len(history))
	}

	if len(history) > 0 && !strings.Contains(history[0].Message, "Test commit") {
		t.Errorf("Expected commit message to contain 'Test commit', got %s", history[0].Message)
	}
}

func TestGitDiff(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test node directory
	nodeDir := filepath.Join(tempDir, "test-node")
	err = os.Mkdir(nodeDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = nodeDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = nodeDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = nodeDir
	cmd.Run()

	// Create and commit initial file
	testFile := filepath.Join(nodeDir, "test.txt")
	err = os.WriteFile(testFile, []byte("initial content\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = nodeDir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = nodeDir
	cmd.Run()

	// Modify the file
	err = os.WriteFile(testFile, []byte("initial content\nmodified content\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create repository instance
	repo := New("test@example.com", "Test User")
	ctx := context.Background()

	// Get unstaged diff
	diff, err := repo.GetDiff(ctx, nodeDir, false)
	if err != nil {
		t.Fatalf("Failed to get diff: %v", err)
	}

	if !strings.Contains(diff, "+modified content") {
		t.Error("Expected diff to contain '+modified content'")
	}

	// Stage the changes
	err = repo.StageFiles(ctx, nodeDir, []string{"test.txt"})
	if err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}

	// Get staged diff
	diff, err = repo.GetDiff(ctx, nodeDir, true)
	if err != nil {
		t.Fatalf("Failed to get staged diff: %v", err)
	}

	if !strings.Contains(diff, "+modified content") {
		t.Error("Expected staged diff to contain '+modified content'")
	}
}

func TestNonGitRepository(t *testing.T) {
	// Create a temporary directory without git
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create repository instance
	repo := New("test@example.com", "Test User")
	ctx := context.Background()

	// All operations should return appropriate errors
	_, err = repo.GetStatus(ctx, tempDir)
	if err == nil {
		t.Error("Expected error for non-git directory")
	}

	_, err = repo.GetDiff(ctx, tempDir, false)
	if err == nil {
		t.Error("Expected error for non-git directory")
	}

	_, err = repo.GetHistory(ctx, tempDir, 10)
	if err == nil {
		t.Error("Expected error for non-git directory")
	}

	err = repo.CreateCommit(ctx, tempDir, "Test")
	if err == nil {
		t.Error("Expected error for non-git directory")
	}
}