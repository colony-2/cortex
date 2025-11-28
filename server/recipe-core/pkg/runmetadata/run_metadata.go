package runmetadata

// Resume contains the execution path to be replayed when rewinding.
type Resume struct {
	ExecutionPath []Segment `json:"execution_path,omitempty"`
}

// Segment describes a single step in the execution path.
type Segment struct {
	InvocationHash string `json:"invocation_hash"`
	WorkflowID     string `json:"workflow_id,omitempty"`
	RunID          string `json:"run_id"`
	EventID        int64  `json:"event_id"`
	RecipeSetIndex *int   `json:"recipe_set_index,omitempty"`
	WaitForChild   *bool  `json:"wait_for_child,omitempty"`
}
