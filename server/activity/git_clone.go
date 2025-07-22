package activity

import (
	"context"

	"github.com/divisive-ai/vibethis/server/activity/pkg/gitshallow"
)

// GitShallowCloneInput represents the input parameters for the GitShallowClone activity
type GitShallowCloneInput struct {
	SourceDir  string `json:"sourceDir"`
	TargetDir  string `json:"targetDir"`
	CommitHash string `json:"commitHash"`
}

// GitShallowCloneOutput represents the output of the GitShallowClone activity
type GitShallowCloneOutput struct {
	ClonedPath string `json:"clonedPath"`
}

// GitShallowClone performs a shallow clone of a local git repository to another directory
func GitShallowClone(ctx context.Context, input GitShallowCloneInput) (*GitShallowCloneOutput, error) {
	cloneInput := gitshallow.CloneInput{
		SourceDir:  input.SourceDir,
		TargetDir:  input.TargetDir,
		CommitHash: input.CommitHash,
	}

	output, err := gitshallow.Clone(ctx, cloneInput)
	if err != nil {
		return nil, err
	}

	return &GitShallowCloneOutput{
		ClonedPath: output.ClonedPath,
	}, nil
}