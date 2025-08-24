package gitshallow

import (
	"context"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
)

// GitShallowConfig defines the configuration for git shallow clone activities - ALL fields MUST have json tags
type GitShallowConfig struct {
	// No configuration needed for this activity
}

// GitShallowInput defines the input for git shallow clone activities - ALL fields MUST have json tags
type GitShallowInput struct {
	SourceDir  string `json:"source_dir"`  // Required: path to source git repository
	TargetDir  string `json:"target_dir"`  // Required: path where to clone
	CommitHash string `json:"commit_hash"` // Required: commit hash to checkout
}

// GitShallowOutput defines the output from git shallow clone activities - ALL fields MUST have json tags
type GitShallowOutput struct {
	ClonedPath string `json:"cloned_path"` // Path to the cloned repository
}

// GitShallowActivityWrapper implements the RegisterableOp interface
type GitShallowActivityWrapper struct{}

// NewGitShallowActivity creates a new git shallow clone activity that implements RegisterableOp
func NewGitShallowActivity() *GitShallowActivityWrapper {
	return &GitShallowActivityWrapper{}
}

func GetOp() types.RegisterableOp {
	a := NewGitShallowActivity()
	return types.NewRegisterableOp(a.GetMetadata(), a.Execute)
}

// GetMetadata returns activity metadata for registration
func (a *GitShallowActivityWrapper) GetMetadata() types.OpMetadata {
	return types.OpMetadata{
		Type:           "git_shallow_clone",
		Name:           "Git Shallow Clone",
		Description:    "Performs a shallow clone of a local git repository to another directory at a specific commit",
		Version:        "1.0.0",
		DefaultTimeout: 2 * time.Minute,
		RetryPolicy: &types.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			NonRetryableErrorTypes: []string{
				"InvalidInputError",
				"DirectoryExistsError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *GitShallowActivityWrapper) Execute(ctx context.Context, config GitShallowConfig, input GitShallowInput) (GitShallowOutput, error) {
	// Build the clone input
	cloneInput := GitShallowCloneInput{
		SourceDir:  input.SourceDir,
		TargetDir:  input.TargetDir,
		CommitHash: input.CommitHash,
	}

	// Execute the clone
	output, err := GitShallowClone(ctx, cloneInput)
	if err != nil {
		return GitShallowOutput{}, err
	}

	return GitShallowOutput{
		ClonedPath: output.ClonedPath,
	}, nil
}
