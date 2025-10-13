package runmetadata

// Signal models the payload delivered via the recipe_run_metadata signal.
type Signal struct {
	TargetRunID string  `json:"target_run_id"`
	Parent      *Parent `json:"parent,omitempty"`
	Resume      *Resume `json:"resume,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

// Parent captures linkage to the parent workflow invocation.
type Parent struct {
	WorkflowID     string `json:"workflow_id"`
	RunID          string `json:"run_id"`
	InvocationHash string `json:"invocation_hash"`
	RecipeSetIndex *int   `json:"recipe_set_index,omitempty"`
}

// Resume contains the execution path to be replayed when rewinding.
type Resume struct {
	ExecutionPath []Segment `json:"execution_path,omitempty"`
}

// Segment describes a single step in the execution path.
type Segment struct {
	InvocationHash string `json:"invocation_hash"`
	RunID          string `json:"run_id"`
	EventID        int64  `json:"event_id"`
	RecipeSetIndex *int   `json:"recipe_set_index,omitempty"`
}
