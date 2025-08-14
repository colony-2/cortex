package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCompleteSchema(t *testing.T) {
	// Create a test registry
	registry := worker.NewActivityRegistry()
	
	// Generate schema
	schema, err := generateCompleteSchemaLegacy(registry, "", false, false)
	require.NoError(t, err)
	
	// Verify basic structure
	assert.Equal(t, "http://json-schema.org/draft-07/schema#", schema["$schema"])
	assert.Equal(t, "Recipe Schema", schema["title"])
	assert.Equal(t, "object", schema["type"])
	
	// Check properties exist
	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, props, "version")
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "description")
	assert.Contains(t, props, "steps")
	assert.Contains(t, props, "inputs")
	
	// Check definitions exist
	defs, ok := schema["definitions"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, defs, "activities")
	assert.Contains(t, defs, "configs")
	assert.Contains(t, defs, "inputs")
	assert.Contains(t, defs, "outputs")
	assert.Contains(t, defs, "compositions")
}

func TestSchemaCommandJSON(t *testing.T) {
	// Test JSON output format
	registry := worker.NewActivityRegistry()
	schema, err := generateCompleteSchemaLegacy(registry, "", false, false)
	require.NoError(t, err)
	
	// Ensure it can be marshaled to JSON
	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)
	assert.True(t, len(jsonBytes) > 0)
	
	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)
}

func TestSchemaFilterActivity(t *testing.T) {
	// Test filtering to specific activity
	registry := worker.NewActivityRegistry()
	
	// Note: This test assumes no activities are registered in the test environment
	// In a real test, you would register test activities first
	schema, err := generateCompleteSchemaLegacy(registry, "llm", false, false)
	require.NoError(t, err)
	assert.NotNil(t, schema)
}

func TestSchemaWithVersion(t *testing.T) {
	// Test schema with version included
	registry := worker.NewActivityRegistry()
	schema, err := generateCompleteSchemaLegacy(registry, "", true, false)
	require.NoError(t, err)
	
	// Check version field exists
	assert.Contains(t, schema, "version")
	assert.Equal(t, "1.0.0", schema["version"])
}

func TestSchemaWithExamples(t *testing.T) {
	// Test schema with examples
	registry := worker.NewActivityRegistry()
	schema, err := generateCompleteSchemaLegacy(registry, "", false, true)
	require.NoError(t, err)
	
	// The schema should still be valid
	assert.NotNil(t, schema)
	
	// Note: Specific example testing would require registered activities
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with_dash"},
		{"with.dot", "with_dot"},
		{"with spaces", "with_spaces"},
		{"123numeric", "123numeric"},
		{"special!@#chars", "special___chars"},
		{"CamelCase", "CamelCase"},
		{"snake_case", "snake_case"},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateExamples(t *testing.T) {
	// Test example generation for known activity types
	tests := []string{"llm", "command_execution", "git_shallow"}
	
	for _, activityType := range tests {
		t.Run(activityType, func(t *testing.T) {
			examples := generateExamples(activityType)
			assert.NotEmpty(t, examples)
			
			// Verify the example has the correct type
			if len(examples) > 0 {
				example, ok := examples[0].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, activityType, example["type"])
				assert.Contains(t, example, "config")
			}
		})
	}
}

func TestConvertSchema(t *testing.T) {
	// Test schema conversion
	// Note: This would need a real jsonschema.Schema object
	// For now, we just test the function exists and handles nil gracefully
	result := convertSchemaLegacy(nil)
	assert.NotNil(t, result)
	assert.IsType(t, map[string]interface{}{}, result)
}

func TestConvertToOpenAPI(t *testing.T) {
	// Test OpenAPI conversion
	inputSchema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"title":   "Test Schema",
		"type":    "object",
	}
	
	openAPI := convertToOpenAPI(inputSchema)
	
	// Verify OpenAPI structure
	assert.Equal(t, "3.0.0", openAPI["openapi"])
	
	info, ok := openAPI["info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Recipe Schema API", info["title"])
	assert.Equal(t, "1.0.0", info["version"])
	
	components, ok := openAPI["components"].(map[string]interface{})
	require.True(t, ok)
	
	schemas, ok := components["schemas"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, schemas, "Recipe")
}

func TestSchemaCommandIntegration(t *testing.T) {
	t.Skip("Integration test - requires full setup")
	
	// This would be an integration test that actually runs the command
	// It's skipped by default but shows the structure
	
	cmd := schemaCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	err := cmd.Execute()
	require.NoError(t, err)
	
	output := buf.String()
	assert.True(t, strings.Contains(output, "$schema"))
}