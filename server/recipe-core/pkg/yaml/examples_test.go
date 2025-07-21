package yaml

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseExampleFiles(t *testing.T) {
	// Find the examples directory relative to this test file
	examplesDir := filepath.Join("..", "..", "examples")
	
	// Check if examples directory exists
	if _, err := os.Stat(examplesDir); os.IsNotExist(err) {
		t.Skip("Examples directory not found")
	}
	
	parser := NewParser()
	
	tests := []struct {
		name     string
		filename string
		validate func(t *testing.T, project *Project)
	}{
		{
			name:     "minimal_workflow",
			filename: "minimal_workflow.yaml",
			validate: func(t *testing.T, project *Project) {
				require.NotNil(t, project.Workflow)
				assert.Equal(t, "hello_world", project.Workflow.Name)
				assert.Equal(t, "A minimal workflow that demonstrates basic structure", project.Workflow.Description)
				assert.Equal(t, "1.0", project.Workflow.Version)
				
				// Check inputs
				require.Len(t, project.Workflow.Inputs, 1)
				assert.Equal(t, "name", project.Workflow.Inputs[0].Name)
				assert.Equal(t, "string", project.Workflow.Inputs[0].Type)
				assert.True(t, project.Workflow.Inputs[0].Required)
				
				// Check workflow steps
				require.Len(t, project.Workflow.Workflow.Steps, 1)
				assert.Equal(t, "greet", project.Workflow.Workflow.Steps[0].ID)
				assert.Equal(t, "echo", project.Workflow.Workflow.Steps[0].Activity)
				
				// Check activities
				require.Len(t, project.Activities, 1)
				assert.Equal(t, "echo", project.Activities[0].Name)
				assert.Equal(t, "Simple echo activity", project.Activities[0].Description)
			},
		},
		{
			name:     "simple_workflow",
			filename: "simple_workflow.yaml",
			validate: func(t *testing.T, project *Project) {
				require.NotNil(t, project.Workflow)
				assert.Equal(t, "simple_research", project.Workflow.Name)
				assert.Equal(t, "A simple research workflow in a single file", project.Workflow.Description)
				assert.Equal(t, "1.0", project.Workflow.Version)
				
				// Check inputs
				require.Len(t, project.Workflow.Inputs, 1)
				assert.Equal(t, "query", project.Workflow.Inputs[0].Name)
				assert.Equal(t, "string", project.Workflow.Inputs[0].Type)
				assert.True(t, project.Workflow.Inputs[0].Required)
				
				// Check workflow has two sequential steps
				require.Len(t, project.Workflow.Workflow.Steps, 2)
				assert.Equal(t, "search", project.Workflow.Workflow.Steps[0].ID)
				assert.Equal(t, "summarize", project.Workflow.Workflow.Steps[1].ID)
				
				// Check activities
				require.Len(t, project.Activities, 2)
				activityNames := []string{project.Activities[0].Name, project.Activities[1].Name}
				assert.Contains(t, activityNames, "quick_search")
				assert.Contains(t, activityNames, "summarize_results")
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath := filepath.Join(examplesDir, tt.filename)
			
			// Parse the file
			project, err := parser.ParseProject(filepath)
			require.NoError(t, err, "Failed to parse %s", tt.filename)
			require.NotNil(t, project)
			
			// Run specific validations
			tt.validate(t, project)
		})
	}
}

func TestParseExampleFilesValid(t *testing.T) {
	// Test that all YAML files in examples directory are valid
	examplesDir := filepath.Join("..", "..", "examples")
	
	// Check if examples directory exists
	if _, err := os.Stat(examplesDir); os.IsNotExist(err) {
		t.Skip("Examples directory not found")
	}
	
	parser := NewParser()
	
	// Read all .yaml files in the examples directory
	files, err := os.ReadDir(examplesDir)
	require.NoError(t, err)
	
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
			t.Run(file.Name(), func(t *testing.T) {
				filepath := filepath.Join(examplesDir, file.Name())
				
				// Parse the file
				project, err := parser.ParseProject(filepath)
				require.NoError(t, err, "Failed to parse %s", file.Name())
				require.NotNil(t, project)
				
				// Basic validation - every example should have a workflow
				require.NotNil(t, project.Workflow, "Example %s should have a workflow", file.Name())
				assert.NotEmpty(t, project.Workflow.Name, "Workflow in %s should have a name", file.Name())
				assert.NotEmpty(t, project.Workflow.Version, "Workflow in %s should have a version", file.Name())
			})
		}
	}
}