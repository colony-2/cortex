package recipe

import (
	"os"
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
			name: "valid recipe with sequence",
			yamlContent: `
name: test-recipe
version: "1.0.0"
description: Test recipe
sequence:
  - id: step1
    op: test_activity
    inputs:
      input1: "value1"
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "test-recipe", recipe.Name)
				assert.Equal(t, "1.0.0", recipe.Version)
				assert.Equal(t, "Test recipe", recipe.Description)
				require.NotNil(t, recipe.Recipe)
				require.NotNil(t, recipe.Recipe.Sequence)
				require.Len(t, recipe.Recipe.Sequence, 1)
				assert.Equal(t, "step1", recipe.Recipe.Sequence[0].ID)
				assert.Equal(t, "test_activity", recipe.Recipe.Sequence[0].Op)
			},
		},
		{
			name: "valid recipe with single operation",
			yamlContent: `
name: simple-recipe
version: "1.0.0"
op: echo_activity
inputs:
  message: "Hello World"
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "simple-recipe", recipe.Name)
				require.NotNil(t, recipe.Recipe)
				assert.Equal(t, "echo_activity", recipe.Recipe.Op)
				assert.Equal(t, "Hello World", recipe.Recipe.Inputs["message"])
			},
		},
		{
			name: "recipe with shared nodes",
			yamlContent: `
name: test-recipe
version: "1.0.0"
shared:
  my_processor:
    op: llm
    inputs:
      model: gpt-4
sequence:
  - id: analyze
    shared: my_processor
    inputs:
      prompt: "Hello"
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "test-recipe", recipe.Name)
				require.NotNil(t, recipe.Recipe)
				require.NotNil(t, recipe.Recipe.Shared)
				assert.Contains(t, recipe.Recipe.Shared, "my_processor")
				assert.Equal(t, "llm", recipe.Recipe.Shared["my_processor"].Op)
				assert.Equal(t, "my_processor", recipe.Recipe.Sequence[0].Shared)
			},
		},
		{
			name: "recipe with parallel execution",
			yamlContent: `
name: parallel-recipe
version: "1.0.0"
parallel:
  - id: task1
    op: command_execution
    inputs:
      run: "echo task1"
  - id: task2
    op: command_execution
    inputs:
      run: "echo task2"
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "parallel-recipe", recipe.Name)
				require.NotNil(t, recipe.Recipe)
				require.NotNil(t, recipe.Recipe.Parallel)
				require.Len(t, recipe.Recipe.Parallel, 2)
				assert.Equal(t, "task1", recipe.Recipe.Parallel[0].ID)
				assert.Equal(t, "task2", recipe.Recipe.Parallel[1].ID)
			},
		},
		{
			name: "recipe with state machine",
			yamlContent: `
name: state-recipe
version: "1.0.0"
states:
  initial: start
  start:
    op: validator
    transitions:
      - to: end
  end:
    op: finalizer
`,
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "state-recipe", recipe.Name)
				require.NotNil(t, recipe.Recipe)
				require.NotNil(t, recipe.Recipe.States)
				assert.Equal(t, "start", recipe.Recipe.States.Initial)
			},
		},
		{
			name: "invalid recipe - missing name",
			yamlContent: `
version: "1.0.0"
op: test_activity
`,
			expectedError: true,
		},
		{
			name: "invalid recipe - no root node",
			yamlContent: `
name: test-recipe
version: "1.0.0"
`,
			expectedError: true,
		},
		{
			name: "invalid recipe - multiple root nodes",
			yamlContent: `
name: test-recipe
version: "1.0.0"
op: test_activity
sequence:
  - id: step1
    op: another_activity
`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpfile, err := os.CreateTemp("", "recipe-*.yaml")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(tt.yamlContent)
			require.NoError(t, err)
			tmpfile.Close()

			// Parse recipe
			recipe, err := parser.ParseRecipe(tmpfile.Name())
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, recipe)
				if tt.validate != nil {
					tt.validate(t, recipe)
				}
			}
		})
	}
}

func TestParseRecipeFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)

	// Test with non-existent file
	_, err := parser.ParseRecipe("/non/existent/path.yaml")
	assert.Error(t, err)
}

func TestValidateRecipe(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)

	tests := []struct {
		name        string
		recipe      *Recipe
		expectError bool
	}{
		{
			name:        "nil recipe",
			recipe:      nil,
			expectError: true,
		},
		{
			name: "recipe without name",
			recipe: &Recipe{
				Version: "1.0.0",
			},
			expectError: true,
		},
		{
			name: "recipe without version",
			recipe: &Recipe{
				Name: "test",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateRecipe(tt.recipe)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestFindRecipeFiles is temporarily disabled as FindRecipeFiles is not implemented
func TestFindRecipeFiles(t *testing.T) {
	t.Skip("FindRecipeFiles method not yet implemented in the new structure")
}