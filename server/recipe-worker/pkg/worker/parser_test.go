package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"go.uber.org/zap/zaptest"
)

func TestParserDebug(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a simple test recipe file with unified format
	recipeContent := `
name: test-recipe
version: "1.0.0"
description: Test unified recipe

shared:
  my_llm:
    op: llm
    inputs:
      model: gpt-4
      type: ai_prompt

sequence:
  - id: step1
    op: test-activity
    inputs:
      type: function
      data: "test"
    outputs:
      result: "processed_data"
  - id: step2
    shared: my_llm
    inputs:
      prompt: "Analyze the data"
`

	recipePath := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)

	// Try to parse it using recipe parser
	logger := zaptest.NewLogger(t)
	parser := recipe.NewParser(logger)
	recipeData, err := parser.ParseRecipe(recipePath)
	
	if err != nil {
		t.Logf("Parse error: %v", err)
	} else {
		t.Logf("Parse success!")
		t.Logf("Recipe name: %s", recipeData.Name)
		t.Logf("Recipe version: %s", recipeData.Version)
		// Log which node type is being used
		if recipeData.Recipe.Op != "" {
			t.Logf("Operation: %s", recipeData.Recipe.Op)
		} else if len(recipeData.Recipe.Sequence) > 0 {
			t.Logf("Sequence nodes: %d", len(recipeData.Recipe.Sequence))
		} else if len(recipeData.Recipe.Parallel) > 0 {
			t.Logf("Parallel nodes: %d", len(recipeData.Recipe.Parallel))
		} else if recipeData.Recipe.States != nil {
			t.Logf("State machine with initial state: %s", recipeData.Recipe.States.Initial)
		}
		t.Logf("Shared activities: %d", len(recipeData.Recipe.Shared))
	}
	
	// Also test the registry's loadUnifiedRecipe
	registry := &Registry{
		recipesDir:   tempDir,
		recipes:      make(map[string]*recipe.Recipe),
		hashComputer: recipe.NewHashComputer(),
	}
	
	loadedRecipe, err := registry.loadUnifiedRecipe(recipePath)
	if err != nil {
		t.Logf("Registry load error: %v", err)
	} else if loadedRecipe == nil {
		t.Logf("Registry returned nil recipe (file was skipped)")
	} else {
		t.Logf("Registry loaded recipe: %s", loadedRecipe.Name)
	}
	
	assert.NotNil(t, recipeData)
	assert.NoError(t, err)
}