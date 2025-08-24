package testfixtures

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/sleepop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

type TestCase struct {
	Name            string                 `yaml:"name"`
	Description     string                 `yaml:"description,omitempty"`
	Inputs          map[string]interface{} `yaml:"inputs"`
	Want            map[string]interface{} `yaml:"want,omitempty"`
	WantErr         bool                   `yaml:"wantErr"`
	WantErrContains string                 `yaml:"wantErrContains,omitempty"`
}

type TestCases struct {
	Tests []TestCase `yaml:"tests"`
}

func RunTestOnAllRecipes(path string, t *testing.T) {
	// Create standalone executor once for all tests
	logger := zaptest.NewLogger(t)
	a := ops.NewActivityRegistry()
	require.NoError(t, a.RegisterAll(sleepop.GetOp(), commandop.GetOp()))
	exec, err := executor.NewStandaloneExecutor(a, logger)
	require.NoError(t, err, "Failed to create standalone executor")

	// Find all .test.yaml files
	testFiles, err := filepath.Glob(path)
	require.NoError(t, err, "Failed to find test files")

	if len(testFiles) == 0 {
		t.Skip("No test files found in recipes/")
	}

	for _, testFile := range testFiles {
		// Extract recipe name from test file
		recipeName := strings.TrimSuffix(filepath.Base(testFile), ".test.yaml")
		recipePath := filepath.Join("recipes", recipeName+".yaml")

		t.Run(recipeName, func(t *testing.T) {
			// Load test cases
			testData, err := os.ReadFile(testFile)
			require.NoError(t, err, "Failed to read test file: %s", testFile)

			var testCases TestCases
			err = yaml.Unmarshal(testData, &testCases)
			require.NoError(t, err, "Failed to parse test file: %s", testFile)

			// Load recipe
			recipeData, err := os.ReadFile(recipePath)
			if err != nil {
				// Skip if recipe file doesn't exist
				t.Skipf("Recipe file not found: %s", recipePath)
				return
			}

			var recipeDef yamlpkg.RecipeDefinition
			err = yaml.Unmarshal(recipeData, &recipeDef)
			require.NoError(t, err, "Failed to parse recipe file: %s", recipePath)

			// Run table-driven tests
			for _, tc := range testCases.Tests {
				tc := tc // capture range variable
				t.Run(tc.Name, func(t *testing.T) {
					// Execute recipe using standalone executor
					result, err := exec.Execute(
						context.Background(),
						&recipeDef,
						tc.Inputs,
						executor.ExecutionOptions{
							SuppressLogs: true, // Keep tests clean
						},
					)

					// Check results
					if tc.WantErr {
						require.Error(t, err, "Expected error but got none")
						if tc.WantErrContains != "" {
							assert.Contains(t, err.Error(), tc.WantErrContains,
								"Error message doesn't contain expected text")
						}
					} else {
						require.NoError(t, err, "Unexpected error executing recipe")

						if tc.Want != nil {
							assert.Equal(t, tc.Want, result, "Output mismatch")
						}
					}
				})
			}
		})
	}
}
