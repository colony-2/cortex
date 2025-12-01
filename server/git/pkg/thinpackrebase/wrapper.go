package thinpackrebase

// ThinpackRebaseInput captures the parameters required to rebase a thin-pack backed workspace.
type ThinpackRebaseInput struct {
	RepoPath       string                 `json:"repo_path,omitempty"`
	TargetBaseHash string                 `json:"target_base_hash"`
	UpstreamRemote string                 `json:"upstream_remote,omitempty"`
	PreserveAuthor *bool                  `json:"preserve_author,omitempty"`
	UpdateRefs     string                 `json:"update_refs,omitempty"`
	Context        map[string]interface{} `json:"context,omitempty"`
}

// RebasedFromSummary records the base/persist pair prior to the rebase.
type RebasedFromSummary struct {
	BaseHash    string `json:"base_hash"`
	PersistHash string `json:"persist_hash"`
}

// ThinpackRebaseOutput returns details about the rebase along with metadata for downstream ops.
type ThinpackRebaseOutput struct {
	TargetBaseHash  string                 `json:"target_base_hash"`
	NewBaseHash     string                 `json:"new_base_hash"`
	NewPersistHash  string                 `json:"new_persist_hash"`
	UpdatedRef      string                 `json:"updated_ref,omitempty"`
	RebasedFrom     RebasedFromSummary     `json:"rebased_from"`
	GitContextPatch map[string]interface{} `json:"git_context_patch,omitempty"`
}
