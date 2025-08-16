package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecuteGitCommand runs a git command with context
func ExecuteGitCommand(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("git command failed: %w (output: %s)", err, string(output))
	}
	return output, nil
}

// ValidateRepository checks if path is a valid Git repository
func ValidateRepository(repoPath string) error {
	if repoPath == "" {
		return fmt.Errorf("repository path cannot be empty")
	}

	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a git repository (or any of the parent directories): %s", repoPath)
		}
		return fmt.Errorf("error checking git repository: %w", err)
	}
	return nil
}

// ParseThinPackName extracts metadata from thin pack filename
func ParseThinPackName(filename string) (*ThinPackMetadata, error) {
	// Parse format: {commit_hash}-{parent_hash}-{root_hash}.pack
	base := strings.TrimSuffix(filepath.Base(filename), ".pack")
	parts := strings.Split(base, "-")
	
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid thin pack filename format: %s", filename)
	}
	
	return &ThinPackMetadata{
		CommitHash: parts[0],
		ParentHash: parts[1],
		RootHash:   parts[2],
		FilePath:   filename,
	}, nil
}

// GetCommitHash returns the full commit hash for a given ref
func GetCommitHash(ctx context.Context, repoPath, ref string) (string, error) {
	output, err := ExecuteGitCommand(ctx, repoPath, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash for %s: %w", ref, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// CommitExists checks if a commit exists in the repository
func CommitExists(ctx context.Context, repoPath, commitHash string) bool {
	_, err := ExecuteGitCommand(ctx, repoPath, "cat-file", "-e", commitHash)
	return err == nil
}