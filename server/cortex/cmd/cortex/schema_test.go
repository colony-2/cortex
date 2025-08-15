package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGenerateCompleteSchema(t *testing.T) {
	// Create logger and registry manager
	logger := zap.NewNop()
	rm, err := shared.NewRegistryManager(logger)
	require.NoError(t, err)
	
	// Generate schema using shared schema manager
	sm := shared.NewSchemaManager(rm)
	schema, err := sm.GenerateCompleteSchema("", false, false)
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
	// New format properties
	assert.Contains(t, props, "sequence")
	assert.Contains(t, props, "parallel")
	assert.Contains(t, props, "states")
	assert.Contains(t, props, "op")
	
	// Check definitions exist
	defs, ok := schema["definitions"].(map[string]interface{})
	require.True(t, ok)
	// New format definitions
	assert.Contains(t, defs, "Node")
	assert.Contains(t, defs, "OpValue")
	assert.Contains(t, defs, "RetryPolicy")
	assert.Contains(t, defs, "StateMap")
	assert.Contains(t, defs, "Activities")
}

func TestSchemaCommandJSON(t *testing.T) {
	// Test JSON output format
	logger := zap.NewNop()
	rm, err := shared.NewRegistryManager(logger)
	require.NoError(t, err)
	
	sm := shared.NewSchemaManager(rm)
	schema, err := sm.GenerateCompleteSchema("", false, false)
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
	logger := zap.NewNop()
	rm, err := shared.NewRegistryManager(logger)
	require.NoError(t, err)
	
	sm := shared.NewSchemaManager(rm)
	schema, err := sm.GenerateCompleteSchema("command_execution", false, false)
	require.NoError(t, err)
	assert.NotNil(t, schema)
}

func TestSchemaWithVersion(t *testing.T) {
	// Test schema with version included
	logger := zap.NewNop()
	rm, err := shared.NewRegistryManager(logger)
	require.NoError(t, err)
	
	sm := shared.NewSchemaManager(rm)
	schema, err := sm.GenerateCompleteSchema("", true, false)
	require.NoError(t, err)
	
	// Check version field exists
	assert.Contains(t, schema, "version")
	assert.Equal(t, "1.0.0", schema["version"])
}

func TestSchemaWithExamples(t *testing.T) {
	// Test schema with examples
	logger := zap.NewNop()
	rm, err := shared.NewRegistryManager(logger)
	require.NoError(t, err)
	
	sm := shared.NewSchemaManager(rm)
	schema, err := sm.GenerateCompleteSchema("", false, true)
	require.NoError(t, err)
	
	// The schema should still be valid
	assert.NotNil(t, schema)
	
	// Check that examples are included
	if examples, ok := schema["examples"]; ok {
		assert.NotNil(t, examples)
	}
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