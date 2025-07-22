package gitshallow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CloneInput represents the input parameters for the Clone function
type CloneInput struct {
	SourceDir  string `json:"sourceDir"`
	TargetDir  string `json:"targetDir"`
	CommitHash string `json:"commitHash"`
}

// CloneOutput represents the output of the Clone function
type CloneOutput struct {
	ClonedPath string `json:"clonedPath"`
}

// Clone performs a shallow clone of a local git repository to another directory
func Clone(ctx context.Context, input CloneInput) (*CloneOutput, error) {
	// Validate inputs
	if input.SourceDir == "" {
		return nil, fmt.Errorf("source directory cannot be empty")
	}
	if input.TargetDir == "" {
		return nil, fmt.Errorf("target directory cannot be empty")
	}
	if input.CommitHash == "" {
		return nil, fmt.Errorf("commit hash cannot be empty")
	}

	// Check if source directory exists and is a git repository
	if _, err := os.Stat(input.SourceDir); err != nil {
		return nil, fmt.Errorf("source directory does not exist: %w", err)
	}

	gitDir := filepath.Join(input.SourceDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, fmt.Errorf("source directory is not a git repository: %w", err)
	}

	// Check if target directory already exists
	if _, err := os.Stat(input.TargetDir); err == nil {
		return nil, fmt.Errorf("target directory already exists: %s", input.TargetDir)
	}

	// Create parent directory if needed
	parentDir := filepath.Dir(input.TargetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Perform shallow clone with depth 1
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--no-single-branch", input.SourceDir, input.TargetDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Fetch the specific commit
	cmd = exec.CommandContext(ctx, "git", "-C", input.TargetDir, "fetch", "--depth", "1", "origin", input.CommitHash)
	if err := cmd.Run(); err != nil {
		// Clean up on failure
		os.RemoveAll(input.TargetDir)
		return nil, fmt.Errorf("failed to fetch commit %s: %w", input.CommitHash, err)
	}

	// Checkout the specific commit
	cmd = exec.CommandContext(ctx, "git", "-C", input.TargetDir, "checkout", input.CommitHash)
	if err := cmd.Run(); err != nil {
		// Clean up on failure
		os.RemoveAll(input.TargetDir)
		return nil, fmt.Errorf("failed to checkout commit %s: %w", input.CommitHash, err)
	}

	return &CloneOutput{
		ClonedPath: input.TargetDir,
	}, nil
}