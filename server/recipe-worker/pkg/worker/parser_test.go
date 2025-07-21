package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

func TestParserDebug(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a simple test recipe file with proper structure
	recipeContent := `
workflow:
  name: test-workflow
  version: "1.0.0"
  description: Test workflow
  
  workflow:
    type: sequential
    steps:
      - id: step1
        activity: test-activity
        inputs:
          data: "test"
    outputs:
      result: "{{ .Steps.step1.outputs.data }}"
      
activities:
  - name: test-activity
    timeout: 30s
`

	recipePath := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)

	// Try to parse it
	parser := yamlpkg.NewParser()
	project, err := parser.ParseProject(recipePath)
	
	if err != nil {
		t.Logf("Parse error: %v", err)
	} else {
		t.Logf("Parse success!")
		if project.Workflow != nil {
			t.Logf("Workflow name: %s", project.Workflow.Name)
			t.Logf("Workflow version: %s", project.Workflow.Version)
		}
		t.Logf("Activities: %d", len(project.Activities))
	}
	
	// Also test the registry's loadSingleFileRecipe
	registry := &Registry{
		recipesDir: tempDir,
		recipes: make(map[string]*recipe.Recipe),
		hashComputer: recipe.NewHashComputer(),
	}
	
	loadedRecipe, err := registry.loadSingleFileRecipe(recipePath)
	if err != nil {
		t.Logf("Registry load error: %v", err)
	} else if loadedRecipe == nil {
		t.Logf("Registry returned nil recipe (file was skipped)")
	} else {
		t.Logf("Registry loaded recipe: %s", loadedRecipe.Name)
	}
	
	assert.NotNil(t, project)
	assert.NoError(t, err)
}