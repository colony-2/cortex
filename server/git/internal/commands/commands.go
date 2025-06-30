package commands

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Status represents the current status of a Git repository
type Status struct {
	Branch       string       `json:"branch"`
	Clean        bool         `json:"clean"`
	Files        []StatusFile `json:"files"`
	Ahead        int          `json:"ahead"`
	Behind       int          `json:"behind"`
	HasRemote    bool         `json:"hasRemote"`
}

// StatusFile represents a single file in the Git status
type StatusFile struct {
	Path   string `json:"path"`
	Status string `json:"status"` // "M", "A", "D", "?", etc.
}

// Commit represents a Git commit
type Commit struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Date      time.Time `json:"date"`
	Message   string    `json:"message"`
	ShortHash string    `json:"shortHash"`
}

// Repository implements git repository operations using command line git
type Repository struct {
	defaultAuthor string
	defaultEmail  string
}

// New creates a new git repository implementation
func New(defaultAuthor, defaultEmail string) *Repository {
	return &Repository{
		defaultAuthor: defaultAuthor,
		defaultEmail:  defaultEmail,
	}
}

// GetStatus returns the current Git status of a node directory
func (r *Repository) GetStatus(ctx context.Context, nodePath string) (*Status, error) {
	// Check if it's a git repository
	if !isGitRepo(nodePath) {
		return nil, fmt.Errorf("not a git repository")
	}

	status := &Status{
		Branch: "main",
		Clean:  true,
		Files:  []StatusFile{},
	}

	// Get current branch
	cmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	cmd.Dir = nodePath
	if output, err := cmd.Output(); err == nil {
		status.Branch = strings.TrimSpace(string(output))
	}

	// Get status
	cmd = exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = nodePath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}

	// Parse status output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		status.Clean = false
		
		// Parse status line (format: "XY filename")
		if len(line) >= 3 {
			statusCode := string(line[0:2])
			filename := strings.TrimSpace(line[3:])
			
			// Convert to simple status
			simpleStatus := "?"
			if strings.Contains(statusCode, "M") {
				simpleStatus = "M"
			} else if strings.Contains(statusCode, "A") {
				simpleStatus = "A"
			} else if strings.Contains(statusCode, "D") {
				simpleStatus = "D"
			} else if strings.Contains(statusCode, "R") {
				simpleStatus = "R"
			} else if strings.Contains(statusCode, "C") {
				simpleStatus = "C"
			}
			
			status.Files = append(status.Files, StatusFile{
				Path:   filename,
				Status: simpleStatus,
			})
		}
	}

	// Check if we have a remote
	cmd = exec.CommandContext(ctx, "git", "remote")
	cmd.Dir = nodePath
	if output, err := cmd.Output(); err == nil && len(output) > 0 {
		status.HasRemote = true
		
		// Get ahead/behind info
		cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", "--left-right", "@{upstream}...HEAD")
		cmd.Dir = nodePath
		if output, err := cmd.Output(); err == nil {
			parts := strings.Fields(string(output))
			if len(parts) == 2 {
				if behind, err := strconv.Atoi(parts[0]); err == nil {
					status.Behind = behind
				}
				if ahead, err := strconv.Atoi(parts[1]); err == nil {
					status.Ahead = ahead
				}
			}
		}
	}

	return status, nil
}

// GetDiff returns the diff of uncommitted changes
func (r *Repository) GetDiff(ctx context.Context, nodePath string, staged bool) (string, error) {
	if !isGitRepo(nodePath) {
		return "", fmt.Errorf("not a git repository")
	}

	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = nodePath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	return string(output), nil
}

// GetHistory returns the commit history for a node
func (r *Repository) GetHistory(ctx context.Context, nodePath string, limit int) ([]Commit, error) {
	if !isGitRepo(nodePath) {
		return nil, fmt.Errorf("not a git repository")
	}

	args := []string{"log", fmt.Sprintf("--max-count=%d", limit), "--pretty=format:%H|%an|%ae|%at|%s"}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = nodePath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	var commits []Commit
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		timestamp, _ := strconv.ParseInt(parts[3], 10, 64)
		commit := Commit{
			Hash:      parts[0],
			Author:    fmt.Sprintf("%s <%s>", parts[1], parts[2]),
			Date:      time.Unix(timestamp, 0),
			Message:   parts[4],
			ShortHash: parts[0][:7],
		}
		commits = append(commits, commit)
	}

	return commits, nil
}

// CreateCommit creates a new commit with the given message
func (r *Repository) CreateCommit(ctx context.Context, nodePath, message string) error {
	if !isGitRepo(nodePath) {
		return fmt.Errorf("not a git repository")
	}

	// Check if there are changes to commit
	status, err := r.GetStatus(ctx, nodePath)
	if err != nil {
		return err
	}

	hasStaged := false
	for _, file := range status.Files {
		if file.Status == "A" || file.Status == "M" || file.Status == "D" {
			hasStaged = true
			break
		}
	}

	if !hasStaged {
		return fmt.Errorf("no changes to commit")
	}

	// Create commit
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Dir = nodePath
	
	// Set author if configured
	if r.defaultAuthor != "" && r.defaultEmail != "" {
		cmd.Env = append(cmd.Environ(),
			fmt.Sprintf("GIT_AUTHOR_NAME=%s", r.defaultAuthor),
			fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", r.defaultEmail),
			fmt.Sprintf("GIT_COMMITTER_NAME=%s", r.defaultAuthor),
			fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", r.defaultEmail),
		)
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create commit: %w\nOutput: %s", err, output)
	}

	return nil
}

// StageFiles stages the specified files for commit
func (r *Repository) StageFiles(ctx context.Context, nodePath string, files []string) error {
	if !isGitRepo(nodePath) {
		return fmt.Errorf("not a git repository")
	}

	if len(files) == 0 {
		return nil
	}

	args := append([]string{"add"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = nodePath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage files: %w\nOutput: %s", err, output)
	}

	return nil
}

// UnstageFiles unstages the specified files
func (r *Repository) UnstageFiles(ctx context.Context, nodePath string, files []string) error {
	if !isGitRepo(nodePath) {
		return fmt.Errorf("not a git repository")
	}

	if len(files) == 0 {
		return nil
	}

	args := append([]string{"reset", "HEAD"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = nodePath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to unstage files: %w\nOutput: %s", err, output)
	}

	return nil
}

// InitRepository initializes a new Git repository in the node directory
func (r *Repository) InitRepository(ctx context.Context, nodePath string) error {
	if isGitRepo(nodePath) {
		return fmt.Errorf("already a git repository")
	}

	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = nodePath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to initialize repository: %w\nOutput: %s", err, output)
	}

	// Configure user if defaults are provided
	if r.defaultAuthor != "" && r.defaultEmail != "" {
		// Set user name
		cmd = exec.CommandContext(ctx, "git", "config", "user.name", r.defaultAuthor)
		cmd.Dir = nodePath
		cmd.Run()

		// Set user email
		cmd = exec.CommandContext(ctx, "git", "config", "user.email", r.defaultEmail)
		cmd.Dir = nodePath
		cmd.Run()
	}

	return nil
}

// isGitRepo checks if a directory is a git repository
func isGitRepo(dirPath string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dirPath
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	
	return cmd.Run() == nil
}