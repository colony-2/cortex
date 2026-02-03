package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func looksLikeCommitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}
	return true
}

func gitCommitExists(ctx context.Context, repoPath string, commitHash string) bool {
	cmd := exec.CommandContext(ctx, "git", "cat-file", "-e", fmt.Sprintf("%s^{commit}", commitHash))
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

func gitIsShallowRepo(ctx context.Context, repoPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-shallow-repository")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func gitFetch(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"fetch"}, args...)...)
	cmd.Dir = repoPath
	return cmd.CombinedOutput()
}

func (s *service) ensureCommitAvailable(ctx context.Context, repoPath string, commitHash string) error {
	if commitHash == "" {
		return fmt.Errorf("empty commit hash")
	}

	if gitCommitExists(ctx, repoPath, commitHash) {
		return nil
	}

	var outBuf bytes.Buffer

	// Best case: fetch just the requested commit.
	if out, err := gitFetch(ctx, repoPath, "--depth", "1", "origin", commitHash); err == nil {
		if gitCommitExists(ctx, repoPath, commitHash) {
			return nil
		}
		outBuf.Write(out)
	} else {
		outBuf.Write(out)
	}

	// Fall back to deepening the shallow clone.
	for _, deepen := range []int{50, 200, 1000, 5000} {
		if out, err := gitFetch(ctx, repoPath, fmt.Sprintf("--deepen=%d", deepen), "origin"); err == nil {
			if gitCommitExists(ctx, repoPath, commitHash) {
				return nil
			}
			outBuf.Write(out)
		} else {
			outBuf.Write(out)
		}
	}

	// Last resort: unshallow if possible.
	if shallow, err := gitIsShallowRepo(ctx, repoPath); err == nil && shallow {
		if out, err := gitFetch(ctx, repoPath, "--unshallow", "origin"); err == nil {
			if gitCommitExists(ctx, repoPath, commitHash) {
				return nil
			}
			outBuf.Write(out)
		} else {
			outBuf.Write(out)
		}
	}

	msg := strings.TrimSpace(outBuf.String())
	if msg == "" {
		msg = "commit object not available after fetch"
	}
	return fmt.Errorf("%s", msg)
}

func (s *service) resolveRefToCommit(ctx context.Context, repoPath string, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}

	// If it's a commit-ish string, let ensureCommitAvailable fetch the object if needed.
	if looksLikeCommitHash(ref) {
		return ref, nil
	}

	revParse := func() (string, []byte, error) {
		cmd := exec.CommandContext(ctx, "git", "rev-parse", ref)
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", out, err
		}
		return strings.TrimSpace(string(out)), out, nil
	}

	if commitHash, _, err := revParse(); err == nil {
		return commitHash, nil
	}

	// Try fetching tags (tag refs are not guaranteed to be present in a depth=1 clone).
	_, _ = gitFetch(ctx, repoPath, "--tags", "origin")
	if commitHash, _, err := revParse(); err == nil {
		return commitHash, nil
	}

	// If this is a shallow repo, unshallow and retry (helps with HEAD~1 style refs).
	if shallow, err := gitIsShallowRepo(ctx, repoPath); err == nil && shallow {
		_, _ = gitFetch(ctx, repoPath, "--unshallow", "origin")
		if commitHash, out, err := revParse(); err == nil {
			_ = out
			return commitHash, nil
		}
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", ref)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	return "", fmt.Errorf("failed to resolve ref %s: %w\nOutput: %s", ref, err, strings.TrimSpace(string(out)))
}

func (s *service) ensureRepoHasHistory(ctx context.Context, repoPath string) error {
	shallow, err := gitIsShallowRepo(ctx, repoPath)
	if err != nil {
		return err
	}
	if !shallow {
		return nil
	}
	out, err := gitFetch(ctx, repoPath, "--unshallow", "origin")
	if err != nil {
		return fmt.Errorf("failed to unshallow repository: %w\nOutput: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

