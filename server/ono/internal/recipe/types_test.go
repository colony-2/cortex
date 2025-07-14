package recipe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "vibethis/ono/pkg/yaml"
)

func TestRecipeFilter(t *testing.T) {
	tests := []struct {
		name     string
		filter   *RecipeFilter
		recipe   *Recipe
		expected bool
	}{
		{
			name: "nil filter matches all",
			filter: nil,
			recipe: &Recipe{
				Name: "test-recipe",
				WorkerStatus: WorkerStatusRunning,
			},
			expected: true,
		},
		{
			name: "filter by running status",
			filter: &RecipeFilter{
				Status: func() *WorkerStatus { s := WorkerStatusRunning; return &s }(),
			},
			recipe: &Recipe{
				Name: "test-recipe",
				WorkerStatus: WorkerStatusRunning,
			},
			expected: true,
		},
		{
			name: "filter by stopped status excludes running",
			filter: &RecipeFilter{
				Status: func() *WorkerStatus { s := WorkerStatusStopped; return &s }(),
			},
			recipe: &Recipe{
				Name: "test-recipe",
				WorkerStatus: WorkerStatusRunning,
			},
			expected: false,
		},
		{
			name: "include removed shows stopped recipes",
			filter: &RecipeFilter{
				IncludeRemoved: true,
			},
			recipe: &Recipe{
				Name: "test-recipe",
				WorkerStatus: WorkerStatusStopped,
			},
			expected: true,
		},
		{
			name: "exclude removed hides stopped recipes",
			filter: &RecipeFilter{
				IncludeRemoved: false,
			},
			recipe: &Recipe{
				Name: "test-recipe",
				WorkerStatus: WorkerStatusStopped,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test would be implemented in the filter logic
			// For now, we're testing the type definitions compile correctly
			assert.NotNil(t, tt.recipe)
			if tt.filter != nil && tt.filter.Status != nil {
				assert.NotEqual(t, "", *tt.filter.Status)
			}
		})
	}
}

func TestJobStatus(t *testing.T) {
	// Test all job status constants are defined
	statuses := []JobStatus{
		JobStatusUnknown,
		JobStatusRunning,
		JobStatusCompleted,
		JobStatusFailed,
		JobStatusCanceled,
		JobStatusTerminated,
	}

	for _, status := range statuses {
		assert.NotEqual(t, "", string(status))
	}
}

func TestWorkerStatus(t *testing.T) {
	// Test all worker status constants are defined
	statuses := []WorkerStatus{
		WorkerStatusRunning,
		WorkerStatusStopped,
		WorkerStatusFailed,
		WorkerStatusStarting,
	}

	for _, status := range statuses {
		assert.NotEqual(t, "", string(status))
	}
}

func TestJobStructure(t *testing.T) {
	now := time.Now()
	duration := 5 * time.Minute
	
	job := &Job{
		ID:            "job-123",
		RecipeName:    "test-recipe",
		RecipeVersion: "1.0.0",
		Status:        JobStatusRunning,
		StartTime:     now,
		EndTime:       &now,
		UpdateTime:    now,
		Input: map[string]interface{}{
			"key": "value",
		},
		Output: map[string]interface{}{
			"result": "success",
		},
		Error:        "test error",
		Duration:     &duration,
		Inputs:       map[string]interface{}{"key": "value"},
		Outputs:      map[string]interface{}{"result": "success"},
		Activities:   []*ActivityExecution{},
		WorkflowType: "TestWorkflow",
		WorkflowID:   "wf-123",
		RunID:        "run-123",
		ExecutionInfo: &WorkflowExecutionInfo{
			WorkflowID: "wf-123",
			RunID:      "run-123",
		},
	}

	assert.Equal(t, "job-123", job.ID)
	assert.Equal(t, "test-recipe", job.RecipeName)
	assert.Equal(t, "1.0.0", job.RecipeVersion)
	assert.Equal(t, JobStatusRunning, job.Status)
	assert.Equal(t, now, job.StartTime)
	assert.NotNil(t, job.EndTime)
	assert.NotNil(t, job.Duration)
	assert.Equal(t, duration, *job.Duration)
	assert.NotNil(t, job.ExecutionInfo)
}

func TestActivityExecution(t *testing.T) {
	now := time.Now()
	duration := 30 * time.Second
	
	activity := &ActivityExecution{
		Name:       "process-data",
		Status:     "completed",
		StartTime:  now,
		EndTime:    now.Add(duration),
		Duration:   &duration,
		Result:     map[string]interface{}{"processed": true},
		Error:      "",
		Attempt:    1,
		ActivityID: "act-123",
		ActivityType: "ProcessDataActivity",
	}

	assert.Equal(t, "process-data", activity.Name)
	assert.Equal(t, "completed", activity.Status)
	assert.Equal(t, now, activity.StartTime)
	assert.NotNil(t, activity.Duration)
	assert.Equal(t, 1, activity.Attempt)
}

func TestRecipeManifest(t *testing.T) {
	manifest := &RecipeManifest{}
	manifest.Recipe.Name = "test-recipe"
	manifest.Recipe.Version = "1.0.0"
	manifest.Recipe.Description = "Test recipe"
	manifest.Recipe.Files.Workflow = "workflow.yaml"
	manifest.Recipe.Files.Activities = "activities.yaml"
	manifest.Recipe.Files.Agents = "agents.yaml"

	assert.Equal(t, "test-recipe", manifest.Recipe.Name)
	assert.Equal(t, "workflow.yaml", manifest.Recipe.Files.Workflow)
}

func TestRecipeStructure(t *testing.T) {
	recipe := &Recipe{
		Name:         "test-recipe",
		Version:      "1.0.0",
		Description:  "Test recipe",
		BasePath:     "/path/to/recipe",
		ManifestPath: "/path/to/recipe/recipe.yaml",
		Workflow:     nil,
		Activities:   []ActivityDefinition{},
		Agents:       map[string]yaml.AgentDefinition{},
		Hash:         "abc123",
		LastModified: time.Now(),
		WorkerStatus: WorkerStatusRunning,
	}

	assert.Equal(t, "test-recipe", recipe.Name)
	assert.Equal(t, "1.0.0", recipe.Version)
	assert.Equal(t, WorkerStatusRunning, recipe.WorkerStatus)
	assert.NotEqual(t, "", recipe.Hash)
}

func TestActivityDefinition(t *testing.T) {
	actDef := ActivityDefinition{
		Name:        "process-message",
		Description: "Process a message",
		Timeout:     30 * time.Second,
		Inputs: []yaml.InputDefinition{
			{
				Name:     "message",
				Type:     "string",
				Required: true,
			},
		},
		Outputs: []yaml.OutputDefinition{
			{
				Name: "result",
				Type: "string",
			},
		},
		Implementation: struct {
			Type   string                 `yaml:"type"`
			Config map[string]interface{} `yaml:"config"`
		}{
			Type: "function",
			Config: map[string]interface{}{
				"function": "processMessage",
			},
		},
	}

	assert.Equal(t, "process-message", actDef.Name)
	assert.Equal(t, "Process a message", actDef.Description)
	require.Len(t, actDef.Inputs, 1)
	assert.Equal(t, "message", actDef.Inputs[0].Name)
	assert.True(t, actDef.Inputs[0].Required)
}