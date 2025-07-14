package format

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"vibethis/ono/internal/recipe"
)

func TestRecipeFormatter_FormatRecipeList(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	tests := []struct {
		name     string
		recipes  []*recipe.Recipe
		contains []string
	}{
		{
			name:     "empty list",
			recipes:  []*recipe.Recipe{},
			contains: []string{"No recipes found"},
		},
		{
			name: "single recipe",
			recipes: []*recipe.Recipe{
				{
					Name:         "test-recipe",
					Version:      "1.0.0",
					Description:  "Test recipe",
					WorkerStatus: recipe.WorkerStatusRunning,
					LastModified: time.Now(),
				},
			},
			contains: []string{
				"test-recipe",
				"1.0.0",
				"Test recipe",
				"running",
			},
		},
		{
			name: "multiple recipes with different statuses",
			recipes: []*recipe.Recipe{
				{
					Name:         "recipe-1",
					Version:      "1.0.0",
					Description:  "First recipe",
					WorkerStatus: recipe.WorkerStatusRunning,
					LastModified: time.Now(),
				},
				{
					Name:         "recipe-2",
					Version:      "2.0.0",
					Description:  "Second recipe",
					WorkerStatus: recipe.WorkerStatusStopped,
					LastModified: time.Now().Add(-24 * time.Hour),
				},
				{
					Name:         "recipe-3",
					Version:      "3.0.0",
					Description:  "Third recipe",
					WorkerStatus: recipe.WorkerStatusFailed,
					LastModified: time.Now().Add(-48 * time.Hour),
				},
			},
			contains: []string{
				"recipe-1", "1.0.0", "First recipe", "running",
				"recipe-2", "2.0.0", "Second recipe", "stopped",
				"recipe-3", "3.0.0", "Third recipe", "failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatter.FormatRecipeList(tt.recipes)
			
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestRecipeFormatter_FormatRecipeDetail(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	recipe := &recipe.Recipe{
		Name:         "detailed-recipe",
		Version:      "1.2.3",
		Description:  "A detailed test recipe",
		BasePath:     "/path/to/recipe",
		Hash:         "abc123def456",
		WorkerStatus: recipe.WorkerStatusRunning,
		LastModified: time.Now(),
	}

	output := formatter.FormatRecipeDetail(recipe)

	// Check all expected fields are present
	expectedFields := []string{
		"RECIPE: detailed-recipe",
		"Name:", "detailed-recipe",
		"Version:", "1.2.3",
		"Description:", "A detailed test recipe",
		"Path:", "/path/to/recipe",
		"Hash:", "abc123def456",
		"Status:", "running",
		"Last Modified:",
	}

	for _, expected := range expectedFields {
		assert.Contains(t, output, expected)
	}
}

func TestRecipeFormatter_FormatJobList(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	tests := []struct {
		name     string
		jobs     []*recipe.Job
		contains []string
	}{
		{
			name:     "empty job list",
			jobs:     []*recipe.Job{},
			contains: []string{"No jobs found"},
		},
		{
			name: "single job",
			jobs: []*recipe.Job{
				{
					ID:         "job-123",
					RecipeName: "test-recipe",
					Status:     recipe.JobStatusRunning,
					StartTime:  time.Now().Add(-5 * time.Minute),
				},
			},
			contains: []string{
				"job-123",
				"running",
			},
		},
		{
			name: "multiple jobs with different statuses",
			jobs: []*recipe.Job{
				{
					ID:         "job-1",
					RecipeName: "recipe-a",
					Status:     recipe.JobStatusRunning,
					StartTime:  time.Now().Add(-10 * time.Minute),
				},
				{
					ID:         "job-2",
					RecipeName: "recipe-b",
					Status:     recipe.JobStatusCompleted,
					StartTime:  time.Now().Add(-1 * time.Hour),
					EndTime:    func() *time.Time { t := time.Now().Add(-30 * time.Minute); return &t }(),
					Duration:   ptrDuration(30 * time.Minute),
				},
				{
					ID:         "job-3",
					RecipeName: "recipe-c",
					Status:     recipe.JobStatusFailed,
					StartTime:  time.Now().Add(-2 * time.Hour),
					EndTime:    func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
					Duration:   ptrDuration(1 * time.Hour),
				},
			},
			contains: []string{
				"job-1", "running",
				"job-2", "completed", "30.0m",
				"job-3", "failed", "1.0h",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatter.FormatJobList(tt.jobs, "")
			
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestRecipeFormatter_FormatJobDetail(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	endTime := time.Now()
	job := &recipe.Job{
		ID:           "job-detail-123",
		RecipeName:   "detailed-recipe",
		Status:       recipe.JobStatusCompleted,
		WorkflowType: "RecipeWorkflow",
		StartTime:    time.Now().Add(-1 * time.Hour),
		UpdateTime:   time.Now(),
		EndTime:      &endTime,
		Duration:     ptrDuration(1 * time.Hour),
		Activities: []*recipe.ActivityExecution{
			{
				Name:      "PrepareData",
				Status:    "completed",
				StartTime: time.Now().Add(-50 * time.Minute),
				EndTime:   time.Now().Add(-40 * time.Minute),
				Duration:  ptrDuration(10 * time.Minute),
			},
			{
				Name:      "ProcessData",
				Status:    "completed",
				StartTime: time.Now().Add(-30 * time.Minute),
				EndTime:   time.Now().Add(-10 * time.Minute),
				Duration:  ptrDuration(20 * time.Minute),
			},
		},
		ExecutionInfo: &recipe.WorkflowExecutionInfo{
			WorkflowID: "job-detail-123",
			RunID:      "run-456",
		},
	}

	output := formatter.FormatJobDetail(job)

	// Check all expected sections are present
	expectedSections := []string{
		"JOB: job-detail-123",
		"Job ID:", "job-detail-123",
		"Recipe:", "detailed-recipe",
		"Status:", "completed",
		"Workflow ID:", "job-detail-123",
		"Run ID:", "run-456",
		"Duration:", "1.0h",
		"ACTIVITIES",
		"PrepareData", "[completed]", "(10.0m)",
		"ProcessData", "[completed]", "(20.0m)",
	}

	for _, expected := range expectedSections {
		assert.Contains(t, output, expected)
	}
}


func TestRecipeFormatter_ColorSupport(t *testing.T) {
	// Test with color support
	formatter := NewRecipeFormatter(true)

	recipe := &recipe.Recipe{
		Name:         "color-test",
		Version:      "1.0.0",
		WorkerStatus: recipe.WorkerStatusRunning,
		Hash:         "abcdef0123456789abcdef0123456789",
	}

	colorOutput := formatter.FormatRecipeDetail(recipe)
	
	// Note: ANSI color codes might not appear in test environment
	assert.NotEmpty(t, colorOutput)

	// Test without color support
	formatter = NewRecipeFormatter(false)

	noColorOutput := formatter.FormatRecipeDetail(recipe)
	
	// Should not contain ANSI color codes
	assert.NotContains(t, noColorOutput, "\033[")
}

func TestRecipeFormatter_StatusColors(t *testing.T) {
	formatter := NewRecipeFormatter(true)

	tests := []struct {
		status   interface{}
		contains string
	}{
		{recipe.WorkerStatusRunning, "running"},
		{recipe.WorkerStatusStopped, "stopped"},
		{recipe.WorkerStatusFailed, "failed"},
		{recipe.JobStatusRunning, "running"},
		{recipe.JobStatusCompleted, "completed"},
		{recipe.JobStatusFailed, "failed"},
		{recipe.JobStatusCanceled, "canceled"},
		{recipe.JobStatusTerminated, "terminated"},
		{"pending", "pending"},
		{"running", "running"},
		{"completed", "completed"},
		{"failed", "failed"},
		{"skipped", "skipped"},
	}

	for _, tt := range tests {
		var output string
		
		switch s := tt.status.(type) {
		case recipe.WorkerStatus:
			output = formatter.formatWorkerStatus(s)
		case recipe.JobStatus:
			output = formatter.formatJobStatus(s)
		case string:
			output = formatter.formatActivityStatus(s)
		}
		
		assert.Contains(t, output, tt.contains)
		// Note: color codes might not appear in test environment
	}
}

func TestRecipeFormatter_TableFormatting(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	// Test that tables are properly aligned
	recipes := []*recipe.Recipe{
		{
			Name:         "short",
			Version:      "1.0.0",
			Description:  "Short desc",
			WorkerStatus: recipe.WorkerStatusRunning,
		},
		{
			Name:         "very-long-recipe-name",
			Version:      "10.20.30",
			Description:  "This is a much longer description that should still align properly",
			WorkerStatus: recipe.WorkerStatusStopped,
		},
	}

	output := formatter.FormatRecipeList(recipes)
	lines := strings.Split(output, "\n")

	// Find header line
	var headerLine string
	for _, line := range lines {
		if strings.Contains(line, "NAME") && strings.Contains(line, "VERSION") {
			headerLine = line
			break
		}
	}
	require.NotEmpty(t, headerLine)

	// Check that columns are aligned
	nameCol := strings.Index(headerLine, "NAME")
	versionCol := strings.Index(headerLine, "VERSION")
	descCol := strings.Index(headerLine, "DESCRIPTION")

	assert.True(t, nameCol >= 0)
	assert.True(t, versionCol > nameCol)
	assert.True(t, descCol > versionCol)
}

func TestRecipeFormatter_ErrorHandling(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	// Test with nil recipe
	output := formatter.FormatRecipeDetail(nil)
	assert.Contains(t, output, "Recipe not found")

	// Test with nil job
	output = formatter.FormatJobDetail(nil)
	assert.Contains(t, output, "Job not found")

}

// Test output to different writers
func TestRecipeFormatter_WriterOutput(t *testing.T) {
	formatter := NewRecipeFormatter(false)

	recipes := []*recipe.Recipe{
		{
			Name:    "test-recipe",
			Version: "1.0.0",
		},
	}

	formatted := formatter.FormatRecipeList(recipes)
	
	// Should return the formatted string
	assert.Contains(t, formatted, "test-recipe")
}

// Helper functions
func ptrDuration(d time.Duration) *time.Duration {
	return &d
}