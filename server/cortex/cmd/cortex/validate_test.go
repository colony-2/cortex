package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestValidateFile(t *testing.T) {
	// Create a test schema
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft7
	
	schemaDoc := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
			"version": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []string{"name"},
	}
	
	schemaBytes, _ := json.Marshal(schemaDoc)
	err := compiler.AddResource("test.json", bytes.NewReader(schemaBytes))
	require.NoError(t, err)
	schema := compiler.MustCompile("test.json")
	
	// Create test files
	tempDir := t.TempDir()
	
	// Valid file
	validFile := filepath.Join(tempDir, "valid.yaml")
	validContent := `name: test-recipe
version: "1.0"
sequence:
  - id: step1
    op: command_execution
    inputs:
      run: "echo test"`
	err = os.WriteFile(validFile, []byte(validContent), 0644)
	require.NoError(t, err)
	
	// Invalid file (missing required field)
	invalidFile := filepath.Join(tempDir, "invalid.yaml")
	invalidContent := `version: "1.0"`
	err = os.WriteFile(invalidFile, []byte(invalidContent), 0644)
	require.NoError(t, err)
	
	// Create a validator for testing
	logger := zap.NewNop()
	rm, _ := shared.NewRegistryManager(logger)
	validator := shared.NewRecipeValidator(rm)
	
	// Test valid file
	result := validateFile(validFile, schema, false, validator)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	
	// Test invalid file
	result = validateFile(invalidFile, schema, false, validator)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestConvertYAMLToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "map conversion",
			input: map[interface{}]interface{}{
				"key": "value",
				123:   "numeric key",
			},
			expected: map[string]interface{}{
				"key": "value",
				"123": "numeric key",
			},
		},
		{
			name: "nested map",
			input: map[interface{}]interface{}{
				"outer": map[interface{}]interface{}{
					"inner": "value",
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name: "array",
			input: []interface{}{
				"item1",
				map[interface{}]interface{}{"key": "value"},
			},
			expected: []interface{}{
				"item1",
				map[string]interface{}{"key": "value"},
			},
		},
		{
			name:     "simple value",
			input:    "string",
			expected: "string",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertYAMLToJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		message  string
		expected string
	}{
		{"value is not valid enum", "enum"},
		{"expected type string", "type"},
		{"missing required property", "required"},
		{"value below minimum", "minimum"},
		{"value exceeds maximum", "maximum"},
		{"does not match pattern", "pattern"},
		{"invalid format", "format"},
		{"array has less than minItems", "minItems"},
		{"array has more than maxItems", "maxItems"},
		{"array items are not unique", "uniqueItems"},
		{"additional properties not allowed", "additionalProperties"},
		{"some other error", "validation"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			err := &jsonschema.ValidationError{
				Message: tt.message,
			}
			result := getErrorType(err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateTemplateVariables(t *testing.T) {
	doc := map[string]interface{}{
		"inputs": []interface{}{
			map[string]interface{}{
				"name": "user_input",
				"type": "string",
			},
		},
		"steps": []interface{}{
			map[string]interface{}{
				"id":   "step1",
				"name": "First Step",
			},
		},
	}
	
	tests := []struct {
		name        string
		content     string
		expectError bool
		errorType   string
	}{
		{
			name:        "valid input reference",
			content:     "value: {{.Inputs.user_input}}",
			expectError: false,
		},
		{
			name:        "invalid input reference",
			content:     "value: {{.Inputs.nonexistent}}",
			expectError: true,
			errorType:   "undefined_input",
		},
		{
			name:        "valid step reference",
			content:     "value: {{.Steps.step1.outputs.result}}",
			expectError: false,
		},
		{
			name:        "invalid step reference",
			content:     "value: {{.Steps.step99.outputs.result}}",
			expectError: true,
			errorType:   "undefined_step",
		},
		{
			name:        "valid context reference",
			content:     "value: {{.Context.workflowID}}",
			expectError: false,
		},
		{
			name:        "invalid context reference",
			content:     "value: {{.Context.invalidField}}",
			expectError: true,
			errorType:   "invalid_context_field",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateTemplateVariables(tt.content, doc)
			if tt.expectError {
				assert.NotEmpty(t, errors)
				if tt.errorType != "" && len(errors) > 0 {
					assert.Equal(t, tt.errorType, errors[0].ErrorType)
				}
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func TestExtractInputs(t *testing.T) {
	doc := map[string]interface{}{
		"inputs": []interface{}{
			map[string]interface{}{
				"name": "input1",
				"type": "string",
			},
			map[string]interface{}{
				"name": "input2",
				"type": "number",
			},
		},
	}
	
	inputs := extractInputs(doc)
	assert.Equal(t, []string{"input1", "input2"}, inputs)
	
	// Test with no inputs
	docNoInputs := map[string]interface{}{}
	inputs = extractInputs(docNoInputs)
	assert.Empty(t, inputs)
}

func TestExtractSteps(t *testing.T) {
	doc := map[string]interface{}{
		"steps": []interface{}{
			map[string]interface{}{
				"id":   "step1",
				"name": "Step 1",
			},
			map[string]interface{}{
				"id":   "step2",
				"name": "Step 2",
				"steps": []interface{}{
					map[string]interface{}{
						"id": "nested1",
					},
				},
			},
			map[string]interface{}{
				"id": "step3",
				"branches": []interface{}{
					map[string]interface{}{
						"condition": "true",
						"steps": []interface{}{
							map[string]interface{}{
								"id": "branch1",
							},
						},
					},
				},
			},
		},
	}
	
	steps := extractSteps(doc)
	assert.Contains(t, steps, "step1")
	assert.Contains(t, steps, "step2")
	assert.Contains(t, steps, "step3")
	assert.Contains(t, steps, "nested1")
	assert.Contains(t, steps, "branch1")
}

func TestContains(t *testing.T) {
	list := []string{"a", "b", "c"}
	assert.True(t, contains(list, "a"))
	assert.True(t, contains(list, "b"))
	assert.True(t, contains(list, "c"))
	assert.False(t, contains(list, "d"))
	assert.False(t, contains([]string{}, "a"))
}

func TestFormatTextOutput(t *testing.T) {
	summary := ValidationSummary{
		FilesValidated: 2,
		Valid:          1,
		Invalid:        1,
		TotalErrors:    2,
		TotalWarnings:  1,
		Results: []ValidationResult{
			{
				File:  "valid.yaml",
				Valid: true,
			},
			{
				File:  "invalid.yaml",
				Valid: false,
				Errors: []ValidationError{
					{
						ErrorType:   "required",
						Field:       "name",
						Description: "missing required field",
					},
					{
						ErrorType:   "type",
						Field:       "version",
						Description: "expected string, got number",
					},
				},
				Warnings: []ValidationError{
					{
						ErrorType:   "deprecated",
						Field:       "old_field",
						Description: "field is deprecated",
					},
				},
			},
		},
	}
	
	output := formatTextOutput(summary)
	
	// Check for key elements in the output
	assert.Contains(t, output, "VALIDATION RESULTS")
	assert.Contains(t, output, "valid.yaml")
	assert.Contains(t, output, "invalid.yaml")
	assert.Contains(t, output, "✅ VALID")
	assert.Contains(t, output, "❌ INVALID")
	assert.Contains(t, output, "SUMMARY")
	assert.Contains(t, output, "Files validated: 2")
}

func TestFormatMarkdownOutput(t *testing.T) {
	summary := ValidationSummary{
		FilesValidated: 1,
		Valid:          0,
		Invalid:        1,
		TotalErrors:    1,
		Results: []ValidationResult{
			{
				File:  "test.yaml",
				Valid: false,
				Errors: []ValidationError{
					{
						ErrorType:   "required",
						Field:       "name",
						Description: "missing required field",
					},
				},
			},
		},
	}
	
	output := formatMarkdownOutput(summary)
	
	// Check for markdown elements
	assert.Contains(t, output, "# Recipe Validation Results")
	assert.Contains(t, output, "## Summary")
	assert.Contains(t, output, "## Detailed Errors")
	assert.Contains(t, output, "## Statistics")
	assert.Contains(t, output, "`test.yaml`")
	assert.Contains(t, output, "❌ INVALID")
	assert.Contains(t, output, "|")  // Table separator
}

func TestOutputResults(t *testing.T) {
	summary := ValidationSummary{
		FilesValidated: 1,
		Valid:          1,
		Invalid:        0,
		Results: []ValidationResult{
			{
				File:  "test.yaml",
				Valid: true,
			},
		},
	}
	
	// Test JSON output
	tempFile := filepath.Join(t.TempDir(), "output.json")
	err := outputResults(summary, "json", tempFile)
	require.NoError(t, err)
	
	data, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	
	var parsed ValidationSummary
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, summary.FilesValidated, parsed.FilesValidated)
	
	// Test text output to stdout (captured in test)
	err = outputResults(summary, "text", "")
	require.NoError(t, err)
	
	// Test markdown output
	tempFile = filepath.Join(t.TempDir(), "output.md")
	err = outputResults(summary, "markdown", tempFile)
	require.NoError(t, err)
	
	data, err = os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "# Recipe Validation Results"))
	
	// Test invalid format
	err = outputResults(summary, "invalid", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported output format")
}