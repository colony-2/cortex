package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/divisive-ai/vibethis/server/activity/pkg/types"
)

// GitShallowCloneConfig defines the configuration for git shallow clone activities - ALL fields MUST have json tags
type GitShallowCloneConfig struct {
	// Optional: working directory for the clone operation
	WorkingDir string `json:"working_dir"`
	// Optional: additional git config settings
	GitConfig map[string]string `json:"git_config"`
}

// GitShallowCloneInput defines the input for git shallow clone activities - ALL fields MUST have json tags
type GitShallowCloneInput struct {
	Repository string `json:"repository"` // Required: repository URL to clone
	Depth      int    `json:"depth"`      // Required: depth of shallow clone (e.g., 1 for single commit)
	Branch     string `json:"branch"`     // Optional: specific branch to clone (defaults to default branch)
	TargetDir  string `json:"target_dir"` // Optional: directory name for the clone (defaults to repo name)
}

// GitShallowCloneOutput defines the output from git shallow clone activities - ALL fields MUST have json tags
type GitShallowCloneOutput struct {
	ClonePath    string `json:"clone_path"`    // The absolute path where the repository was cloned
	CommitHash   string `json:"commit_hash"`   // The commit hash of the cloned repository
	Branch       string `json:"branch"`        // The branch that was cloned
	CloneDepth   int    `json:"clone_depth"`   // The actual depth of the clone
	RepositoryURL string `json:"repository_url"` // The repository URL that was cloned
}

// GitShallowCloneActivityWrapper implements the RegisterableActivity interface
type GitShallowCloneActivityWrapper struct{}

// Ensure we implement the interface
var _ types.RegisterableActivity[GitShallowCloneConfig, GitShallowCloneInput, GitShallowCloneOutput] = (*GitShallowCloneActivityWrapper)(nil)

// NewGitShallowCloneActivity creates a new git shallow clone activity that implements RegisterableActivity
func NewGitShallowCloneActivity() types.RegisterableActivity[GitShallowCloneConfig, GitShallowCloneInput, GitShallowCloneOutput] {
	return &GitShallowCloneActivityWrapper{}
}

// GetMetadata returns activity metadata for registration
func (a *GitShallowCloneActivityWrapper) GetMetadata() types.ActivityMetadata {
	return types.ActivityMetadata{
		Type:           "git_shallow_clone",
		Name:           "Git Shallow Clone",
		Description:    "Performs a shallow clone of a git repository with specified depth",
		Version:        "1.0.0",
		DefaultTimeout: 5 * time.Minute,
		RetryPolicy: &types.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    60 * time.Second,
			NonRetryableErrorTypes: []string{
				"RepositoryNotFoundError",
				"AuthenticationError",
				"InvalidRepositoryError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *GitShallowCloneActivityWrapper) Execute(ctx context.Context, config GitShallowCloneConfig, input GitShallowCloneInput) (GitShallowCloneOutput, error) {
	// Validate inputs
	if input.Repository == "" {
		return GitShallowCloneOutput{}, fmt.Errorf("repository URL is required")
	}
	if input.Depth < 1 {
		return GitShallowCloneOutput{}, fmt.Errorf("depth must be at least 1")
	}

	// Determine working directory
	workingDir := config.WorkingDir
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	// Determine target directory
	targetDir := input.TargetDir
	if targetDir == "" {
		// Extract repository name from URL
		targetDir = filepath.Base(input.Repository)
		if ext := filepath.Ext(targetDir); ext == ".git" {
			targetDir = targetDir[:len(targetDir)-len(ext)]
		}
	}

	// Create absolute path for the clone
	clonePath := filepath.Join(workingDir, targetDir)

	// Build git command
	args := []string{"clone", "--depth", fmt.Sprintf("%d", input.Depth)}
	
	// Add branch if specified
	if input.Branch != "" {
		args = append(args, "--branch", input.Branch)
	}

	// Add repository and target directory
	args = append(args, input.Repository, clonePath)

	// Create and configure the command
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workingDir

	// Apply any additional git config
	if len(config.GitConfig) > 0 {
		for key, value := range config.GitConfig {
			configCmd := exec.CommandContext(ctx, "git", "config", "--global", key, value)
			if err := configCmd.Run(); err != nil {
				return GitShallowCloneOutput{}, fmt.Errorf("failed to set git config %s: %w", key, err)
			}
		}
	}

	// Execute the clone
	output, err := cmd.CombinedOutput()
	if err != nil {
		return GitShallowCloneOutput{}, fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	// Get the commit hash of the cloned repository
	hashCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	hashCmd.Dir = clonePath
	hashOutput, err := hashCmd.Output()
	if err != nil {
		return GitShallowCloneOutput{}, fmt.Errorf("failed to get commit hash: %w", err)
	}
	commitHash := string(hashOutput)
	if len(commitHash) > 0 && commitHash[len(commitHash)-1] == '\n' {
		commitHash = commitHash[:len(commitHash)-1]
	}

	// Get the current branch
	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = clonePath
	branchOutput, err := branchCmd.Output()
	if err != nil {
		return GitShallowCloneOutput{}, fmt.Errorf("failed to get branch name: %w", err)
	}
	branch := string(branchOutput)
	if len(branch) > 0 && branch[len(branch)-1] == '\n' {
		branch = branch[:len(branch)-1]
	}

	return GitShallowCloneOutput{
		ClonePath:     clonePath,
		CommitHash:    commitHash,
		Branch:        branch,
		CloneDepth:    input.Depth,
		RepositoryURL: input.Repository,
	}, nil
}