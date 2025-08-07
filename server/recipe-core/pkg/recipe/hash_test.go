package recipe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestHashComputer_ComputeRecipeHash(t *testing.T) {
	hashComputer := NewHashComputer()

	tests := []struct {
		name          string
		recipe1       *Recipe
		recipe2       *Recipe
		shouldBeEqual bool
	}{
		{
			name: "identical recipes produce same hash",
			recipe1: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
					Steps: []yamlpkg.Step{
						{ID: "step1", Uses: "activity1"},
					},
				},
			},
			recipe2: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
					Steps: []yamlpkg.Step{
						{ID: "step1", Uses: "activity1"},
					},
				},
			},
			shouldBeEqual: true,
		},
		{
			name: "different names produce different hashes",
			recipe1: &Recipe{
				Name:        "recipe1",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "recipe1",
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				Name:        "recipe2",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "recipe2",
					Version: "1.0.0",
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "different versions produce different hashes",
			recipe1: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				Name:        "test-recipe",
				Version:     "2.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "2.0.0",
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "different shared activities produce different hashes",
			recipe1: &Recipe{
				Name: "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
					Shared: map[string]yamlpkg.SharedActivity{
						"activity1": {Uses: "llm", Config: map[string]interface{}{"model": "gpt-4"}},
					},
				},
			},
			recipe2: &Recipe{
				Name: "test-recipe",
				Version: "1.0.0",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
					Shared: map[string]yamlpkg.SharedActivity{
						"activity1": {Uses: "llm", Config: map[string]interface{}{"model": "gpt-3.5"}},
					},
				},
			},
			shouldBeEqual: false,
		},
		{
			name: "whitespace differences don't affect hash",
			recipe1: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
				},
			},
			recipe2: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe   ", // Extra spaces
				Recipe: &yamlpkg.RecipeDefinition{
					Name:    "test-recipe",
					Version: "1.0.0",
				},
			},
			shouldBeEqual: true, // Whitespace is trimmed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashComputer.ComputeRecipeHash(tt.recipe1)
			hash2 := hashComputer.ComputeRecipeHash(tt.recipe2)

			if tt.shouldBeEqual {
				if hash1 != hash2 {
					// Debug output
					norm1 := hashComputer.normalizeRecipe(tt.recipe1)
					norm2 := hashComputer.normalizeRecipe(tt.recipe2)
					t.Logf("Recipe1 normalized: %+v", norm1)
					t.Logf("Recipe2 normalized: %+v", norm2)
				}
				assert.Equal(t, hash1, hash2, "Hashes should be equal")
			} else {
				assert.NotEqual(t, hash1, hash2, "Hashes should be different")
			}

			// Verify hash format (should be hex string)
			assert.Regexp(t, "^[a-f0-9]{64}$", hash1, "Hash should be 64-char hex string")
		})
	}
}

func TestHashComputer_Deterministic(t *testing.T) {
	hashComputer := NewHashComputer()

	recipe := &Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Recipe: &yamlpkg.RecipeDefinition{
			Name:    "test-recipe",
			Version: "1.0.0",
			Steps: []yamlpkg.Step{
				{ID: "step1", Uses: "activity1"},
				{ID: "step2", Uses: "activity2"},
				{ID: "step3", Uses: "activity3"},
			},
		},
	}

	// Compute hash multiple times
	hashes := make([]string, 10)
	for i := 0; i < 10; i++ {
		hashes[i] = hashComputer.ComputeRecipeHash(recipe)
	}

	// All hashes should be identical
	for i := 1; i < 10; i++ {
		assert.Equal(t, hashes[0], hashes[i], "Hash should be deterministic")
	}
}