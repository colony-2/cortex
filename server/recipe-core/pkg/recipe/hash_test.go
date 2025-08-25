package recipe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// Helper function for tests
func hashRecipe(recipe *Recipe) (string, error) {
	if recipe == nil || recipe.Recipe == nil {
		return "", assert.AnError
	}
	computer := NewHashComputer()
	return computer.ComputeRecipeHash(recipe), nil
}

func TestHashRecipe(t *testing.T) {
	tests := []struct {
		name          string
		recipe1       *Recipe
		recipe2       *Recipe
		shouldBeEqual bool
	}{
		{
			name: "identical recipes produce same hash",
			recipe1: &Recipe{
				ID:          "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID:   "test-recipe",
						Desc: "Test recipe",
						Op:   "test_activity",
					},
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				ID:          "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID:   "test-recipe",
						Desc: "Test recipe",
						Op:   "test_activity",
					},
					Version: "1.0.0",
				},
			},
			shouldBeEqual: true,
		},
		{
			name: "different IDs produce different hashes",
			recipe1: &Recipe{
				ID:          "recipe1",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "recipe1",
						Op: "test_activity",
					},
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				ID:          "recipe2",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "recipe2",
						Op: "test_activity",
					},
					Version: "1.0.0",
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "different operations produce different hashes",
			recipe1: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Op: "activity1",
					},
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Op: "activity2",
					},
					Version: "1.0.0",
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "recipes with shared nodes",
			recipe1: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Sequence: []yamlpkg.Node{
							{ID: "step1", Shared: "my_processor"},
						},
					},
					Version: "1.0.0",
					Defs: map[string]yamlpkg.Node{
						"my_processor": {
							Op: "llm",
							Inputs: map[string]interface{}{
								"model": "gpt-4",
							},
						},
					},
				},
			},
			recipe2: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Sequence: []yamlpkg.Node{
							{ID: "step1", Shared: "my_processor"},
						},
					},
					Version: "1.0.0",
					Defs: map[string]yamlpkg.Node{
						"my_processor": {
							Op: "llm",
							Inputs: map[string]interface{}{
								"model": "gpt-3.5",
							},
						},
					},
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "order of sequence matters",
			recipe1: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Sequence: []yamlpkg.Node{
							{ID: "step1", Op: "activity1"},
							{ID: "step2", Op: "activity2"},
						},
					},
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Sequence: []yamlpkg.Node{
							{ID: "step2", Op: "activity2"},
							{ID: "step1", Op: "activity1"},
						},
					},
					Version: "1.0.0",
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "parallel vs sequence produces different hashes",
			recipe1: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Sequence: []yamlpkg.Node{
							{ID: "step1", Op: "activity1"},
							{ID: "step2", Op: "activity2"},
						},
					},
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				ID:      "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Node: yamlpkg.Node{
						ID: "test-recipe",
						Parallel: []yamlpkg.Node{
							{ID: "step1", Op: "activity1"},
							{ID: "step2", Op: "activity2"},
						},
					},
					Version: "1.0.0",
				},
			},
			shouldBeEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1, err := hashRecipe(tt.recipe1)
			require.NoError(t, err)
			
			hash2, err := hashRecipe(tt.recipe2)
			require.NoError(t, err)
			
			if tt.shouldBeEqual {
				assert.Equal(t, hash1, hash2, "Expected recipes to have same hash")
			} else {
				assert.NotEqual(t, hash1, hash2, "Expected recipes to have different hashes")
			}
			
			// Hashes should be consistent
			hash1Again, err := hashRecipe(tt.recipe1)
			require.NoError(t, err)
			assert.Equal(t, hash1, hash1Again, "Hash should be consistent for same recipe")
		})
	}
}

func TestHashRecipe_NilRecipe(t *testing.T) {
	hash, err := hashRecipe(nil)
	assert.Error(t, err)
	assert.Empty(t, hash)
	
	recipe := &Recipe{
		ID:      "test",
		Version: "1.0",
		Recipe:  nil,
	}
	hash, err = hashRecipe(recipe)
	assert.Error(t, err)
	assert.Empty(t, hash)
}