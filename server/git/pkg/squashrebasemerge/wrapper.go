package squashrebasemerge

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
