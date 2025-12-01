package gitcommit

import (
	"time"
)

// PersistCommitInput defines the input for persist commit activities - ALL fields MUST have json tags
type PersistCommitInput struct {
	RepoPath        string        `json:"repo_path"`                // Required: path to the local Git repository
	StorageLocation string        `json:"storage_location"`         // Required: directory path where thin packs will be stored
	RootHash        string        `json:"root_hash"`                // Required: base commit hash this set was built upon
	CommitMessage   string        `json:"commit_message,omitempty"` // Optional: message for the commit
	Author          string        `json:"author,omitempty"`         // Optional: author name and email
	Timeout         time.Duration `json:"timeout,omitempty"`        // Optional: operation timeout
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
