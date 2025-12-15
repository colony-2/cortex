package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/cortex/internal/shared"
	recipe "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func TestParseInputs(t *testing.T) {
	tests := []struct {
		name        string
		inputFlags  []string
		inputFile   string
		fileContent string
		expected    map[string]interface{}
		expectError bool
	}{
		{
			name: "simple key-value pairs",
			inputFlags: []string{
				"message=hello",
				"count=42",
				"enabled=true",
			},
			expected: map[string]interface{}{
				"message": "hello",
				"count":   float64(42),
				"enabled": true,
			},
		},
		{
			name: "JSON values",
			inputFlags: []string{
				`array=["item1","item2"]`,
				`object={"key":"value"}`,
			},
			expected: map[string]interface{}{
				"array":  []interface{}{"item1", "item2"},
				"object": map[string]interface{}{"key": "value"},
			},
		},
		{
			name: "nested keys",
			inputFlags: []string{
				"data.source=database",
				"data.port=5432",
			},
			expected: map[string]interface{}{
				"data": map[string]interface{}{
					"source": "database",
					"port":   float64(5432),
				},
			},
		},
		{
			name:        "invalid format",
			inputFlags:  []string{"invalid_no_equals"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file if needed
			var inputFile string
			if tt.fileContent != "" {
				tmpFile, err := os.CreateTemp("", "input-*.json")
				require.NoError(t, err)
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tt.fileContent)
				require.NoError(t, err)
				tmpFile.Close()

				inputFile = tmpFile.Name()
			} else {
				inputFile = tt.inputFile
			}

			result, err := parseInputs(tt.inputFlags, inputFile)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseInputsFromFile(t *testing.T) {
	t.Run("JSON file", func(t *testing.T) {
		content := `{
			"message": "hello",
			"count": 42,
			"data": {
				"nested": true
			}
		}`

		tmpFile, err := os.CreateTemp("", "input-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		tmpFile.Close()

		result, err := parseInputs(nil, tmpFile.Name())
		require.NoError(t, err)

		expected := map[string]interface{}{
			"message": "hello",
			"count":   float64(42),
			"data": map[string]interface{}{
				"nested": true,
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("YAML file", func(t *testing.T) {
		content := `
message: hello
count: 42
data:
  nested: true
  items:
    - one
    - two
`

		tmpFile, err := os.CreateTemp("", "input-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		tmpFile.Close()

		result, err := parseInputs(nil, tmpFile.Name())
		require.NoError(t, err)

		assert.Equal(t, "hello", result["message"])
		assert.Equal(t, 42, result["count"])
		assert.NotNil(t, result["data"])
	})

	t.Run("CLI overrides file", func(t *testing.T) {
		content := `{"message": "from_file", "count": 10}`

		tmpFile, err := os.CreateTemp("", "input-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		tmpFile.Close()

		result, err := parseInputs([]string{"message=from_cli"}, tmpFile.Name())
		require.NoError(t, err)

		assert.Equal(t, "from_cli", result["message"])
		assert.Equal(t, float64(10), result["count"])
	})
}

func TestValidateRecipeStructure(t *testing.T) {
	tests := []struct {
		name        string
		recipe      *recipe.Recipe
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid sequence recipe",
			recipe: &recipe.Recipe{RecipeImpl: &recipe.RecipeSequence{
				RecipeMetadata: recipe.RecipeMetadata{Version: "1.0"},
				SequenceData: recipe.SequenceData{Sequence: []recipe.Node{
					{NodeImpl: &recipe.NodeOp{NodeMetadata: recipe.NodeMetadata{ID: "step1"}, OpData: recipe.OpData{Op: "some_activity"}}},
				}},
			}},
			expectError: false,
		},
		{
			name: "valid op recipe",
			recipe: &recipe.Recipe{RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{Version: "1.0"},
				OpData:         recipe.OpData{Op: "some_activity"},
			}},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecipeStructure(tt.recipe)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateInputs(t *testing.T) {
	tests := []struct {
		name        string
		recipe      *recipe.Recipe
		inputs      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "all required inputs provided",
			recipe: &recipe.Recipe{RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{InputSchema: map[string]recipe.InputSchema{
					"required1": {Type: "string", Required: true},
					"required2": {Type: "number", Required: true},
					"optional":  {Type: "string", Required: false},
				}},
				OpData: recipe.OpData{Op: "noop"},
			}},
			inputs: map[string]interface{}{
				"required1": "value",
				"required2": 42,
			},
			expectError: false,
		},
		{
			name: "missing required input",
			recipe: &recipe.Recipe{RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{InputSchema: map[string]recipe.InputSchema{
					"required": {Type: "string", Required: true},
				}},
				OpData: recipe.OpData{Op: "noop"},
			}},
			inputs:      map[string]interface{}{},
			expectError: true,
			errorMsg:    "required input 'required' not provided",
		},
		{
			name: "optional input not required",
			recipe: &recipe.Recipe{RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{InputSchema: map[string]recipe.InputSchema{
					"optional": {Type: "string", Required: false},
				}},
				OpData: recipe.OpData{Op: "noop"},
			}},
			inputs:      map[string]interface{}{},
			expectError: false,
		},
		{
			name: "extra inputs allowed",
			recipe: &recipe.Recipe{RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{InputSchema: map[string]recipe.InputSchema{
					"defined": {Type: "string", Required: true},
				}},
				OpData: recipe.OpData{Op: "noop"},
			}},
			inputs: map[string]interface{}{
				"defined": "value",
				"extra":   "ignored",
			},
			expectError: false,
		},
	}

	_ = zap.NewNop()
	validator := shared.NewRecipeValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateInputs(tt.recipe, tt.inputs)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetNestedValue(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]interface{}
		key      string
		value    interface{}
		expected map[string]interface{}
	}{
		{
			name:    "simple key",
			initial: map[string]interface{}{},
			key:     "simple",
			value:   "value",
			expected: map[string]interface{}{
				"simple": "value",
			},
		},
		{
			name:    "one level nested",
			initial: map[string]interface{}{},
			key:     "level1.level2",
			value:   "value",
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": "value",
				},
			},
		},
		{
			name:    "deep nesting",
			initial: map[string]interface{}{},
			key:     "a.b.c.d",
			value:   42,
			expected: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": map[string]interface{}{
							"d": 42,
						},
					},
				},
			},
		},
		{
			name: "existing structure",
			initial: map[string]interface{}{
				"existing": map[string]interface{}{
					"field": "old",
				},
			},
			key:   "existing.newfield",
			value: "new",
			expected: map[string]interface{}{
				"existing": map[string]interface{}{
					"field":    "old",
					"newfield": "new",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.initial
			if result == nil {
				result = make(map[string]interface{})
			}

			setNestedValue(result, tt.key, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSaveAndLoadState(t *testing.T) {
	tempDir := t.TempDir()

	state := &ExecutionState{
		RunID:          "test-run-123",
		RecipeFile:     "/path/to/recipe.yaml",
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
		Status:         "running",
		CompletedSteps: []string{"step1", "step2"},
		CurrentStep:    "step3",
		Outputs: map[string]interface{}{
			"result": "partial",
		},
	}

	// Save state
	err := saveState(tempDir, state)
	require.NoError(t, err)

	// Load state
	stateFile := filepath.Join(tempDir, "state.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var loaded ExecutionState
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, state.RunID, loaded.RunID)
	assert.Equal(t, state.RecipeFile, loaded.RecipeFile)
	assert.Equal(t, state.Status, loaded.Status)
	assert.Equal(t, state.CompletedSteps, loaded.CompletedSteps)
	assert.Equal(t, state.CurrentStep, loaded.CurrentStep)
	assert.Equal(t, state.Outputs, loaded.Outputs)
}

func TestFormatTextResult(t *testing.T) {
	tests := []struct {
		name     string
		result   ExecutionResult
		contains []string
	}{
		{
			name: "successful execution",
			result: ExecutionResult{
				Success:       true,
				Recipe:        "test.yaml",
				RunID:         "run-123",
				ExecutionTime: "5.2s",
				Outputs: map[string]interface{}{
					"result": "success",
					"count":  42,
				},
			},
			contains: []string{
				"Recipe: test.yaml",
				"Status: SUCCESS",
				"Run ID: run-123",
				"Execution Time: 5.2s",
				"Outputs:",
				"result: success",
				"count: 42",
			},
		},
		{
			name: "failed execution",
			result: ExecutionResult{
				Success:       false,
				Recipe:        "failed.yaml",
				RunID:         "run-456",
				ExecutionTime: "1.5s",
				Error: &ExecutionError{
					Message: "Activity failed",
					StepID:  "step1",
					Details: "Connection timeout",
				},
			},
			contains: []string{
				"Status: FAILED",
				"Error:",
				"Message: Activity failed",
				"Step: step1",
				"Details: Connection timeout",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatTextResult(tt.result)

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestOutputResult(t *testing.T) {
	result := ExecutionResult{
		Success:       true,
		Recipe:        "test.yaml",
		RunID:         "run-123",
		ExecutionTime: "3.5s",
		Outputs: map[string]interface{}{
			"message": "hello",
		},
	}

	t.Run("JSON format", func(t *testing.T) {
		// Capture stdout
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := outputResult(result, "json")
		assert.NoError(t, err)

		w.Close()
		os.Stdout = old

		var buf [1024]byte
		n, _ := r.Read(buf[:])
		output := string(buf[:n])

		// For successful results, only outputs are returned
		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(output), &parsed)
		assert.NoError(t, err)
		assert.Equal(t, result.Outputs["message"], parsed["message"])
	})

	t.Run("YAML format", func(t *testing.T) {
		// Capture stdout
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := outputResult(result, "yaml")
		assert.NoError(t, err)

		w.Close()
		os.Stdout = old

		var buf [1024]byte
		n, _ := r.Read(buf[:])
		output := string(buf[:n])

		// For successful results, only outputs are returned
		var parsed map[string]interface{}
		err = yaml.Unmarshal([]byte(output), &parsed)
		assert.NoError(t, err)
		assert.Equal(t, result.Outputs["message"], parsed["message"])
	})

	t.Run("unsupported format", func(t *testing.T) {
		err := outputResult(result, "xml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported output format")
	})

	t.Run("error result outputs full structure", func(t *testing.T) {
		errorResult := ExecutionResult{
			Success:       false,
			Recipe:        "test.yaml",
			RunID:         "run-456",
			ExecutionTime: "1.2s",
			Error: &ExecutionError{
				Message: "Test error",
				Details: "Something went wrong",
			},
		}

		// Capture stdout
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := outputResult(errorResult, "json")
		assert.NoError(t, err)

		w.Close()
		os.Stdout = old

		var buf [1024]byte
		n, _ := r.Read(buf[:])
		output := string(buf[:n])

		// For error results, full structure is returned
		var parsed ExecutionResult
		err = json.Unmarshal([]byte(output), &parsed)
		assert.NoError(t, err)
		assert.Equal(t, errorResult.Success, parsed.Success)
		assert.Equal(t, errorResult.Error.Message, parsed.Error.Message)
	})
}

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		format      string
		noColor     bool
		expectError bool
	}{
		{
			name:   "debug level text format",
			level:  "debug",
			format: "text",
		},
		{
			name:   "info level json format",
			level:  "info",
			format: "json",
		},
		{
			name:    "no color",
			level:   "warn",
			format:  "text",
			noColor: true,
		},
		{
			name:        "invalid level",
			level:       "invalid",
			format:      "text",
			expectError: true,
		},
		{
			name:   "error level",
			level:  "error",
			format: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := setupLogger(tt.level, tt.format, tt.noColor)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, logger)

				// Test that logger works
				logger.Info("test message", zap.String("field", "value"))
			}
		})
	}
}
