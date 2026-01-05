package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/model"
)

// MockClock is a controllable clock for testing.
type MockClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewMockClock(t time.Time) *MockClock {
	return &MockClock{now: t}
}

func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *MockClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *MockClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// MockIDGenerator generates predictable IDs for testing.
type MockIDGenerator struct {
	mu      sync.Mutex
	counter int
	prefix  string
}

func NewMockIDGenerator(prefix string) *MockIDGenerator {
	return &MockIDGenerator{prefix: prefix}
}

func (g *MockIDGenerator) NewID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counter++
	return fmt.Sprintf("%s%04d", g.prefix, g.counter), nil
}

// MockProjectService is a simple project service for testing.
type MockProjectService struct {
	mu       sync.RWMutex
	projects map[project.ID]*project.Project
}

func NewMockProjectService() *MockProjectService {
	return &MockProjectService{
		projects: make(map[project.ID]*project.Project),
	}
}

func (s *MockProjectService) AddProject(p *project.Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[p.ID] = p
}

func (s *MockProjectService) CreateProject(ctx context.Context, input project.CreateInput) (*project.Project, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MockProjectService) GetProject(ctx context.Context, id project.ID) (*project.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	if !ok {
		return nil, fmt.Errorf("project not found")
	}
	return p, nil
}

func (s *MockProjectService) ListProjects(ctx context.Context, filter project.SearchFilter) (project.Iterator[*project.Project], error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MockProjectService) UpdateProject(ctx context.Context, id project.ID, patch project.UpdateInput) (*project.Project, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MockProjectService) DeleteProject(ctx context.Context, id project.ID) error {
	return fmt.Errorf("not implemented")
}

// RealGitRepository implements git.Repository using real git commands.
// This is for integration testing with actual git operations.
type RealGitRepository struct{
	underlying git.Repository
}

func NewRealGitRepository() *RealGitRepository {
	// Use the actual git.Repository implementation for correct behavior
	underlying := git.NewRepository(git.Config{
		DefaultAuthor: "Test User",
		DefaultEmail:  "test@example.com",
	})
	return &RealGitRepository{underlying: underlying}
}

func (r *RealGitRepository) InitRepository(ctx context.Context, path string, opts git.InitOptions) error {
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure git user for commits
	configCmds := [][]string{
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	}
	for _, args := range configCmds {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = path
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (r *RealGitRepository) Clone(ctx context.Context, url string, path string, opts git.CloneOptions) error {
	args := []string{"clone"}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	args = append(args, url, path)

	cmd := exec.CommandContext(ctx, "git", args...)
	return cmd.Run()
}

func (r *RealGitRepository) Fetch(ctx context.Context, path string, opts git.FetchOptions) error {
	args := []string{"fetch"}
	if opts.Remote != "" {
		args = append(args, opts.Remote)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	return cmd.Run()
}

func (r *RealGitRepository) Pull(ctx context.Context, path string, opts git.PullOptions) (*git.PullResult, error) {
	// Get current commit before pull
	oldCommit, _ := r.GetCurrentCommit(ctx, path)

	args := []string{"pull"}
	if opts.Remote != "" {
		args = append(args, opts.Remote)
	}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}
	if opts.FastForward {
		args = append(args, "--ff-only")
	}
	if opts.Rebase {
		args = append(args, "--rebase")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	// Get new commit after pull
	newCommit, _ := r.GetCurrentCommit(ctx, path)

	return &git.PullResult{
		Updated:   oldCommit != newCommit,
		OldCommit: oldCommit,
		NewCommit: newCommit,
	}, nil
}

func (r *RealGitRepository) Push(ctx context.Context, path string, opts git.PushOptions) (*git.PushResult, error) {
	// Delegate to underlying implementation which handles bare/non-bare logic
	return r.underlying.Push(ctx, path, opts)
}

func (r *RealGitRepository) CreateCommit(ctx context.Context, path string, message string) error {
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Dir = path
	return cmd.Run()
}

func (r *RealGitRepository) StageFiles(ctx context.Context, path string, files []string) error {
	args := append([]string{"add"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	return cmd.Run()
}

func (r *RealGitRepository) UnstageFiles(ctx context.Context, path string, files []string) error {
	return fmt.Errorf("not implemented")
}

func (r *RealGitRepository) GetStatus(ctx context.Context, path string) (*git.Status, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *RealGitRepository) GetDiff(ctx context.Context, path string, staged bool) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (r *RealGitRepository) GetHistory(ctx context.Context, path string, limit int) ([]git.Commit, error) {
	args := []string{"log", "--format=%H|%an|%ae|%at|%s"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	commits := make([]git.Commit, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}
		var timestamp int64
		fmt.Sscanf(parts[3], "%d", &timestamp)
		shortHash := parts[0]
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}
		commits = append(commits, git.Commit{
			Hash:      parts[0],
			Author:    parts[1],
			Date:      time.Unix(timestamp, 0),
			Message:   parts[4],
			ShortHash: shortHash,
		})
	}
	return commits, nil
}

func (r *RealGitRepository) GetCurrentCommit(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *RealGitRepository) GetFileAtCommit(ctx context.Context, path string, commit string, filePath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "show", fmt.Sprintf("%s:%s", commit, filePath))
	cmd.Dir = path
	return cmd.Output()
}

func (r *RealGitRepository) ListFilesAtCommit(ctx context.Context, path string, commit string, directory string) ([]git.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *RealGitRepository) IsAncestor(ctx context.Context, path string, ancestor string, descendant string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (r *RealGitRepository) GetRemoteHead(ctx context.Context, path string, remote string, branch string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (r *RealGitRepository) Checkout(ctx context.Context, path string, ref string, opts git.CheckoutOptions) error {
	return fmt.Errorf("not implemented")
}

func (r *RealGitRepository) AddRemote(ctx context.Context, path string, name string, url string) error {
	cmd := exec.CommandContext(ctx, "git", "remote", "add", name, url)
	cmd.Dir = path
	return cmd.Run()
}

func (r *RealGitRepository) ListRemotes(ctx context.Context, path string) ([]git.Remote, error) {
	// Get remote names
	cmd := exec.CommandContext(ctx, "git", "remote")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	remotes := make([]git.Remote, 0, len(lines))

	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// Get fetch URL
		cmd := exec.CommandContext(ctx, "git", "remote", "get-url", name)
		cmd.Dir = path
		fetchURL, err := cmd.Output()
		if err != nil {
			continue
		}

		// Get push URL (try, may be same as fetch)
		cmd = exec.CommandContext(ctx, "git", "remote", "get-url", "--push", name)
		cmd.Dir = path
		pushURL, err := cmd.Output()
		if err != nil {
			pushURL = fetchURL
		}

		remotes = append(remotes, git.Remote{
			Name:     name,
			FetchURL: strings.TrimSpace(string(fetchURL)),
			PushURL:  strings.TrimSpace(string(pushURL)),
		})
	}

	return remotes, nil
}

func (r *RealGitRepository) IsRepoBare(ctx context.Context, path string) (bool, error) {
	// Delegate to underlying implementation
	return r.underlying.IsRepoBare(ctx, path)
}

func (r *RealGitRepository) GetCurrentBranch(ctx context.Context, path string) (string, error) {
	// Delegate to underlying implementation
	return r.underlying.GetCurrentBranch(ctx, path)
}

func (r *RealGitRepository) ConfigureUser(ctx context.Context, path string, name string, email string) error {
	// Delegate to underlying implementation
	return r.underlying.ConfigureUser(ctx, path, name, email)
}

func (r *RealGitRepository) UpdateRemoteURL(ctx context.Context, path string, remoteName string, newURL string) error {
	// Delegate to underlying implementation
	return r.underlying.UpdateRemoteURL(ctx, path, remoteName, newURL)
}

// CreateTestRecipeContent creates valid recipe YAML content for testing.
func CreateTestRecipeContent(id string) []byte {
	return []byte(fmt.Sprintf(`version: "1.0"
id: "%s"
op: echo
inputs:
  message: "Test recipe %s"
`, id, id))
}

// CreateGitRepo creates a real git repository in the given path for testing.
func CreateGitRepo(ctx context.Context, path string) error {
	repo := NewRealGitRepository()
	if err := repo.InitRepository(ctx, path, git.InitOptions{}); err != nil {
		return err
	}

	// Configure git user for commits
	if err := ConfigureGitUser(ctx, path); err != nil {
		return err
	}

	// Create a minimal initial commit
	initFile := filepath.Join(path, ".gitkeep")
	if err := writeFile(initFile, []byte("")); err != nil {
		return err
	}

	if err := repo.StageFiles(ctx, path, []string{".gitkeep"}); err != nil {
		return err
	}

	return repo.CreateCommit(ctx, path, "Initial commit")
}

// CreateBareGitRepo creates a bare git repository in the given path for testing.
// Bare repositories can receive pushes, unlike non-bare repositories.
func CreateBareGitRepo(ctx context.Context, path string) error {
	repo := NewRealGitRepository()
	if err := repo.InitRepository(ctx, path, git.InitOptions{Bare: true}); err != nil {
		return err
	}

	// Set default branch to avoid issues with initial push
	cmd := GitCommand(ctx, path, "symbolic-ref", "HEAD", "refs/heads/main")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set default branch: %w", err)
	}

	return nil
}

// ConfigureGitUser configures git user.name and user.email for a repository.
// This is required before creating commits in tests.
func ConfigureGitUser(ctx context.Context, path string) error {
	cmds := [][]string{
		{"git", "config", "user.name", "Test User"},
		{"git", "config", "user.email", "test@example.com"},
	}

	for _, cmdArgs := range cmds {
		cmd := GitCommand(ctx, path, cmdArgs[1:]...)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

func writeFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := mkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

func mkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// GitCommand creates a git command in the specified directory.
func GitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd
}

var (
	_ model.Clock            = (*MockClock)(nil)
	_ model.ShortIDGenerator = (*MockIDGenerator)(nil)
	_ project.Service        = (*MockProjectService)(nil)
	_ git.Repository         = (*RealGitRepository)(nil)
)
