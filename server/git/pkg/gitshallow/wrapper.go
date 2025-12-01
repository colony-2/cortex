package gitshallow

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
