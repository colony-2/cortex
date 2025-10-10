package storybuilder

import "time"

// Story represents the reconstructed execution story for a workflow run.
type Story struct {
	Metadata StoryMetadata   `json:"metadata"`
	Nodes    []*StoryNode    `json:"nodes"`
	Timeline []TimelineEntry `json:"timeline"`
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
	InvocationID string                 `json:"invocation_id"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Status       string                 `json:"status"`
	Attempt      int                    `json:"attempt"`
	Inputs       map[string]interface{} `json:"inputs,omitempty"`
	Outputs      map[string]interface{} `json:"outputs,omitempty"`
	Error        string                 `json:"error,omitempty"`
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
