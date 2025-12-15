package squashrebasemerge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/colony-2/colony2/server/git/pkg/common"
)

const defaultTargetBranch = "refs/heads/main"

// workspaceSnapshot captures relevant git context for the operation.
type workspaceSnapshot struct {
	RepoPath    string
	BaseHash    string
	PersistHash string
	BaseRepo    string
	GitAuthor   string
	Worktree    string
	CellName    string
}

// Run executes the squash-rebase-merge flow.
func Run(ctx context.Context, input SquashRebaseMergeInput) (*SquashRebaseMergeOutput, error) {
	snapshot, err := resolveWorkspaceSnapshot(input)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateRepository(snapshot.RepoPath); err != nil {
		return nil, fmt.Errorf("validate repository: %w", err)
	}

	targetBranch := strings.TrimSpace(input.TargetBranch)
	if targetBranch == "" {
		targetBranch = defaultTargetBranch
	}

	if snapshot.BaseHash == "" {
		return nil, fmt.Errorf("context.git.base_hash is required")
	}

	if snapshot.PersistHash == "" {
		hash, err := common.GetCommitHash(ctx, snapshot.RepoPath, "HEAD")
		if err != nil {
			return nil, fmt.Errorf("determine current HEAD: %w", err)
		}
		snapshot.PersistHash = hash
	}

	remoteName, err := determineRemoteName(ctx, snapshot, input.UpstreamRemote)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(remoteName) == "" {
		return nil, fmt.Errorf("no remote configured for squashrebasemerge")
	}

	if err := fetchTargetBranch(ctx, snapshot.RepoPath, remoteName, targetBranch); err != nil {
		return nil, err
	}

	remoteRef := remoteTrackingRef(remoteName, targetBranch)
	remoteTip, err := common.GetCommitHash(ctx, snapshot.RepoPath, remoteRef)
	if err != nil {
		return nil, fmt.Errorf("resolve remote target %s: %w", remoteRef, err)
	}

	ancestor, err := isAncestor(ctx, snapshot.RepoPath, snapshot.BaseHash, remoteTip)
	if err != nil {
		return nil, err
	}
	if !ancestor {
		return nil, fmt.Errorf("remote branch %s no longer descends from base %s", targetBranch, shortHash(snapshot.BaseHash))
	}

	skipRebase := input.SkipRebase
	rangeBaseHash := snapshot.BaseHash
	if skipRebase {
		fastForwardable, err := isAncestor(ctx, snapshot.RepoPath, remoteTip, snapshot.PersistHash)
		if err != nil {
			return nil, fmt.Errorf("check fast-forward eligibility: %w", err)
		}
		if !fastForwardable {
			return nil, common.NotFastForwardError{
				TargetBranch: targetBranch,
				LocalHead:    snapshot.PersistHash,
				UpstreamHead: remoteTip,
			}
		}
		rangeBaseHash = remoteTip
	}

	newlyCommitted, err := countCommitsBetween(ctx, snapshot.RepoPath, rangeBaseHash, snapshot.PersistHash)
	if err != nil {
		return nil, err
	}

	if newlyCommitted == 0 {
		// Nothing to merge; fast-forward local context to remote tip.
		if _, err := common.ExecuteGitCommand(ctx, snapshot.RepoPath, "reset", "--hard", remoteTip); err != nil {
			return nil, fmt.Errorf("fast-forward workspace to %s: %w", shortHash(remoteTip), err)
		}
		return &SquashRebaseMergeOutput{
			TargetBranch: targetBranch,
			RemoteRef:    remoteRef,
			MergedHash:   remoteTip,
			SquashedCommits: SquashRangeSummary{
				BaseHash:    snapshot.BaseHash,
				PersistHash: snapshot.PersistHash,
			},
			GitContextPatch: map[string]interface{}{
				"base_hash":     remoteTip,
				"persist_hash":  remoteTip,
				"previous_hash": snapshot.PersistHash,
			},
			FastForward: skipRebase,
		}, nil
	}

	commitLog, err := collectCommitSummaries(ctx, snapshot.RepoPath, rangeBaseHash, snapshot.PersistHash)
	if err != nil {
		return nil, err
	}

	originalHead := snapshot.PersistHash
	restoreOnError := func() {
		_, _ = common.ExecuteGitCommand(context.Background(), snapshot.RepoPath, "reset", "--hard", originalHead)
	}

	if err := createSquashCommit(ctx, snapshot, rangeBaseHash, targetBranch, commitLog, input.PreserveAuthor); err != nil {
		restoreOnError()
		return nil, err
	}

	var mergedHash string
	if skipRebase {
		mergedHash, err = common.GetCommitHash(ctx, snapshot.RepoPath, "HEAD")
		if err != nil {
			restoreOnError()
			return nil, fmt.Errorf("determine merged head: %w", err)
		}

		stillAncestor, err := isAncestor(ctx, snapshot.RepoPath, remoteTip, mergedHash)
		if err != nil {
			restoreOnError()
			return nil, fmt.Errorf("confirm fast-forward ancestor: %w", err)
		}
		if !stillAncestor {
			restoreOnError()
			return nil, common.NotFastForwardError{
				TargetBranch: targetBranch,
				LocalHead:    mergedHash,
				UpstreamHead: remoteTip,
			}
		}

		if _, err := common.ExecuteGitCommand(ctx, snapshot.RepoPath, "push", remoteName, fmt.Sprintf("HEAD:%s", targetBranch)); err != nil {
			restoreOnError()
			if isNonFastForwardError(err) {
				return nil, common.NotFastForwardError{
					TargetBranch: targetBranch,
					LocalHead:    mergedHash,
					UpstreamHead: remoteTip,
				}
			}
			return nil, fmt.Errorf("push to %s: %w", targetBranch, err)
		}
	} else {
		if err := rebaseOntoTarget(ctx, snapshot.RepoPath, remoteTip, snapshot.BaseHash, input.PreserveAuthor, snapshot.GitAuthor); err != nil {
			restoreOnError()
			return nil, err
		}

		mergedHash, err = common.GetCommitHash(ctx, snapshot.RepoPath, "HEAD")
		if err != nil {
			restoreOnError()
			return nil, fmt.Errorf("determine merged head: %w", err)
		}

		if _, err := common.ExecuteGitCommand(ctx, snapshot.RepoPath, "push", remoteName, fmt.Sprintf("HEAD:%s", targetBranch)); err != nil {
			restoreOnError()
			return nil, fmt.Errorf("push to %s: %w", targetBranch, err)
		}
	}

	return &SquashRebaseMergeOutput{
		TargetBranch: targetBranch,
		RemoteRef:    remoteRef,
		MergedHash:   mergedHash,
		SquashedCommits: SquashRangeSummary{
			BaseHash:    snapshot.BaseHash,
			PersistHash: snapshot.PersistHash,
		},
		GitContextPatch: map[string]interface{}{
			"base_hash":     mergedHash,
			"persist_hash":  mergedHash,
			"previous_hash": snapshot.PersistHash,
		},
		FastForward: skipRebase,
	}, nil
}

func resolveWorkspaceSnapshot(input SquashRebaseMergeInput) (workspaceSnapshot, error) {
	snapshot := workspaceSnapshot{}
	ctxMap := input.Context
	var gitMap map[string]interface{}
	if ctxMap != nil {
		if m, ok := ctxMap["git"].(map[string]interface{}); ok {
			gitMap = m
		}
	}

	repoPath := strings.TrimSpace(input.RepoPath)
	if repoPath == "" && gitMap != nil {
		if val, ok := stringFromMap(gitMap, "worktree_path"); ok {
			repoPath = val
		}
	}
	if repoPath == "" && ctxMap != nil {
		if val, ok := stringFromMap(ctxMap, "worktree"); ok {
			repoPath = val
		}
	}
	if repoPath == "" {
		return snapshot, fmt.Errorf("repo_path or context.git.worktree_path is required")
	}

	snapshot.RepoPath = repoPath
	snapshot.Worktree = repoPath

	if gitMap != nil {
		snapshot.BaseHash, _ = stringFromMap(gitMap, "base_hash")
		snapshot.PersistHash, _ = stringFromMap(gitMap, "persist_hash")
		snapshot.BaseRepo, _ = stringFromMap(gitMap, "base_repo")
		snapshot.GitAuthor, _ = stringFromMap(gitMap, "git_author")
		if wt, ok := stringFromMap(gitMap, "worktree_path"); ok {
			snapshot.Worktree = wt
		}
	}

	if ctxMap != nil {
		snapshot.CellName, _ = stringFromMap(ctxMap, "cellname")
		if snapshot.GitAuthor == "" && snapshot.CellName != "" {
			snapshot.GitAuthor = fmt.Sprintf("%s <%s@colony2>", snapshot.CellName, snapshot.CellName)
		}
	}

	return snapshot, nil
}

func determineRemoteName(ctx context.Context, snapshot workspaceSnapshot, explicit string) (string, error) {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit, nil
	}

	out, err := common.ExecuteGitCommand(ctx, snapshot.RepoPath, "remote", "-v")
	if err != nil {
		return "", fmt.Errorf("list remotes: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	baseNorm := normalizeRemoteURL(snapshot.BaseRepo)
	remoteSeen := make(map[string]struct{})
	var first string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		url := fields[1]
		remoteSeen[name] = struct{}{}
		if first == "" {
			first = name
		}
		if baseNorm != "" && normalizeRemoteURL(url) == baseNorm {
			return name, nil
		}
	}

	if len(remoteSeen) == 1 {
		return first, nil
	}

	if len(remoteSeen) == 0 {
		return "", fmt.Errorf("repository has no remotes configured")
	}

	return "", fmt.Errorf("unable to determine remote; provide upstream_remote explicitly")
}

func fetchTargetBranch(ctx context.Context, repoPath, remoteName, targetBranch string) error {
	if _, err := common.ExecuteGitCommand(ctx, repoPath, "fetch", remoteName, targetBranch); err != nil {
		return fmt.Errorf("fetch %s %s: %w", remoteName, targetBranch, err)
	}
	return nil
}

func remoteTrackingRef(remoteName, targetBranch string) string {
	short := strings.TrimPrefix(targetBranch, "refs/")
	short = strings.TrimPrefix(short, "heads/")
	if short == targetBranch {
		short = strings.TrimPrefix(targetBranch, "refs/heads/")
	}
	short = strings.TrimPrefix(short, "heads/")
	short = strings.TrimPrefix(short, "remotes/")
	return fmt.Sprintf("%s/%s", remoteName, short)
}

func collectCommitSummaries(ctx context.Context, repoPath, baseHash, persistHash string) (string, error) {
	out, err := common.ExecuteGitCommand(ctx, repoPath, "log", "--pretty=format:%h %an <%ae> %s", fmt.Sprintf("%s..%s", baseHash, persistHash))
	if err != nil {
		return "", fmt.Errorf("collect commit summaries: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func createSquashCommit(ctx context.Context, snapshot workspaceSnapshot, squashBaseHash, targetBranch, commitLog string, preserveAuthor *bool) error {
	if _, err := common.ExecuteGitCommand(ctx, snapshot.RepoPath, "reset", "--soft", squashBaseHash); err != nil {
		return fmt.Errorf("prepare squash reset: %w", err)
	}

	author := snapshot.GitAuthor
	keepAuthor := true
	if preserveAuthor != nil {
		keepAuthor = *preserveAuthor
	}

	if keepAuthor {
		originalAuthor, err := common.ExecuteGitCommand(ctx, snapshot.RepoPath, "show", "-s", "--format=%an <%ae>", snapshot.PersistHash)
		if err == nil {
			author = strings.TrimSpace(string(originalAuthor))
		}
	}

	message := buildSquashCommitMessage(snapshot, targetBranch, commitLog, squashBaseHash)

	args := []string{"commit", "-m", message}
	env := os.Environ()
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if author != "" {
		args = append(args, "--author", author)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = snapshot.RepoPath
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create squash commit: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

func rebaseOntoTarget(ctx context.Context, repoPath, remoteTip, originalBase string, preserveAuthor *bool, fallbackAuthor string) error {
	keepAuthor := true
	if preserveAuthor != nil {
		keepAuthor = *preserveAuthor
	}

	args := []string{"rebase", "--reapply-cherry-picks", "--onto", remoteTip, originalBase}

	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SEQUENCE_EDITOR=true")
	if !keepAuthor && fallbackAuthor != "" {
		name, email := splitAuthor(fallbackAuthor)
		if name != "" && email != "" {
			args = append([]string{"-c", fmt.Sprintf("user.name=%s", name), "-c", fmt.Sprintf("user.email=%s", email)}, args...)
			args = append(args, "--exec", "git commit --amend --no-edit --reset-author")
		}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = abortRebase(ctx, repoPath)
		return fmt.Errorf("git rebase failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func abortRebase(ctx context.Context, repoPath string) error {
	_, err := common.ExecuteGitCommand(ctx, repoPath, "rebase", "--abort")
	return err
}

func countCommitsBetween(ctx context.Context, repoPath, baseHash, persistHash string) (int, error) {
	if strings.TrimSpace(baseHash) == "" || strings.TrimSpace(persistHash) == "" {
		return 0, fmt.Errorf("base and persist hashes are required to compute squash range")
	}
	out, err := common.ExecuteGitCommand(ctx, repoPath, "rev-list", "--count", fmt.Sprintf("%s..%s", baseHash, persistHash))
	if err != nil {
		return 0, fmt.Errorf("count commits between %s and %s: %w", shortHash(baseHash), shortHash(persistHash), err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}
	count, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse commit count: %w", err)
	}
	return count, nil
}

func isAncestor(ctx context.Context, repoPath, ancestor, descendant string) (bool, error) {
	if ancestor == "" || descendant == "" {
		return false, fmt.Errorf("ancestor and descendant must be provided")
	}
	_, err := common.ExecuteGitCommand(ctx, repoPath, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false, err
	}
	if exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func buildSquashCommitMessage(snapshot workspaceSnapshot, targetBranch, commitLog string, baseHash string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Squash delivery to %s\n\n", targetBranch)
	fmt.Fprintf(&b, "Base: %s\n", baseHash)
	fmt.Fprintf(&b, "Original tip: %s\n", snapshot.PersistHash)
	b.WriteString("\nCommits:\n")
	if commitLog == "" {
		b.WriteString("  - (no individual commits listed)\n")
	} else {
		for _, line := range strings.Split(commitLog, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	return b.String()
}

func splitAuthor(author string) (string, string) {
	author = strings.TrimSpace(author)
	if author == "" {
		return "", ""
	}
	if !strings.Contains(author, "<") {
		return author, ""
	}
	idx := strings.Index(author, "<")
	name := strings.TrimSpace(author[:idx])
	email := strings.TrimSpace(strings.TrimSuffix(author[idx+1:], ">"))
	return name, email
}

func normalizeRemoteURL(url string) string {
	if url == "" {
		return ""
	}
	cleaned := strings.TrimSpace(url)
	cleaned = strings.TrimSuffix(cleaned, "(fetch)")
	cleaned = strings.TrimSuffix(cleaned, "(push)")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "file://")
	if strings.HasPrefix(cleaned, "ssh://") {
		cleaned = strings.TrimPrefix(cleaned, "ssh://")
	}
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = strings.TrimRight(cleaned, "/")
	return cleaned
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func stringFromMap(m map[string]interface{}, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			str = strings.TrimSpace(str)
			if str != "" {
				return str, true
			}
		}
	}
	return "", false
}

func isNonFastForwardError(err error) bool {
	if err == nil {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "non-fast-forward") || strings.Contains(lowered, "[rejected]")
}
