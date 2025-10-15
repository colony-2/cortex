package storybuilder

import "time"

// Story represents the reconstructed execution story for a workflow run.
type Story struct {
	Metadata StoryMetadata   `json:"metadata"`
	Nodes    []*StoryNode    `json:"nodes"`
	Timeline []TimelineEntry `json:"timeline"`
	Indexes  StoryIndexes    `json:"-"`
}

// StoryIndexes provides O(1) lookup helpers for common run queries.
type StoryIndexes struct {
	ByInvocationID   map[string]*NodeRun `json:"-"`
	ByInvocationHash map[string]*NodeRun `json:"-"`
	ByChildRunID     map[string]*NodeRun `json:"-"`
}

// StoryMetadata captures top-level workflow execution metadata.
type StoryMetadata struct {
	RecipeName  string         `json:"recipe_name"`
	WorkflowID  string         `json:"workflow_id"`
	RunID       string         `json:"run_id"`
	Status      string         `json:"status"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Duration    *time.Duration `json:"duration,omitempty"`
	Parent      *StoryParentLink `json:"parent,omitempty"`
}

// StoryParentLink captures linkage to the parent recipe run when present.
type StoryParentLink struct {
	WorkflowID     string `json:"workflow_id"`
	RunID          string `json:"run_id"`
	InvocationHash string `json:"invocation_hash"`
	RecipeSetIndex *int   `json:"recipe_set_index,omitempty"`
}

// StoryNode represents a recipe node (op/sequence/state) and its execution runs.
type StoryNode struct {
	Path        string       `json:"path"`
	Type        string       `json:"type"`
	DisplayName string       `json:"display_name"`
	Runs        []*NodeRun   `json:"runs"`
	Events      []StoryEvent `json:"events"`
	Children    []*StoryNode `json:"children"`
}

// NodeRun represents a single invocation of a node (activity or inline op).
type NodeRun struct {
	InvocationID    string                 `json:"invocation_id"`
	InvocationHash  string                 `json:"invocation_hash"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	Status          string                 `json:"status"`
	Attempt         int                    `json:"attempt"`
	Inputs          map[string]interface{} `json:"inputs,omitempty"`
	Outputs         map[string]interface{} `json:"outputs,omitempty"`
	Error           string                 `json:"error,omitempty"`
	ChildWorkflowID string                 `json:"child_workflow_id,omitempty"`
	ChildRunID      string                 `json:"child_run_id,omitempty"`
	ChildRecipeName string                 `json:"child_recipe_name,omitempty"`
	ResumeEventID   int64                  `json:"resume_event_id,omitempty"`
	ScheduleEventID int64                  `json:"schedule_event_id,omitempty"`
	MarkerEventID   int64                  `json:"marker_event_id,omitempty"`
	RecipeSetIndex  *int                   `json:"recipe_set_index,omitempty"`
	ParentWorkflowID     string `json:"parent_workflow_id,omitempty"`
	ParentRunID          string `json:"parent_run_id,omitempty"`
	ParentInvocationHash string `json:"parent_invocation_hash,omitempty"`
	WaitForChild         *bool  `json:"wait_for_child,omitempty"`
}

// StoryEvent captures notable inline events associated with a node run.
type StoryEvent struct {
	Kind string                 `json:"kind"`
	At   time.Time              `json:"at"`
	Data map[string]interface{} `json:"data,omitempty"`
}

// TimelineEntry provides a flat, chronological representation of events.
type TimelineEntry struct {
	EventID int64                  `json:"event_id"`
	At      time.Time              `json:"at"`
	Kind    string                 `json:"kind"`
	Ref     string                 `json:"ref"`
	Data    map[string]interface{} `json:"data,omitempty"`
}
