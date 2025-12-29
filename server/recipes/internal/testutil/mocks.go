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
type RealGitRepository struct{}

func NewRealGitRepository() *RealGitRepository {
	return &RealGitRepository{}
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
	return fmt.Errorf("not implemented")
}

func (r *RealGitRepository) Fetch(ctx context.Context, path string, opts git.FetchOptions) error {
	return nil // No-op for local testing
}

func (r *RealGitRepository) Pull(ctx context.Context, path string, opts git.PullOptions) (*git.PullResult, error) {
	return &git.PullResult{Updated: false}, nil // No-op for local testing
}

func (r *RealGitRepository) Push(ctx context.Context, path string, opts git.PushOptions) (*git.PushResult, error) {
	return &git.PushResult{}, nil // No-op for local testing
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
	return fmt.Errorf("not implemented")
}

func (r *RealGitRepository) ListRemotes(ctx context.Context, path string) ([]git.Remote, error) {
	return nil, fmt.Errorf("not implemented")
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

var (
	_ model.Clock            = (*MockClock)(nil)
	_ model.ShortIDGenerator = (*MockIDGenerator)(nil)
	_ project.Service        = (*MockProjectService)(nil)
	_ git.Repository         = (*RealGitRepository)(nil)
)
