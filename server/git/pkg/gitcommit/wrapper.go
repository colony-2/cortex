package gitcommit

import (
	"context"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// PersistCommitConfig defines the configuration for persist commit activities - ALL fields MUST have json tags
type PersistCommitConfig struct {
	// No configuration needed for this activity
}

// PersistCommitInput defines the input for persist commit activities - ALL fields MUST have json tags
type PersistCommitInput struct {
	RepoPath        string        `json:"repo_path"`                // Required: path to the local Git repository
	StorageLocation string        `json:"storage_location"`         // Required: directory path where thin packs will be stored
	RootHash        string        `json:"root_hash"`                // Required: base commit hash this set was built upon
	CommitMessage   string        `json:"commit_message,omitempty"` // Optional: message for the commit
	Author          string        `json:"author,omitempty"`         // Optional: author name and email
	Timeout         time.Duration `json:"timeout,omitempty"`        // Optional: operation timeout
}

// PersistCommitActivityWrapper implements the RegisterableOp interface
type PersistCommitActivityWrapper struct{}

// NewPersistCommitActivity creates a new persist commit activity that implements RegisterableOp
func NewPersistCommitActivity() *PersistCommitActivityWrapper {
	return &PersistCommitActivityWrapper{}
}

func GetPersistOp() types.RegisterableOp {
	p := &PersistCommitActivityWrapper{}
	return types.NewRegisterableOp(p.GetMetadata(), p.Execute)
}

// GetMetadata returns activity metadata for registration
func (a *PersistCommitActivityWrapper) GetMetadata() types.OpMetadata {
	return types.OpMetadata{
		Type:           "git_persist_commit",
		Name:           "Git Persist Commit",
		Description:    "Capture a Git commit and generate a portable thin pack for external storage",
		Version:        "1.0.0",
		DefaultTimeout: 5 * time.Minute,
		RetryPolicy: &yaml.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			NonRetryableErrorTypes: []string{
				"InvalidRepositoryError",
				"InvalidRootHashError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *PersistCommitActivityWrapper) Execute(ctx context.Context, config PersistCommitConfig, input PersistCommitInput) (PersistCommitOutput, error) {
	// Build the persist input
	persistInput := PersistCommitActivity{
		RepoPath:        input.RepoPath,
		StorageLocation: input.StorageLocation,
		RootHash:        input.RootHash,
		CommitMessage:   input.CommitMessage,
		Author:          input.Author,
		Timeout:         input.Timeout,
	}

	// Execute the persist operation
	output, err := PersistCommit(ctx, persistInput)
	if err != nil {
		return PersistCommitOutput{}, err
	}

	return *output, nil
}

// RestoreCommitConfig defines the configuration for restore commit activities - ALL fields MUST have json tags
type RestoreCommitConfig struct {
	// No configuration needed for this activity
}

// RestoreCommitInput defines the input for restore commit activities - ALL fields MUST have json tags
type RestoreCommitInput struct {
	RepoPath        string        `json:"repo_path"`         // Required: path to the local Git repository
	TargetCommit    string        `json:"target_commit"`     // Required: commit hash to restore to
	RootHash        string        `json:"root_hash"`         // Required: root commit hash for this set
	StorageLocation string        `json:"storage_location"`  // Required: directory containing thin packs
	Force           bool          `json:"force,omitempty"`   // Optional: force checkout even with uncommitted changes
	Timeout         time.Duration `json:"timeout,omitempty"` // Optional: operation timeout
}

// RestoreCommitActivityWrapper implements the RegisterableOp interface
type RestoreCommitActivityWrapper struct{}

// NewRestoreCommitActivity creates a new restore commit activity that implements RegisterableOp
func NewRestoreCommitActivity() *RestoreCommitActivityWrapper {
	return &RestoreCommitActivityWrapper{}
}

func GetRestoreOp() types.RegisterableOp {
	r := &RestoreCommitActivityWrapper{}
	return types.NewRegisterableOp(r.GetMetadata(), r.Execute)
}

// GetMetadata returns activity metadata for registration
func (a *RestoreCommitActivityWrapper) GetMetadata() types.OpMetadata {
	return types.OpMetadata{
		Type:           "git_restore_commit",
		Name:           "Git Restore Commit",
		Description:    "Restore a specific commit state, rebuilding from thin packs if necessary",
		Version:        "1.0.0",
		DefaultTimeout: 5 * time.Minute,
		RetryPolicy: &yaml.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			NonRetryableErrorTypes: []string{
				"InvalidRepositoryError",
				"CommitNotFoundError",
				"UncommittedChangesError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *RestoreCommitActivityWrapper) Execute(ctx context.Context, config RestoreCommitConfig, input RestoreCommitInput) (RestoreCommitOutput, error) {
	// Build the restore input
	restoreInput := RestoreCommitActivity{
		RepoPath:        input.RepoPath,
		TargetCommit:    input.TargetCommit,
		RootHash:        input.RootHash,
		StorageLocation: input.StorageLocation,
		Force:           input.Force,
		Timeout:         input.Timeout,
	}

	// Execute the restore operation
	output, err := RestoreCommit(ctx, restoreInput)
	if err != nil {
		return RestoreCommitOutput{}, err
	}

	return *output, nil
}
