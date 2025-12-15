// Package git provides Git repository operations for colony2.
package git

import (
	"context"
	"github.com/colony-2/colony2/server/git/internal/commands"
	"time"
)

// Status represents the current status of a Git repository.
type Status struct {
	Branch    string       `json:"branch"`
	Clean     bool         `json:"clean"`
	Files     []StatusFile `json:"files"`
	Ahead     int          `json:"ahead"`
	Behind    int          `json:"behind"`
	HasRemote bool         `json:"hasRemote"`
}

// StatusFile represents a single file in the Git status.
type StatusFile struct {
	Path   string `json:"path"`
	Status string `json:"status"` // "M", "A", "D", "?", etc.
}

// Commit represents a Git commit.
type Commit struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Date      time.Time `json:"date"`
	Message   string    `json:"message"`
	ShortHash string    `json:"shortHash"`
}

// Repository provides Git operations for node directories.
type Repository interface {
	// GetStatus returns the current Git status of a node directory.
	GetStatus(ctx context.Context, nodePath string) (*Status, error)

	// GetDiff returns the diff of uncommitted changes.
	GetDiff(ctx context.Context, nodePath string, staged bool) (string, error)

	// GetHistory returns the commit history for a node.
	GetHistory(ctx context.Context, nodePath string, limit int) ([]Commit, error)

	// CreateCommit creates a new commit with the given message.
	CreateCommit(ctx context.Context, nodePath, message string) error

	// StageFiles stages the specified files for commit.
	StageFiles(ctx context.Context, nodePath string, files []string) error

	// UnstageFiles unstages the specified files.
	UnstageFiles(ctx context.Context, nodePath string, files []string) error
}

// Config defines configuration for Git operations.
type Config struct {
	// DefaultAuthor is used when no Git user is configured.
	DefaultAuthor string

	// DefaultEmail is used when no Git email is configured.
	DefaultEmail string
}

// NewRepository creates a new Git repository interface.
func NewRepository(config Config) Repository {
	repo := commands.New(config.DefaultAuthor, config.DefaultEmail)
	return &repoAdapter{repo: repo}
}

// repoAdapter adapts the internal repository to the public interface
type repoAdapter struct {
	repo *commands.Repository
}

func (a *repoAdapter) GetStatus(ctx context.Context, nodePath string) (*Status, error) {
	status, err := a.repo.GetStatus(ctx, nodePath)
	if err != nil {
		return nil, err
	}

	// Convert internal Status to public Status
	result := &Status{
		Branch:    status.Branch,
		Clean:     status.Clean,
		Files:     make([]StatusFile, len(status.Files)),
		Ahead:     status.Ahead,
		Behind:    status.Behind,
		HasRemote: status.HasRemote,
	}

	for i, f := range status.Files {
		result.Files[i] = StatusFile{
			Path:   f.Path,
			Status: f.Status,
		}
	}

	return result, nil
}

func (a *repoAdapter) GetDiff(ctx context.Context, nodePath string, staged bool) (string, error) {
	return a.repo.GetDiff(ctx, nodePath, staged)
}

func (a *repoAdapter) GetHistory(ctx context.Context, nodePath string, limit int) ([]Commit, error) {
	commits, err := a.repo.GetHistory(ctx, nodePath, limit)
	if err != nil {
		return nil, err
	}

	// Convert internal Commit to public Commit
	result := make([]Commit, len(commits))
	for i, c := range commits {
		result[i] = Commit{
			Hash:      c.Hash,
			Author:    c.Author,
			Date:      c.Date,
			Message:   c.Message,
			ShortHash: c.ShortHash,
		}
	}

	return result, nil
}

func (a *repoAdapter) CreateCommit(ctx context.Context, nodePath, message string) error {
	return a.repo.CreateCommit(ctx, nodePath, message)
}

func (a *repoAdapter) StageFiles(ctx context.Context, nodePath string, files []string) error {
	return a.repo.StageFiles(ctx, nodePath, files)
}

func (a *repoAdapter) UnstageFiles(ctx context.Context, nodePath string, files []string) error {
	return a.repo.UnstageFiles(ctx, nodePath, files)
}
