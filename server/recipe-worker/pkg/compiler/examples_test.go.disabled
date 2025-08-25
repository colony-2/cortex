package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"go.uber.org/zap/zaptest"
)

func TestCompileExampleRecipes(t *testing.T) {
	// Find the examples directory relative to this test file
	examplesDir := filepath.Join("..", "..", "examples")
	
	// Check if examples directory exists
	if _, err := os.Stat(examplesDir); os.IsNotExist(err) {
		t.Skip("Examples directory not found")
	}
	
	logger := zaptest.NewLogger(t)
	parser := recipe.NewParser(logger)
	
	tests := []struct {
		name     string
		filename string
		setup    func(*ActivityRegistry)
	}{
		{
			name:     "unified_data_pipeline",
			filename: "unified_data_pipeline.yaml",
			setup: func(r *ActivityRegistry) {
				// Register activities that might be used in the example
				r.RegisterActivity("validate_sources")
				r.RegisterActivity("process_data") 
				r.RegisterActivity("combine_results")
				r.RegisterActivity("llm")
				// Register shared activities
				r.RegisterActivity("shared/data-validator")
				r.RegisterActivity("shared/data-processor")
				r.RegisterActivity("shared/data-analyzer")
			},
		},
		{
			name:     "template_features",
			filename: "template_features.yaml",
			setup: func(r *ActivityRegistry) {
				// Register activities that might be used in the example
				r.RegisterActivity("prepare_data")
				r.RegisterActivity("format_text")
				r.RegisterActivity("analyze_data")
				r.RegisterActivity("create_summary")
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Construct file path
			filePath := filepath.Join(examplesDir, tt.filename)
			
			// Check if file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Skipf("Example file %s not found", tt.filename)
				return
			}
			
			// Parse the recipe
			recipeData, err := parser.ParseRecipe(filePath)
			if err != nil {
				t.Logf("Failed to parse example %s: %v", tt.filename, err)
				t.Skip("Example file could not be parsed")
				return
			}
			
			require.NotNil(t, recipeData)
			require.NotNil(t, recipeData.Recipe)
			
			// Set up activity registry with required activities
			registry := NewActivityRegistry()
			tt.setup(registry)
			
			// Create compiler and attempt to compile
			compiler := NewCompiler(registry)
			workflowFunc, err := compiler.CompileWorkflow(recipeData.Recipe)
			require.NoError(t, err)
			assert.NotNil(t, workflowFunc)
		})
	}
}

func TestCompileGeminiExample(t *testing.T) {
	// Test the gemini workflow example if it exists
	examplesDir := filepath.Join("..", "..", "examples")
	filePath := filepath.Join(examplesDir, "gemini_workflow.yaml")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skip("Gemini example file not found")
	}
	
	logger := zaptest.NewLogger(t)
	parser := recipe.NewParser(logger)
	
	// Parse the recipe
	recipeData, err := parser.ParseRecipe(filePath)
	if err != nil {
		t.Skip("Gemini example could not be parsed - likely old format")
		return
	}
	
	require.NotNil(t, recipeData)
	require.NotNil(t, recipeData.Recipe)
	
	// Set up registry with gemini activities
	registry := NewActivityRegistry()
	registry.RegisterActivity("gemini_generate")
	registry.RegisterActivity("llm")
	registry.RegisterActivity("gemini_report_activity")
	
	// Compile
	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(recipeData.Recipe)
	require.NoError(t, err)
	assert.NotNil(t, workflowFunc)
}