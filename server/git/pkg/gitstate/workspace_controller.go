package gitstate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/colony-2/colony2/server/git/pkg/common"
	"github.com/colony-2/colony2/server/git/pkg/gitcommit"
	"github.com/colony-2/colony2/server/git/pkg/gitshallow"
)

// Controller orchestrates cloning, restoring, and persisting git state per activity invocation.
type Controller struct {
	adapters map[string]StorageAdapter
}

// NewController constructs a Controller with the supplied adapters. If nil, a default file adapter is used.
func NewController(adapters map[string]StorageAdapter) *Controller {
	if adapters == nil {
		adapters = map[string]StorageAdapter{}
	}
	if _, ok := adapters["file"]; !ok {
		adapters["file"] = FileAdapter{}
	}
	return &Controller{
		adapters: adapters,
	}
}

// PrepareWorkspace ensures the worktree exists and the blob-store location is ready.

func (c *Controller) prepareWorkspace(ctx context.Context, task *GitTaskContext) error {
	if task.GetWorktreePath() == "" {
		return fmt.Errorf("git workspace requires worktree path")
	}
	if task.GetBaseRepo() == "" {
		return fmt.Errorf("git workspace requires base repository path")
	}
	if task.GetBaseRef() == "" {
		return fmt.Errorf("git workspace requires base ref")
	}

	if err := c.cloneIfNeeded(ctx, task); err != nil {
		return err
	}

	if task.ResolvedBaseHash == "" {
		if head, err := common.GetCommitHash(ctx, task.GetWorktreePath(), "HEAD"); err == nil {
			task.ResolvedBaseHash = head
		}
	}

	adapter, err := c.adapterFor(task.GetBlobStoreURI())
	if err != nil {
		return err
	}
	if _, err := adapter.EnsureLocation(ctx, task.GetBlobStoreURI()); err != nil {
		return fmt.Errorf("prepare blobstore: %w", err)
	}
	return nil
}

// Restore replays thin packs when the target persist hash differs from the workspace state. It also prepares the workspace if it doesn't yet have a local copy of the repo.
func (c *Controller) Restore(ctx context.Context, task *GitTaskContext) error {
	err := c.prepareWorkspace(ctx, task)
	if err != nil {
		return err
	}

	if task.GetWorktreePath() == "" {
		return fmt.Errorf("git workspace requires worktree path")
	}

	targetRef := strings.TrimSpace(task.GetBaseRef())
	targetHash := strings.TrimSpace(task.GetPersistHash())

	if !dirExists(filepath.Join(task.GetWorktreePath(), ".git")) {
		return fmt.Errorf("git workspace missing .git directory: %s", task.GetWorktreePath())
	}

	if targetRef == "" && targetHash == "" {
		return nil
	}

	// Ref-first path: follow the ref tip and update resolved hash without thin packs.
	if targetHash == "" && targetRef != "" {
		hash, err := checkoutAndTrackRef(ctx, task.GetWorktreePath(), targetRef)
		if err != nil {
			return err
		}
		task.ResolvedBaseHash = hash
		task.ParentHash = ""
		task.PersistHash = ""
		return c.ensureCleanAfterRestore(ctx, task)
	}

	// if hashes are populated, operate in hash mode

	current, err := common.GetCommitHash(ctx, task.GetWorktreePath(), "HEAD")
	if err != nil {
		return fmt.Errorf("determine current commit: %w", err)
	}
	if hashesEqual(current, targetHash) {
		task.ResolvedBaseHash = current
		return c.ensureCleanAfterRestore(ctx, task)
	}

	rootHash := strings.TrimSpace(task.GetResolvedBaseHash())
	if rootHash == "" {
		rootHash = strings.TrimSpace(task.GetParentHash())
	}
	if rootHash == "" {
		rootHash = targetHash
	}
	if rootHash == "" {
		return fmt.Errorf("git workspace requires resolved base hash for restore")
	}

	adapter, err := c.adapterFor(task.GetBlobStoreURI())
	if err != nil {
		return err
	}
	storageRoot, err := adapter.EnsureLocation(ctx, task.GetBlobStoreURI())
	if err != nil {
		return err
	}

	thinPackDir := filepath.Join(storageRoot, thinPackSubdir)
	packEntries, err := adapter.ListBlobs(ctx, task.GetBlobStoreURI(), filepath.Join(thinPackSubdir, "*.pack"))
	if err == nil {
		expectedCommit := shortHash(task.GetPersistHash())
		expectedRoot := shortHash(rootHash)
		found := false
		for _, entry := range packEntries {
			name := filepath.Base(entry)
			if strings.HasPrefix(name, expectedCommit) && strings.HasSuffix(name, expectedRoot+".pack") {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	restoreInput := gitcommit.RestoreCommitActivity{
		RepoPath:        task.GetWorktreePath(),
		TargetCommit:    task.GetPersistHash(),
		RootHash:        rootHash,
		StorageLocation: thinPackDir,
		Force:           true,
	}
	if _, err := gitcommit.RestoreCommit(ctx, restoreInput); err != nil {
		return fmt.Errorf("restore git state: %w", err)
	}

	if err := c.ensureCleanAfterRestore(ctx, task); err != nil {
		return err
	}
	task.ResolvedBaseHash = targetHash
	return nil
}

// Persist captures repository changes, writes thin packs, and returns the new commit hash alongside refreshed context.
func (c *Controller) Persist(ctx context.Context, task *GitTaskContext) (*gitcommit.PersistCommitOutput, error) {
	scopePath, err := c.prepareScopedWorkspace(ctx, task)
	if err != nil {
		return nil, err
	}

	adapter, err := c.adapterFor(task.GetBlobStoreURI())
	if err != nil {
		return nil, err
	}
	localRoot, err := adapter.EnsureLocation(ctx, task.GetBlobStoreURI())
	if err != nil {
		return nil, err
	}

	thinPackDir := filepath.Join(localRoot, thinPackSubdir)
	if err := os.MkdirAll(thinPackDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure thin-pack dir: %w", err)
	}

	rootHash := task.GetResolvedBaseHash()
	if strings.TrimSpace(rootHash) == "" {
		var err error
		rootHash, err = common.GetCommitHash(ctx, task.GetWorktreePath(), "HEAD")
		if err != nil {
			return nil, fmt.Errorf("resolve base hash: %w", err)
		}
		task.ResolvedBaseHash = rootHash
	}

	commitMessage := buildCommitMessage(task, "pending", "")
	author := task.GetGitAuthor()
	if author == "" && task.GetCellName() != "" {
		author = fmt.Sprintf("%s <%s@colony2>", task.GetCellName(), task.GetCellName())
	}

	persistInput := gitcommit.PersistCommitActivity{
		RepoPath:        task.GetWorktreePath(),
		StorageLocation: thinPackDir,
		RootHash:        rootHash,
		CommitMessage:   commitMessage,
		Author:          author,
	}

	output, err := gitcommit.PersistCommit(ctx, persistInput)
	if err != nil {
		return nil, fmt.Errorf("persist commit failed %w", err)
	}

	if output.HasChanges && output.ThinPackPath != "" {
		relativePackPath, err := filepath.Rel(localRoot, output.ThinPackPath)
		if err != nil {
			return nil, fmt.Errorf("compute thin-pack path: %w", err)
		}
		if _, err := adapter.PutBlob(ctx, task.GetBlobStoreURI(), relativePackPath, output.ThinPackPath); err != nil {
			return nil, fmt.Errorf("store thin-pack: %w", err)
		}
		task.ThinPackPath = filepath.ToSlash(relativePackPath)
	}

	//updated.ThinPackPath = filepath.ToSlash(relativePackPath)

	if scopePath != "" {
		// Ensure the worktree remains clean after persistence.
		if err := common.EnsureCleanAfterRestore(ctx, task.GetWorktreePath(), scopePath, output.CommitHash); err != nil {
			return nil, fmt.Errorf("failed to ensure clean repo after persist: %w", err)
		}
	}

	if output.HasChanges {
		task.ParentHash = output.ParentHash
		task.PersistHash = output.CommitHash
		task.ResolvedBaseHash = output.CommitHash
	} else {
		task.ParentHash = output.ParentHash
		task.PersistHash = ""
		task.ResolvedBaseHash = rootHash
	}

	return output, nil
}

func (c *Controller) prepareScopedWorkspace(ctx context.Context, task *GitTaskContext) (string, error) {
	worktree := strings.TrimSpace(task.GetWorktreePath())
	if worktree == "" {
		return "", fmt.Errorf("git workspace requires worktree path")
	}

	scope, err := c.resolveScopePath(ctx, task)
	if err != nil {
		return "", err
	}

	if scope == "" {
		return "", nil
	}

	if _, err := common.PrepareScopedCommit(ctx, worktree, scope); err != nil {
		return "", err
	}

	return scope, nil
}

func (c *Controller) ensureCleanAfterRestore(ctx context.Context, task *GitTaskContext) error {
	worktree := strings.TrimSpace(task.GetWorktreePath())
	if worktree == "" {
		return fmt.Errorf("git workspace requires worktree path")
	}

	scope, err := c.resolveScopePath(ctx, task)
	if err != nil {
		return err
	}
	if scope == "" {
		scope = "."
	}

	return common.EnsureCleanAfterRestore(ctx, worktree, scope, task.GetPersistHash())
}

func (c *Controller) resolveScopePath(ctx context.Context, task *GitTaskContext) (string, error) {
	cell := strings.TrimSpace(task.GetCellName())
	if cell == "" {
		return ".", nil
	}

	sanitized := filepath.ToSlash(strings.Trim(cell, "/"))
	if sanitized == "" {
		return "", fmt.Errorf("cell name cannot resolve to repository root")
	}
	if strings.Contains(sanitized, "..") || filepath.IsAbs(sanitized) {
		return "", fmt.Errorf("invalid cell path %s", cell)
	}
	return sanitized, nil
}

func (c *Controller) adapterFor(uri string) (StorageAdapter, error) {
	scheme := "file"
	if uri != "" {
		if parsed, err := url.Parse(uri); err == nil {
			if parsed.Scheme != "" {
				scheme = strings.ToLower(parsed.Scheme)
			}
		} else if idx := strings.Index(uri, ":"); idx > 0 {
			scheme = strings.ToLower(uri[:idx])
		}
	}
	adapter, ok := c.adapters[scheme]
	if !ok {
		return nil, fmt.Errorf("no storage adapter configured for scheme %s", scheme)
	}
	return adapter, nil
}

func checkoutAndTrackRef(ctx context.Context, repoPath, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("checkout requires ref")
	}

	if _, err := common.ExecuteGitCommand(ctx, repoPath, "fetch", "--all", "--tags", "--prune"); err != nil {
		return "", fmt.Errorf("fetch ref %s: %w", ref, err)
	}
	if _, err := common.ExecuteGitCommand(ctx, repoPath, "checkout", ref); err != nil {
		return "", fmt.Errorf("checkout ref %s: %w", ref, err)
	}

	hash, err := common.GetCommitHash(ctx, repoPath, "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve ref %s: %w", ref, err)
	}
	return hash, nil
}

func (c *Controller) cloneIfNeeded(ctx context.Context, task *GitTaskContext) error {
	if dirExists(filepath.Join(task.GetWorktreePath(), ".git")) {
		return nil
	}

	source := task.GetBaseRepo()
	if strings.HasPrefix(source, "file://") {
		path, err := fileURIToPath(source)
		if err != nil {
			return err
		}
		source = path
	}

	if err := os.MkdirAll(filepath.Dir(task.GetWorktreePath()), 0o755); err != nil {
		return fmt.Errorf("prepare worktree dir: %w", err)
	}

	input := gitshallow.GitShallowCloneInput{
		SourceDir:  source,
		TargetDir:  task.GetWorktreePath(),
		CommitHash: task.GetBaseRef(),
	}
	if _, err := gitshallow.GitShallowClone(ctx, input); err != nil {
		return fmt.Errorf("clone workspace: %w", err)
	}

	if head, err := common.GetCommitHash(ctx, task.GetWorktreePath(), "HEAD"); err == nil {
		task.ResolvedBaseHash = head
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func hashesEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) > 7 {
		a = a[:7]
	}
	if len(b) > 7 {
		b = b[:7]
	}
	return strings.EqualFold(a, b)
}

func shortHash(hash string) string {
	if len(hash) >= 7 {
		return hash[:7]
	}
	return hash
}
