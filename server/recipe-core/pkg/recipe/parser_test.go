package recipe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewParser(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)
	
	assert.NotNil(t, parser)
	assert.NotNil(t, parser.logger)
	assert.NotNil(t, parser.activityTypeRegistry)
}

func TestParseRecipe_UnifiedFormat(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)

	tests := []struct {
		name          string
		yamlContent   string
		expectedError bool
		validate      func(t *testing.T, recipe *Recipe)
	}{
		{
			name: "valid unified recipe",
			yamlContent: `
name: test-recipe
version: "1.0.0"
description: Test recipe
steps:
  - id: step1
    uses: test_activity
    inputs:
      input1: "value1"
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "test-recipe", recipe.Name)
				assert.Equal(t, "1.0.0", recipe.Version)
				assert.Equal(t, "Test recipe", recipe.Description)
				require.NotNil(t, recipe.Recipe)
				require.Len(t, recipe.Recipe.Steps, 1)
				assert.Equal(t, "step1", recipe.Recipe.Steps[0].ID)
				assert.Equal(t, "test_activity", recipe.Recipe.Steps[0].Uses)
			},
		},
		{
			name: "recipe with shared activities",
			yamlContent: `
name: test-recipe
version: "1.0.0"
shared:
  my_llm:
    uses: llm
    config:
      model: gpt-4
steps:
  - id: analyze
    uses: shared/my_llm
    inputs:
      prompt: "Hello"
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "test-recipe", recipe.Name)
				require.NotNil(t, recipe.Recipe)
				require.NotNil(t, recipe.Recipe.Shared)
				assert.Contains(t, recipe.Recipe.Shared, "my_llm")
				assert.Equal(t, "llm", recipe.Recipe.Shared["my_llm"].Uses)
				assert.Equal(t, "shared/my_llm", recipe.Recipe.Steps[0].Uses)
			},
		},
		{
			name: "invalid recipe - missing name",
			yamlContent: `
version: "1.0.0"
steps:
  - id: step1
    uses: activity1
`,
			expectedError: true,
		},
		{
			name: "invalid recipe - missing steps",
			yamlContent: `
name: test-recipe
version: "1.0.0"
steps: []
`,
			expectedError: true,
		},
		{
			name: "invalid recipe - step missing uses",
			yamlContent: `
name: test-recipe
version: "1.0.0"
steps:
  - id: step1
    inputs:
      input1: "value"
`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir, err := os.MkdirTemp("", "recipe-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			filePath := filepath.Join(tmpDir, "recipe.yaml")
			require.NoError(t, os.WriteFile(filePath, []byte(tt.yamlContent), 0644))

			// Parse recipe
			recipe, err := parser.ParseRecipe(filePath)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, recipe)

			// Basic validations
			assert.NotEmpty(t, recipe.BasePath)
			assert.Equal(t, filePath, recipe.ManifestPath)
			assert.NotEmpty(t, recipe.Hash)
			assert.False(t, recipe.LastModified.IsZero())

			// Custom validations
			if tt.validate != nil {
				tt.validate(t, recipe)
			}
		})
	}
}

func TestParseRecipe_MultiFileDeprecated(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)

	// Create temporary directory with recipe.yaml (old format)
	tmpDir, err := os.MkdirTemp("", "recipe-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	recipeYAML := `
recipe:
  name: old-format-recipe
  version: "1.0.0"
  files:
    workflow: workflow.yaml
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "recipe.yaml"), []byte(recipeYAML), 0644))

	// Try to parse directory (should fail with deprecation message)
	_, err = parser.ParseRecipe(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multi-file recipe format is deprecated")
}

func TestValidateRecipe(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)

	tests := []struct {
		name          string
		recipe        *Recipe
		expectedError bool
		errorContains string
	}{
		{
			name:          "nil recipe",
			recipe:        nil,
			expectedError: true,
			errorContains: "recipe cannot be nil",
		},
		{
			name: "missing name",
			recipe: &Recipe{
				Version: "1.0.0",
			},
			expectedError: true,
			errorContains: "recipe name is required",
		},
		{
			name: "missing version",
			recipe: &Recipe{
				Name: "test-recipe",
			},
			expectedError: true,
			errorContains: "recipe version is required",
		},
		{
			name: "nil recipe definition",
			recipe: &Recipe{
				Name:    "test-recipe",
				Version: "1.0.0",
				Recipe:  nil,
			},
			expectedError: true,
			errorContains: "recipe definition is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateRecipe(tt.recipe)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}