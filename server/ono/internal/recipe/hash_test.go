package recipe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yamlpkg "vibethis/ono/pkg/yaml"
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
				Workflow: &yamlpkg.WorkflowDefinition{
					Name: "test-workflow",
					Workflow: yamlpkg.WorkflowSpec{
						Steps: []yamlpkg.Step{
							{ID: "step1", Activity: "activity1"},
						},
					},
				},
			},
			recipe2: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
				Workflow: &yamlpkg.WorkflowDefinition{
					Name: "test-workflow",
					Workflow: yamlpkg.WorkflowSpec{
						Steps: []yamlpkg.Step{
							{ID: "step1", Activity: "activity1"},
						},
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
			},
			recipe2: &Recipe{
				Name:        "recipe2",
				Version:     "1.0.0",
				Description: "Test recipe",
			},
			shouldBeEqual: false,
		},
		{
			name: "different versions produce different hashes",
			recipe1: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
			},
			recipe2: &Recipe{
				Name:        "test-recipe",
				Version:     "2.0.0",
				Description: "Test recipe",
			},
			shouldBeEqual: false,
		},
		{
			name: "different activity order produces same hash",
			recipe1: &Recipe{
				Name: "test-recipe",
				Version: "1.0.0",
				Activities: []yamlpkg.ActivityDefinition{
					{Name: "activity1", Description: "First activity"},
					{Name: "activity2", Description: "Second activity"},
				},
			},
			recipe2: &Recipe{
				Name: "test-recipe",
				Version: "1.0.0",
				Activities: []yamlpkg.ActivityDefinition{
					{Name: "activity2", Description: "Second activity"},
					{Name: "activity1", Description: "First activity"},
				},
			},
			shouldBeEqual: true, // Activities are sorted by name
		},
		{
			name: "whitespace differences don't affect hash",
			recipe1: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe",
			},
			recipe2: &Recipe{
				Name:        "test-recipe",
				Version:     "1.0.0",
				Description: "Test recipe   ", // Extra spaces
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
		Activities: []yamlpkg.ActivityDefinition{
			{Name: "activity1", Description: "First"},
			{Name: "activity2", Description: "Second"},
			{Name: "activity3", Description: "Third"},
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