package testfixtures

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
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

func ensureGitInputs(inputs map[string]interface{}) map[string]interface{} {
	clone := make(map[string]interface{}, len(inputs)+4)
	for k, v := range inputs {
		clone[k] = v
	}

	if _, ok := clone["basegitrepo"]; !ok {
		clone["basegitrepo"] = "/tmp/vibethis/test-repo"
	}
	if _, ok := clone["basegithash"]; !ok {
		clone["basegithash"] = "0000000000000000000000000000000000000000"
	}
	if _, ok := clone["ticketid"]; !ok {
		clone["ticketid"] = "TEST-TICKET"
	}
	if _, ok := clone["cellname"]; !ok {
		clone["cellname"] = "test-cell"
	}

	return clone
}

// equalWithTypeFlexibility compares two values with flexibility for numeric types.
// It treats int and float64 as equivalent when they represent the same numeric value,
// but only as a fallback after exact comparison fails.
func equalWithTypeFlexibility(expected, actual interface{}) bool {
	// First try exact comparison
	if reflect.DeepEqual(expected, actual) {
		return true
	}

	// If exact comparison failed, try with type flexibility

	// Handle maps recursively
	if expectedMap, ok := expected.(map[string]interface{}); ok {
		actualMap, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}

		// Check if both maps have the same keys
		if len(expectedMap) != len(actualMap) {
			return false
		}

		// Compare each key-value pair with type flexibility
		for key, expectedValue := range expectedMap {
			actualValue, exists := actualMap[key]
			if !exists {
				return false
			}

			if !equalWithTypeFlexibility(expectedValue, actualValue) {
				return false
			}
		}
		return true
	}

	// Handle slices recursively
	if expectedSlice, ok := expected.([]interface{}); ok {
		actualSlice, ok := actual.([]interface{})
		if !ok {
			return false
		}

		if len(expectedSlice) != len(actualSlice) {
			return false
		}

		for i := range expectedSlice {
			if !equalWithTypeFlexibility(expectedSlice[i], actualSlice[i]) {
				return false
			}
		}
		return true
	}

	// Handle numeric comparisons with type flexibility only as fallback
	expectedNum, expectedIsNum := toFloat64(expected)
	actualNum, actualIsNum := toFloat64(actual)

	if expectedIsNum && actualIsNum {
		return expectedNum == actualNum
	}

	// Values are not equal even with type flexibility
	return false
}

// toFloat64 attempts to convert a value to float64 for numeric comparison
func toFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

// assertEqualWithTypeFlexibility wraps the comparison with proper test assertion messaging
func assertEqualWithTypeFlexibility(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if equalWithTypeFlexibility(expected, actual) {
		return true
	}

	// If not equal, use standard assert.Equal to get nice diff output
	// This will fail but provide good diagnostic information
	return assert.Equal(t, expected, actual, msgAndArgs...)
}

func RunTestOnAllRecipes(path string, t *testing.T) {
	// Create standalone executor once for all tests
	logger := zaptest.NewLogger(t)
	a, err := ops.NewActivityRegistry()
	require.NoError(t, err)
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

			var recipeDef recipe.Recipe
			err = yaml.Unmarshal(recipeData, &recipeDef)
			require.NoError(t, err, "Failed to parse recipe file: %s", recipePath)

			// Run table-driven tests
			for _, tc := range testCases.Tests {
				tc := tc // capture range variable
				t.Run(tc.Name, func(t *testing.T) {
					// Execute recipe using standalone executor
					result, err := exec.Execute(
						context.Background(),
						recipeDef,
						ensureGitInputs(tc.Inputs),
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
							assertEqualWithTypeFlexibility(t, tc.Want, result, "Output mismatch")
						}
					}
				})
			}
		})
	}
}
