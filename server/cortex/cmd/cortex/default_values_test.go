// +build integration

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"encoding/json"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to extract JSON from output that might have debug logs
func extractJSONDefault(t *testing.T, output []byte) []byte {
	t.Helper()
	if len(output) == 0 {
		return nil
	}
	jsonStart := strings.Index(string(output), "{")
	if jsonStart == -1 {
		return nil
	}
	return output[jsonStart:]
}

func TestDefaultInputValues(t *testing.T) {
	// Test that default values are properly used when inputs are not provided
	tempDir := t.TempDir()
	
	// Create a recipe with a default value
	recipeContent := `
name: test-defaults
description: Test default values
version: "1.0"

inputs:
  - name: greeting
    type: string
    default: "Hello"
  - name: name
    type: string
    default: "World"

outputs:
  - name: result
    type: string

steps:
  - id: greet
    uses: command_execution
    inputs:
      run: "echo {{ .Inputs.greeting }} {{ .Inputs.name }}"
    outputs:
      stdout: result
`
	recipeFile := filepath.Join(tempDir, "test-defaults.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-tags=integration", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build: %s", output)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Test 1: Execute without any inputs (should use both defaults)
	cmd := exec.Command(cortexPath, "execute", recipeFile, "-o", "json", "--log-level", "error")
	output, err = cmd.Output()
	require.NoError(t, err, "Command should succeed with defaults")
	
	var result ExecutionResult
	err = json.Unmarshal(extractJSONDefault(t, output), &result)
	require.NoError(t, err)
	assert.True(t, result.Success)
	
	// Test 2: Execute with one input (should use one default)
	cmd = exec.Command(cortexPath, "execute", recipeFile, "-i", "greeting=Hi", "-o", "json", "--log-level", "error")
	output, err = cmd.Output()
	require.NoError(t, err, "Command should succeed with partial inputs")
	
	err = json.Unmarshal(extractJSONDefault(t, output), &result)
	require.NoError(t, err)
	assert.True(t, result.Success)
	
	// Test 3: Execute with both inputs (should not use defaults)
	cmd = exec.Command(cortexPath, "execute", recipeFile, "-i", "greeting=Hey", "-i", "name=Test", "-o", "json", "--log-level", "error")
	output, err = cmd.Output()
	require.NoError(t, err, "Command should succeed with all inputs")
	
	err = json.Unmarshal(extractJSONDefault(t, output), &result)
	require.NoError(t, err)
	assert.True(t, result.Success)
}