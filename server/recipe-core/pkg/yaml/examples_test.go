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
		validate func(t *testing.T, recipe *RecipeDefinition)
	}{
		{
			name:     "simple_workflow",
			filename: "simple_workflow.yaml",
			validate: func(t *testing.T, recipe *RecipeDefinition) {
				assert.Equal(t, "simple_research", recipe.Name)
				assert.Equal(t, "A simple research workflow using unified activity model", recipe.Description)
				assert.Equal(t, "1.0", recipe.Version)
				
				// Check inputs
				require.Len(t, recipe.Inputs, 1)
				assert.Equal(t, "query", recipe.Inputs[0].Name)
				assert.Equal(t, "string", recipe.Inputs[0].Type)
				assert.True(t, recipe.Inputs[0].Required)
				
				// Check workflow has two sequential steps
				require.Len(t, recipe.Steps, 2)
				assert.Equal(t, "search", recipe.Steps[0].ID)
				assert.Equal(t, "summarize", recipe.Steps[1].ID)
				
				// Check steps use unified activity pattern
				assert.Equal(t, "quick_search_activity", recipe.Steps[0].Uses)
				assert.Equal(t, "summarize_activity", recipe.Steps[1].Uses)
			},
		},
		{
			name:     "unified_data_pipeline",
			filename: "unified_data_pipeline.yaml",
			validate: func(t *testing.T, recipe *RecipeDefinition) {
				assert.Equal(t, "data-pipeline", recipe.Name)
				assert.Equal(t, "1.0", recipe.Version)
				
				// Check has shared section
				require.NotNil(t, recipe.Shared)
				assert.Contains(t, recipe.Shared, "data_analyst")
				assert.Contains(t, recipe.Shared, "report_writer")
				assert.Contains(t, recipe.Shared, "quality_review_machine")
				
				// Check data analyst persona
				dataAnalyst := recipe.Shared["data_analyst"]
				assert.Equal(t, "llm", dataAnalyst.Uses)
				assert.Equal(t, "gpt-4", dataAnalyst.Config["model"])
				
				// Check multiple steps with various activity types
				require.GreaterOrEqual(t, len(recipe.Steps), 5)
				
				// Find specific steps by ID
				var validateStep, processStep, analyzeStep *Step
				for _, step := range recipe.Steps {
					switch step.ID {
					case "validate":
						validateStep = &step
					case "process_datasets":
						processStep = &step
					case "analyze_results":
						analyzeStep = &step
					}
				}
				
				require.NotNil(t, validateStep)
				assert.Equal(t, "shared/dataset_validator", validateStep.Uses)
				
				require.NotNil(t, processStep)
				require.NotNil(t, processStep.Parallel)
				assert.Equal(t, "{{ .Steps.validate.outputs.valid_ids }}", processStep.Parallel.ForEach)
				
				require.NotNil(t, analyzeStep)
				assert.Equal(t, "shared/data_analyst", analyzeStep.Uses)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath := filepath.Join(examplesDir, tt.filename)
			
			// Parse the file
			recipe, err := parser.ParseRecipe(filepath)
			require.NoError(t, err, "Failed to parse %s", tt.filename)
			require.NotNil(t, recipe)
			
			// Run specific validations
			tt.validate(t, recipe)
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
				recipe, err := parser.ParseRecipe(filepath)
				require.NoError(t, err, "Failed to parse %s", file.Name())
				require.NotNil(t, recipe)
				
				// Basic validation - every example should have a recipe
				assert.NotEmpty(t, recipe.Name, "Recipe in %s should have a name", file.Name())
				assert.NotEmpty(t, recipe.Version, "Recipe in %s should have a version", file.Name())
				assert.NotEmpty(t, recipe.Steps, "Recipe in %s should have steps", file.Name())
				
				// Validate that all steps have 'uses' field
				for _, step := range recipe.Steps {
					assert.NotEmpty(t, step.Uses, "Step %s in %s should have 'uses' field", step.ID, file.Name())
				}
			})
		}
	}
}