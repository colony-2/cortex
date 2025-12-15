package squashrebasemerge

import (
	"context"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

// SquashRebaseMergeInput defines the parameters for the squash-rebase-merge git op.
type SquashRebaseMergeInput struct {
	RepoPath       string                 `json:"repo_path,omitempty"`
	TargetBranch   string                 `json:"target_branch,omitempty"`
	UpstreamRemote string                 `json:"upstream_remote,omitempty"`
	PreserveAuthor *bool                  `json:"preserve_author,omitempty"`
	SkipRebase     bool                   `json:"skip_rebase,omitempty"`
	Context        map[string]interface{} `json:"context,omitempty"`
}

// SquashRangeSummary captures the original commit range that was squashed.
type SquashRangeSummary struct {
	BaseHash    string `json:"base_hash"`
	PersistHash string `json:"persist_hash"`
}

// SquashRebaseMergeOutput returns metadata about the merge operation.
type SquashRebaseMergeOutput struct {
	TargetBranch    string                 `json:"target_branch"`
	RemoteRef       string                 `json:"remote_ref"`
	MergedHash      string                 `json:"merged_hash"`
	SquashedCommits SquashRangeSummary     `json:"squashed_commits"`
	GitContextPatch map[string]interface{} `json:"git_context_patch,omitempty"`
	FastForward     bool                   `json:"fast_forward"`
}

// Activity implements the RegisterableOp interface for squash-rebase-merge.
type Activity struct{}

// NewActivity constructs the squash-rebase-merge activity wrapper.
func NewActivity() *Activity {
	return &Activity{}
}

// GetOp exposes the operation to recipe registration.
func GetOp() ops.RegisterableOp {
	return ops.NewActivityMappedOpV2[SquashRebaseMergeInput, SquashRebaseMergeOutput](NewActivity().GetMetadata(), func(inv ops.OpDependencies, ctx context.Context, input SquashRebaseMergeInput) (SquashRebaseMergeOutput, error) {
		result, err := Run(ctx, input)
		if err != nil {
			return SquashRebaseMergeOutput{}, err
		}
		return *result, nil
	})
}

// GetMetadata describes the operation for discovery and documentation.
func (a *Activity) GetMetadata() ops.OpMetadata {
	return ops.OpMetadata{
		Type:        "squashrebasemerge",
		Description: "Squashes local commits, rebases onto the latest target branch tip, and fast-forward merges back to the remote",
		Version:     "1.0.0",
	}
}
