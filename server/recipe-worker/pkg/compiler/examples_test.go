package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"gopkg.in/yaml.v3"
)

func TestParseExampleRecipes(t *testing.T) {
	// Find the examples directory relative to this test file
	examplesDir := filepath.Join("..", "..", "examples")
	
	// Check if examples directory exists
	if _, err := os.Stat(examplesDir); os.IsNotExist(err) {
		t.Skip("Examples directory not found")
	}
	
	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "unified_data_pipeline",
			filename: "unified_data_pipeline.yaml",
		},
		{
			name:     "template_features",
			filename: "template_features.yaml",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipePath := filepath.Join(examplesDir, tt.filename)
			
			// Check if recipe file exists
			if _, err := os.Stat(recipePath); os.IsNotExist(err) {
				t.Skipf("Recipe file not found: %s", recipePath)
			}
			
			// Read recipe file
			recipeData, err := os.ReadFile(recipePath)
			require.NoError(t, err, "Failed to read recipe file")
			
			// Parse recipe YAML
			var recipeDef yamlpkg.RecipeDefinition
			err = yaml.Unmarshal(recipeData, &recipeDef)
			require.NoError(t, err, "Failed to parse recipe YAML")
			
			// Basic validation
			assert.NotEmpty(t, recipeDef.Version, "Recipe should have a version")
			
			// Check that recipe has at least one node type defined
			hasNode := recipeDef.Op != "" || 
				len(recipeDef.Sequence) > 0 || 
				len(recipeDef.Parallel) > 0 || 
				recipeDef.States != nil
			assert.True(t, hasNode, "Recipe should define at least one node type")
		})
	}
}

func TestRecipeStructureValidation(t *testing.T) {
	tests := []struct {
		name      string
		recipe    yamlpkg.RecipeDefinition
		wantError bool
	}{
		{
			name: "valid_operation_recipe",
			recipe: yamlpkg.RecipeDefinition{
				Node: yamlpkg.Node{
					Op: "test_activity",
					Inputs: map[string]interface{}{
						"param": "value",
					},
				},
				Version: "1.0",
			},
			wantError: false,
		},
		{
			name: "valid_sequence_recipe",
			recipe: yamlpkg.RecipeDefinition{
				Node: yamlpkg.Node{
					Sequence: []yamlpkg.Node{
						{
							ID: "step1",
							Op: "activity1",
						},
						{
							ID: "step2",
							Op: "activity2",
						},
					},
				},
				Version: "1.0",
			},
			wantError: false,
		},
		{
			name: "valid_parallel_recipe",
			recipe: yamlpkg.RecipeDefinition{
				Node: yamlpkg.Node{
					Parallel: []yamlpkg.Node{
						{
							ID: "task1",
							Op: "activity1",
						},
						{
							ID: "task2",
							Op: "activity2",
						},
					},
				},
				Version: "1.0",
			},
			wantError: false,
		},
		{
			name: "recipe_with_shared_definitions",
			recipe: yamlpkg.RecipeDefinition{
				Node: yamlpkg.Node{
					Sequence: []yamlpkg.Node{
						{
							ID:     "step1",
							Shared: "shared_op",
						},
					},
				},
				Version: "1.0",
				Defs: map[string]yamlpkg.Node{
					"shared_op": {
						Op: "shared_activity",
						Inputs: map[string]interface{}{
							"param": "value",
						},
					},
				},
			},
			wantError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic structural validation
			assert.NotEmpty(t, tt.recipe.Version, "Recipe should have a version")
			
			// Check node structure
			node := &tt.recipe.Node
			nodeCount := 0
			if node.Op != "" {
				nodeCount++
			}
			if len(node.Sequence) > 0 {
				nodeCount++
			}
			if len(node.Parallel) > 0 {
				nodeCount++
			}
			if node.States != nil {
				nodeCount++
			}
			
			// A node should define exactly one type
			if !tt.wantError {
				assert.Equal(t, 1, nodeCount, "Node should define exactly one of: op, sequence, parallel, or states")
			}
		})
	}
}