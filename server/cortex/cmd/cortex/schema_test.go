package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/colony-2/colony2/server/cortex/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGenerateCompleteSchema(t *testing.T) {
	// Ensure ops registered
	_ = zap.NewNop()
	shared.RegisterOps()
	// Generate schema using shared schema manager
	sm := shared.NewSchemaManager()
	schema, err := sm.GenerateCompleteSchema("", false)
	require.NoError(t, err)

	// Basic presence
	assert.NotEmpty(t, schema)

	// Check definitions exist and include Node
	var defs map[string]interface{}
	if d, ok := schema["$defs"].(map[string]interface{}); ok {
		defs = d
	} else if d, ok := schema["definitions"].(map[string]interface{}); ok {
		defs = d
	}
	require.NotNil(t, defs)
	assert.Contains(t, defs, "Node")
}

func TestSchemaCommandJSON(t *testing.T) {
	// Test JSON output format
	_ = zap.NewNop()
	shared.RegisterOps()
	sm := shared.NewSchemaManager()
	schema, err := sm.GenerateCompleteSchema("", false)
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
	_ = zap.NewNop()
	shared.RegisterOps()
	sm := shared.NewSchemaManager()
	schema, err := sm.GenerateCompleteSchema("command_execution", false)
	require.NoError(t, err)
	assert.NotNil(t, schema)
}

func TestSchemaWithVersion(t *testing.T) {
	// Test schema with version included
	_ = zap.NewNop()
	shared.RegisterOps()
	sm := shared.NewSchemaManager()
	schema, err := sm.GenerateCompleteSchema("", true)
	require.NoError(t, err)

	// Check version field exists
	assert.Contains(t, schema, "version")
	assert.Equal(t, "1.0.0", schema["version"])
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
	assert.Equal(t, "3.1.0", openAPI["openapi"])

	info, ok := openAPI["info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Vibethis Recipe Schema API", info["title"])
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
