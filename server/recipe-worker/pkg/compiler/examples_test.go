package compiler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

func TestCompileExampleWorkflows(t *testing.T) {
	// Find the examples directory relative to this test file
	examplesDir := filepath.Join("..", "..", "examples")
	
	// Check if examples directory exists
	if _, err := os.Stat(examplesDir); os.IsNotExist(err) {
		t.Skip("Examples directory not found")
	}
	
	parser := yamlpkg.NewParser()
	
	tests := []struct {
		name     string
		filename string
		setup    func(*ActivityRegistry)
	}{
		{
			name:     "parallel_workflow",
			filename: "parallel_workflow.yaml",
			setup: func(r *ActivityRegistry) {
				// Register activities used in the example
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "validate_sources", Timeout: 30 * time.Second})
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "process_data", Timeout: 2 * time.Minute})
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "combine_results", Timeout: time.Minute})
			},
		},
		{
			name:     "template_features",
			filename: "template_features.yaml",
			setup: func(r *ActivityRegistry) {
				// Register activities used in the example
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "prepare_data", Timeout: 30 * time.Second})
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "format_text", Timeout: 10 * time.Second})
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "analyze_data", Timeout: time.Minute})
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "create_summary", Timeout: 30 * time.Second})
			},
		},
		{
			name:     "gemini_workflow",
			filename: "gemini_workflow.yaml",
			setup: func(r *ActivityRegistry) {
				// Register activities for Gemini workflow
				r.RegisterActivity(&recipe.ActivityDefinition{Name: "gemini_generate", Timeout: 2 * time.Minute})
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
			require.NotNil(t, project.Workflow)
			
			// Setup activities for this test
			registry := NewActivityRegistry()
			if tt.setup != nil {
				tt.setup(registry)
			}
			
			// Register all activities from the project
			for _, activity := range project.Activities {
				registry.RegisterActivity(&recipe.ActivityDefinition{
					Name:        activity.Name,
					Description: activity.Description,
					Timeout:     activity.Timeout,
				})
			}
			
			// Compile the workflow
			compiler := NewCompiler(registry)
			workflowFunc, err := compiler.CompileWorkflow(project.Workflow)
			require.NoError(t, err, "Failed to compile workflow from %s", tt.filename)
			assert.NotNil(t, workflowFunc)
		})
	}
}

func TestCompileResearchProjectExample(t *testing.T) {
	// Special test for the research project directory structure
	projectDir := filepath.Join("..", "..", "examples", "research_project")
	
	// Check if project directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		t.Skip("Research project example directory not found")
	}
	
	parser := yamlpkg.NewParser()
	
	// Parse the project directory
	project, err := parser.ParseProject(projectDir)
	require.NoError(t, err, "Failed to parse research project")
	require.NotNil(t, project)
	require.NotNil(t, project.Workflow)
	
	// Create registry and register all activities
	registry := NewActivityRegistry()
	for _, activity := range project.Activities {
		registry.RegisterActivity(&recipe.ActivityDefinition{
			Name:        activity.Name,
			Description: activity.Description,
			Timeout:     activity.Timeout,
		})
	}
	
	// Compile the workflow
	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(project.Workflow)
	require.NoError(t, err, "Failed to compile research project workflow")
	assert.NotNil(t, workflowFunc)
	
	// Verify the workflow has expected characteristics
	assert.Equal(t, "research_report_workflow", project.Workflow.Name)
	assert.Equal(t, "sequential", project.Workflow.Workflow.Type)
	assert.GreaterOrEqual(t, len(project.Workflow.Workflow.Steps), 3, "Research workflow should have at least 3 steps")
}