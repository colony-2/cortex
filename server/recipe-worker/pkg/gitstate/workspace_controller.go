package gitstate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/divisive-ai/vibethis/server/git/pkg/common"
	"github.com/divisive-ai/vibethis/server/git/pkg/gitcommit"
	"github.com/divisive-ai/vibethis/server/git/pkg/gitshallow"
)

// Workspace defines the contract the controller relies upon for git state.
type Workspace interface {
	GetBaseRepo() string
	GetBaseHash() string
	GetPersistHash() string
	GetPreviousHash() string
	GetWorktreePath() string
	GetBlobStoreURI() string
	GetTicketID() string
	GetCellName() string
	GetRecipeID() string
	GetRecipeNode() string
	GetWorkflowID() string
	GetWorkflowRunID() string
	GetInvocationID() string
	GetInvocationHash() string
	GetInvocationAttempt() int
	GetBoxID() string
	GetActivityID() string
	GetGitAuthor() string
	GetThinPackPath() string
	IsWorkspacePrepared() bool
}

// Controller orchestrates cloning, restoring, and persisting git state per activity invocation.
type Controller struct {
	adapters map[string]StorageAdapter
	mu       sync.Mutex
	prepared map[string]bool
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
		prepared: make(map[string]bool),
	}
}

// PrepareWorkspace ensures the worktree exists and the blob-store location is ready.

func (c *Controller) PrepareWorkspace(ctx context.Context, ws Workspace) error {
	if ws.GetWorktreePath() == "" {
		return fmt.Errorf("git workspace requires worktree path")
	}
	if ws.GetBaseRepo() == "" {
		return fmt.Errorf("git workspace requires base repository path")
	}
	if ws.GetBaseHash() == "" {
		return fmt.Errorf("git workspace requires base hash")
	}

	c.mu.Lock()
	alreadyPrepared := c.prepared[ws.GetWorktreePath()]
	c.mu.Unlock()

	if !alreadyPrepared {
		if err := c.cloneIfNeeded(ctx, ws); err != nil {
			return err
		}
		c.mu.Lock()
		c.prepared[ws.GetWorktreePath()] = true
		c.mu.Unlock()
	}

	adapter, err := c.adapterFor(ws.GetBlobStoreURI())
	if err != nil {
		return err
	}
	if _, err := adapter.EnsureLocation(ctx, ws.GetBlobStoreURI()); err != nil {
		return fmt.Errorf("prepare blobstore: %w", err)
	}

	if mut, ok := ws.(interface{ SetWorkspacePrepared(bool) }); ok {
		mut.SetWorkspacePrepared(true)
	}
	return nil
}

// Restore replays thin packs when the target persist hash differs from the workspace state.
func (c *Controller) Restore(ctx context.Context, ws Workspace) error {
	if ws.GetPersistHash() == "" || ws.GetWorktreePath() == "" {
		return nil
	}

	if !dirExists(filepath.Join(ws.GetWorktreePath(), ".git")) {
		return fmt.Errorf("git workspace missing .git directory: %s", ws.GetWorktreePath())
	}

	current, err := common.GetCommitHash(ctx, ws.GetWorktreePath(), "HEAD")
	if err != nil {
		return fmt.Errorf("determine current commit: %w", err)
	}
	if hashesEqual(current, ws.GetPersistHash()) {
		return c.ensureCleanAfterRestore(ctx, ws)
	}

	adapter, err := c.adapterFor(ws.GetBlobStoreURI())
	if err != nil {
		return err
	}
	storageRoot, err := adapter.EnsureLocation(ctx, ws.GetBlobStoreURI())
	if err != nil {
		return err
	}

	thinPackDir := filepath.Join(storageRoot, thinPackSubdir)
	packEntries, err := adapter.ListBlobs(ctx, ws.GetBlobStoreURI(), filepath.Join(thinPackSubdir, "*.pack"))
	if err == nil {
		expectedCommit := shortHash(ws.GetPersistHash())
		expectedRoot := shortHash(ws.GetBaseHash())
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
		RepoPath:        ws.GetWorktreePath(),
		TargetCommit:    ws.GetPersistHash(),
		RootHash:        ws.GetBaseHash(),
		StorageLocation: thinPackDir,
		Force:           true,
	}
	if _, err := gitcommit.RestoreCommit(ctx, restoreInput); err != nil {
		return fmt.Errorf("restore git state: %w", err)
	}

	return c.ensureCleanAfterRestore(ctx, ws)
}

// Persist captures repository changes, writes thin packs, and returns the new commit hash alongside refreshed context.
func (c *Controller) Persist(ctx context.Context, ws Workspace) (string, Context, error) {
	scopePath, err := c.prepareScopedWorkspace(ctx, ws)
	if err != nil {
		return "", Context{}, err
	}

	adapter, err := c.adapterFor(ws.GetBlobStoreURI())
	if err != nil {
		return "", Context{}, err
	}
	localRoot, err := adapter.EnsureLocation(ctx, ws.GetBlobStoreURI())
	if err != nil {
		return "", Context{}, err
	}

	thinPackDir := filepath.Join(localRoot, thinPackSubdir)
	if err := os.MkdirAll(thinPackDir, 0o755); err != nil {
		return "", Context{}, fmt.Errorf("ensure thin-pack dir: %w", err)
	}

	ctxInfo := contextFromWorkspace(ws)
	commitMessage := buildCommitMessage(ctxInfo, "pending", "")
	author := ws.GetGitAuthor()
	if author == "" && ws.GetCellName() != "" {
		author = fmt.Sprintf("%s <%s@vibethis>", ws.GetCellName(), ws.GetCellName())
	}

	persistInput := gitcommit.PersistCommitActivity{
		RepoPath:        ws.GetWorktreePath(),
		StorageLocation: thinPackDir,
		RootHash:        ws.GetBaseHash(),
		CommitMessage:   commitMessage,
		Author:          author,
	}

	output, err := gitcommit.PersistCommit(ctx, persistInput)
	if err != nil {
		if strings.Contains(err.Error(), "root hash") && strings.Contains(err.Error(), "does not exist") {
			return ws.GetPersistHash(), ctxInfo, nil
		}
		return "", Context{}, fmt.Errorf("persist commit: %w", err)
	}

	relativePackPath, err := filepath.Rel(localRoot, output.ThinPackPath)
	if err != nil {
		return "", Context{}, fmt.Errorf("compute thin-pack path: %w", err)
	}
	if _, err := adapter.PutBlob(ctx, ws.GetBlobStoreURI(), relativePackPath, output.ThinPackPath); err != nil {
		return "", Context{}, fmt.Errorf("store thin-pack: %w", err)
	}

	updated := ctxInfo
	updated.PreviousHash = ws.GetPersistHash()
	updated.PersistHash = output.CommitHash
	updated.ThinPackPath = filepath.ToSlash(relativePackPath)
	updated.WorkspacePrepared = true

	if scopePath != "" {
		// Ensure the worktree remains clean after persistence.
		if err := common.EnsureCleanAfterRestore(ctx, ws.GetWorktreePath(), scopePath, output.CommitHash); err != nil {
			return "", Context{}, err
		}
	}

	return output.CommitHash, updated, nil
}

func (c *Controller) prepareScopedWorkspace(ctx context.Context, ws Workspace) (string, error) {
	worktree := strings.TrimSpace(ws.GetWorktreePath())
	if worktree == "" {
		return "", fmt.Errorf("git workspace requires worktree path")
	}

	scope, err := c.resolveScopePath(ctx, ws)
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

func (c *Controller) ensureCleanAfterRestore(ctx context.Context, ws Workspace) error {
	worktree := strings.TrimSpace(ws.GetWorktreePath())
	if worktree == "" {
		return fmt.Errorf("git workspace requires worktree path")
	}

	scope, err := c.resolveScopePath(ctx, ws)
	if err != nil {
		return err
	}
	if scope == "" {
		scope = "."
	}

	return common.EnsureCleanAfterRestore(ctx, worktree, scope, ws.GetPersistHash())
}

func (c *Controller) resolveScopePath(ctx context.Context, ws Workspace) (string, error) {
	cell := strings.TrimSpace(ws.GetCellName())
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

func (c *Controller) cloneIfNeeded(ctx context.Context, ws Workspace) error {
	if dirExists(filepath.Join(ws.GetWorktreePath(), ".git")) {
		return nil
	}

	source := ws.GetBaseRepo()
	if strings.HasPrefix(source, "file://") {
		path, err := fileURIToPath(source)
		if err != nil {
			return err
		}
		source = path
	}

	if err := os.MkdirAll(filepath.Dir(ws.GetWorktreePath()), 0o755); err != nil {
		return fmt.Errorf("prepare worktree dir: %w", err)
	}

	input := gitshallow.GitShallowCloneInput{
		SourceDir:  source,
		TargetDir:  ws.GetWorktreePath(),
		CommitHash: ws.GetBaseHash(),
	}
	if _, err := gitshallow.GitShallowClone(ctx, input); err != nil {
		return fmt.Errorf("clone workspace: %w", err)
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

func contextFromWorkspace(ws Workspace) Context {
	return Context{
		BaseRepo:          ws.GetBaseRepo(),
		BaseHash:          ws.GetBaseHash(),
		PersistHash:       ws.GetPersistHash(),
		PreviousHash:      ws.GetPreviousHash(),
		WorktreePath:      ws.GetWorktreePath(),
		BlobStoreURI:      ws.GetBlobStoreURI(),
		ThinPackPath:      ws.GetThinPackPath(),
		TicketID:          ws.GetTicketID(),
		CellName:          ws.GetCellName(),
		RecipeID:          ws.GetRecipeID(),
		RecipeNode:        ws.GetRecipeNode(),
		InvocationID:      ws.GetInvocationID(),
		InvocationHash:    ws.GetInvocationHash(),
		InvocationAttempt: ws.GetInvocationAttempt(),
		BoxID:             ws.GetBoxID(),
		ActivityID:        ws.GetActivityID(),
		WorkflowID:        ws.GetWorkflowID(),
		WorkflowRunID:     ws.GetWorkflowRunID(),
		WorkspacePrepared: ws.IsWorkspacePrepared(),
		GitAuthor:         ws.GetGitAuthor(),
	}
}

func shortHash(hash string) string {
	if len(hash) >= 7 {
		return hash[:7]
	}
	return hash
}
