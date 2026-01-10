package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// normalizeScope ensures the pathspec is repo-relative using forward slashes.
func normalizeScope(scope string) (string, error) {
	trimmed := strings.TrimSpace(scope)
	if trimmed == "" {
		return "", fmt.Errorf("scope path cannot be empty")
	}
	cleaned := filepath.ToSlash(trimmed)
	for strings.HasPrefix(cleaned, "./") {
		cleaned = strings.TrimPrefix(cleaned, "./")
	}
	cleaned = strings.Trim(cleaned, "/")
	if cleaned == "" {
		return ".", nil
	}
	if cleaned == "." {
		return ".", nil
	}
	return cleaned, nil
}

// PrepareScopedCommit resets staged changes outside the provided scope, cleans untracked
// files, and stages the scoped path. It returns true when the scope produces staged changes.
func PrepareScopedCommit(ctx context.Context, repoPath, scope string) (bool, error) {
	normalized, err := normalizeScope(scope)
	if err != nil {
		return false, err
	}

	if normalized == "." {
		output, err := ExecuteGitCommand(ctx, repoPath, "status", "--porcelain")
		if err != nil {
			return false, fmt.Errorf("inspect git status: %w", err)
		}
		return strings.TrimSpace(string(output)) != "", nil
	}

	if _, err := ExecuteGitCommand(ctx, repoPath, "reset", "--mixed"); err != nil {
		return false, fmt.Errorf("reset staging area: %w", err)
	}

	statusOutput, err := ExecuteGitCommand(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("inspect git status: %w", err)
	}
	scopePrefix := normalized + "/"
	lines := strings.Split(strings.TrimSpace(string(statusOutput)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		// Check if path is within scope (exact match or subdirectory)
		if path == normalized || strings.HasPrefix(path, scopePrefix) {
			continue
		}
		// Check if path is a parent directory of the scope
		// e.g., path="cells/" and scope="cells/test"
		// Git status shows "?? cells/" when there are untracked files under cells/
		if strings.HasSuffix(path, "/") && strings.HasPrefix(scopePrefix, path) {
			continue
		}
		statusCode := line[:2]
		fullPath := filepath.Join(repoPath, path)
		if strings.HasPrefix(statusCode, "??") {
			if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
				return false, fmt.Errorf("remove untracked %s: %w", path, err)
			}
			continue
		}
		if _, err := ExecuteGitCommand(ctx, repoPath, "restore", "--staged", "--worktree", "--", path); err != nil {
			return false, fmt.Errorf("restore tracked %s: %w", path, err)
		}
	}

	if _, err := ExecuteGitCommand(ctx, repoPath, "add", "-A"); err != nil {
		return false, fmt.Errorf("stage changes: %w", err)
	}

	diffOutput, err := ExecuteGitCommand(ctx, repoPath, "diff", "--cached", "--name-only")
	if err != nil {
		return false, fmt.Errorf("inspect staged diff: %w", err)
	}

	hasChanges := strings.TrimSpace(string(diffOutput)) != ""
	if !hasChanges {
		statusOutput, err := ExecuteGitCommand(ctx, repoPath, "status", "--porcelain")
		if err != nil {
			return false, fmt.Errorf("final status check: %w", err)
		}
		if residual := strings.TrimSpace(string(statusOutput)); residual != "" {
			return false, fmt.Errorf("residual changes after scoping: %s", residual)
		}
	}

	return hasChanges, nil
}

// EnsureCleanAfterRestore enforces a clean worktree after a restore operation by aligning the
// checkout with the target commit and cleaning untracked files outside the scoped path.

func EnsureCleanAfterRestore(ctx context.Context, repoPath, scope, targetCommit string) error {
	if targetCommit != "" {
		if _, err := ExecuteGitCommand(ctx, repoPath, "reset", "--hard", targetCommit); err != nil {
			return fmt.Errorf("reset to %s: %w", targetCommit, err)
		}
	} else {
		if _, err := ExecuteGitCommand(ctx, repoPath, "reset", "--hard"); err != nil {
			return fmt.Errorf("reset worktree: %w", err)
		}
	}

	if scope := strings.TrimSpace(scope); scope != "" && scope != "." {
		exclude := fmt.Sprintf(":^%s", scope)
		if _, err := ExecuteGitCommand(ctx, repoPath, "clean", "-fd", "--", exclude); err != nil {
			return fmt.Errorf("clean worktree outside scope: %w", err)
		}
	} else {
		if _, err := ExecuteGitCommand(ctx, repoPath, "clean", "-fd"); err != nil {
			return fmt.Errorf("clean worktree: %w", err)
		}
	}

	statusOutput, err := ExecuteGitCommand(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("post-restore status: %w", err)
	}
	if residual := strings.TrimSpace(string(statusOutput)); residual != "" {
		return fmt.Errorf("worktree dirty after restore: %s", residual)
	}
	return nil
}
