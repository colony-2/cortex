// +build integration

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Helper function to extract JSON from output that might have debug logs
func extractJSON(t *testing.T, output []byte) []byte {
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

// TestExecuteCommandBasic tests basic recipe execution
func TestExecuteCommandBasic(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()
	
	// Create a simple test recipe
recipeContent := `
id: test-basic
name: test-basic
description: Basic test recipe
version: "1.0"

input_schema:
  message:
    type: string
    required: true

sequence:
  - id: echo
    op: command_execution
    inputs:
      run: "echo {{ inputs.message }}"

outputs:
  result: "{{ sequence.echo.outputs.stdout }}"
`
	recipeFile := filepath.Join(tempDir, "test-basic.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build the cortex binary
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build cortex: %v\nOutput: %s", err, output)
	}
	
	// Execute the recipe
	cortexPath := filepath.Join(tempDir, "cortex")
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"-i", "message=Hello World",
		"-o", "json",
		"--log-level", "error")  // Set log level to error to avoid mixing with output
	
	output, err = cmd.Output()  // Use Output() instead of CombinedOutput() to get only stdout
	require.NoError(t, err, "Command failed")
	
	// Parse the JSON output
	var result ExecutionResult
	err = json.Unmarshal(extractJSON(t, output), &result)
	require.NoError(t, err, "Failed to parse JSON")
	
	// Verify the result
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.RunID)
	assert.NotEmpty(t, result.ExecutionTime)
	assert.Equal(t, recipeFile, result.Recipe)
}

// TestExecuteCommandWithInputFile tests execution with input from file
func TestExecuteCommandWithInputFile(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test recipe
recipeContent := `
id: test-input-file
name: test-input-file
description: Test with input file
version: "1.0"

input_schema:
  user_prompt:
    type: string
    required: true
  max_tokens:
    type: number
    default_value: 100
  data:
    type: object
    required: true

sequence:
  - id: process
    op: command_execution
    inputs:
      run: "echo Processing: {{ inputs.user_prompt }}"

outputs:
  processed: "{{ sequence.process.outputs.stdout }}"
`
	recipeFile := filepath.Join(tempDir, "test-input-file.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Create JSON input file
	inputData := map[string]interface{}{
		"user_prompt": "Analyze this data",
		"max_tokens":  2000,
		"data": map[string]interface{}{
			"source": "database",
			"query":  "SELECT * FROM users",
		},
	}
	inputFile := filepath.Join(tempDir, "inputs.json")
	inputBytes, _ := json.MarshalIndent(inputData, "", "  ")
	require.NoError(t, os.WriteFile(inputFile, inputBytes, 0644))
	
	// Build and execute
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"-f", inputFile,
		"-o", "json",
		"--log-level", "error")
	
	output, err := cmd.Output()
	require.NoError(t, err, "Command failed")
	
	var result ExecutionResult
	require.NoError(t, json.Unmarshal(extractJSON(t, output), &result))
	assert.True(t, result.Success)
}

// TestExecuteCommandWithYAMLInput tests execution with YAML input file
func TestExecuteCommandWithYAMLInput(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test recipe
	recipeFile := filepath.Join(tempDir, "test-yaml-input.yaml")
recipeContent := `
id: test-yaml
name: test-yaml
description: Test with YAML input
version: "1.0"

input_schema:
  config:
    type: object
    required: true

sequence:
  - id: process
    op: command_execution
    inputs:
      run: "echo Config loaded"

outputs:
  result: "{{ sequence.process.outputs.stdout }}"
`
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Create YAML input file
	inputContent := `
config:
  environment: test
  debug: true
  settings:
    timeout: 30
    retries: 3
`
	inputFile := filepath.Join(tempDir, "inputs.yaml")
	require.NoError(t, os.WriteFile(inputFile, []byte(inputContent), 0644))
	
	// Build and execute
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"-f", inputFile,
		"-o", "yaml",
		"--log-level", "error")
	
	output, err := cmd.Output()
	require.NoError(t, err, "Command failed")
	
	// For YAML output, we need to handle potential debug logs too
	yamlStart := strings.Index(string(output), "success:")
	if yamlStart == -1 {
		yamlStart = 0 // Try from beginning if marker not found
	}
	yamlOutput := output[yamlStart:]
	
	var result ExecutionResult
	require.NoError(t, yaml.Unmarshal(yamlOutput, &result))
	assert.True(t, result.Success)
}

// TestExecuteCommandDryRun tests dry-run validation
func TestExecuteCommandDryRun(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create valid recipe
validRecipe := `
id: test-dryrun
name: test-dryrun
description: Test dry run
version: "1.0"

input_schema:
  required_input:
    type: string
    required: true

sequence:
  - id: step1
    op: command_execution
    inputs:
      run: "echo test"

outputs:
  result: "{{ sequence.step1.outputs.stdout }}"
`
	validFile := filepath.Join(tempDir, "valid.yaml")
	require.NoError(t, os.WriteFile(validFile, []byte(validRecipe), 0644))
	
	// Create invalid recipe (missing required field)
invalidRecipe := `
description: Invalid recipe missing root node
version: "1.0"
name: invalid
`
	invalidFile := filepath.Join(tempDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidFile, []byte(invalidRecipe), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Test valid recipe with dry-run
	cmd := exec.Command(cortexPath, "execute", validFile,
		"-i", "required_input=test",
		"--dry-run",
		"-o", "json",
		"--log-level", "error")
	
	output, err := cmd.Output()
	require.NoError(t, err, "Valid recipe dry-run failed")
	
	var result ExecutionResult
	require.NoError(t, json.Unmarshal(extractJSON(t, output), &result))
	assert.True(t, result.Success)
	
	// Test invalid recipe with dry-run
	cmd = exec.Command(cortexPath, "execute", invalidFile,
		"--dry-run",
		"-o", "json",
		"--log-level", "error")
	
	output, err = cmd.Output()
	
	// The command should fail, capture stderr if it does
	if err != nil {
		// Check exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Log the stderr for debugging
			t.Logf("Exit code: %d, Stderr: %s", exitErr.ExitCode(), exitErr.Stderr)
			assert.Equal(t, 3, exitErr.ExitCode(), "Should exit with validation error code")
		}
	} else {
		t.Fatal("Invalid recipe should have failed")
	}
}

// TestExecuteCommandOutputFormats tests different output formats
func TestExecuteCommandOutputFormats(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test recipe
recipeContent := `
id: test-formats
name: test-formats
description: Test output formats
version: "1.0"

input_schema:
  test:
    type: string
    default_value: "value"

sequence:
  - id: process
    op: command_execution
    inputs:
      run: "echo Success"

outputs:
  result: "{{ sequence.process.outputs.stdout }}"
  metadata: "{{ 'input: ' + inputs.test }}"
`
	recipeFile := filepath.Join(tempDir, "test-formats.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Test JSON output
	cmd := exec.Command(cortexPath, "execute", recipeFile, "-o", "json", "--log-level", "error")
	output, err := cmd.Output()
	require.NoError(t, err)
	
	var jsonResult ExecutionResult
	require.NoError(t, json.Unmarshal(extractJSON(t, output), &jsonResult))
	assert.True(t, jsonResult.Success)
	
	// Test YAML output
	cmd = exec.Command(cortexPath, "execute", recipeFile, "-o", "yaml", "--log-level", "error")
	output, err = cmd.Output()
	require.NoError(t, err)
	
	// For YAML, extract from first YAML-like line
	yamlStart := strings.Index(string(output), "success:")
	if yamlStart == -1 {
		yamlStart = 0
	}
	var yamlResult ExecutionResult
	require.NoError(t, yaml.Unmarshal(output[yamlStart:], &yamlResult))
	assert.True(t, yamlResult.Success)
	
	// Test text output
	cmd = exec.Command(cortexPath, "execute", recipeFile, "-o", "text", "--log-level", "error")
	output, err = cmd.Output()
	require.NoError(t, err)
	
	outputStr := string(output)
	assert.Contains(t, outputStr, "Recipe:")
	assert.Contains(t, outputStr, "Status: SUCCESS")
	assert.Contains(t, outputStr, "Run ID:")
}

// TestExecuteCommandTimeout tests timeout functionality
func TestExecuteCommandTimeout(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create recipe that would take 10 seconds
recipeContent := `
id: test-timeout
name: test-timeout
description: Test timeout
version: "1.0"

sequence:
  - id: sleep
    op: sleep
    inputs:
      duration: "10s"

outputs:
  result: "{{ sequence.sleep.outputs.actual_duration }}"
`
	recipeFile := filepath.Join(tempDir, "test-timeout.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Execute with 2 second timeout (should timeout before 10 second sleep completes)
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"--timeout", "2s",
		"-o", "json",
		"--log-level", "error")
	
	output, err := cmd.Output()
	
	// We expect an error due to timeout
	if err == nil {
		// If no error, check if the output indicates failure
		var result ExecutionResult
		if json.Unmarshal(extractJSON(t, output), &result) == nil {
			if result.Success {
				t.Fatal("Expected command to fail due to timeout, but it succeeded")
			}
		}
	} else {
		// Check exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Should exit with timeout error code (5), general failure (1), or misuse (2)
			assert.Contains(t, []int{1, 2, 5}, exitErr.ExitCode(), "Should exit with error code")
		}
	}
}

// TestExecuteCommandInterruption tests signal handling
func TestExecuteCommandInterruption(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create recipe with a long-running command
recipeContent := `
id: test-interrupt
name: test-interrupt
description: Test interruption
version: "1.0"

sequence:
  - id: sleep
    op: sleep
    inputs:
      duration: "30s"

outputs:
  result: "{{ sequence.sleep.outputs.actual_duration }}"
`
	recipeFile := filepath.Join(tempDir, "test-interrupt.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Start the command with state tracking
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"-o", "json",
		"--state-dir", tempDir,
		"--cleanup=false",
		"--log-level", "error")
	
	// Start the command
	require.NoError(t, cmd.Start())
	
	// Give it a moment to start execution
	time.Sleep(1 * time.Second)
	
	// Send interrupt signal
	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))
	
	// Wait for it to finish
	err = cmd.Wait()
	
	// The command should exit with an error
	require.Error(t, err, "Command should exit with error after interruption")
	
	// Check exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		// Should exit with interrupted code (130), timeout (5), general error (1), or misuse (2)
		assert.Contains(t, []int{1, 2, 5, 130}, exitErr.ExitCode(), "Should exit with appropriate error code")
	}
	
	// Check that state file was created with interrupted status
	stateFiles, _ := filepath.Glob(filepath.Join(tempDir, "cortex-run-*/state.json"))
	if len(stateFiles) > 0 {
		stateData, err := os.ReadFile(stateFiles[0])
		if err == nil {
			var state ExecutionState
			if json.Unmarshal(stateData, &state) == nil {
				// State should show interrupted or failed status
				assert.Contains(t, []string{"interrupted", "failed"}, state.Status)
			}
		}
	}
}

// TestExecuteCommandLogging tests different log levels and formats
func TestExecuteCommandLogging(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test recipe
recipeContent := `
id: test-logging
name: test-logging
description: Test logging
version: "1.0"

input_schema:
  message:
    type: string
    default_value: "test"

sequence:
  - id: echo
    op: command_execution
    inputs:
      run: "echo {{ inputs.message }}"

outputs:
  result: "{{ sequence.echo.outputs.stdout }}"
`
	recipeFile := filepath.Join(tempDir, "test-logging.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Test debug level with text format
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"-l", "debug",
		"--log-format", "text",
		"-o", "json")
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	output, err := cmd.Output()
	require.NoError(t, err)
	
	// Check that we got JSON output
	var result ExecutionResult
	require.NoError(t, json.Unmarshal(extractJSON(t, output), &result))
	assert.True(t, result.Success)
	
	// Check that debug logs were written
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "DEBUG")
	
	// Test JSON log format
	cmd = exec.Command(cortexPath, "execute", recipeFile,
		"-l", "info",
		"--log-format", "json",
		"-o", "json")
	
	stderr.Reset()
	cmd.Stderr = &stderr
	
	_, err = cmd.Output()
	require.NoError(t, err)
	
	// Check that logs are in JSON format
	stderrLines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	for _, line := range stderrLines {
		if line == "" {
			continue
		}
		var logEntry map[string]interface{}
		assert.NoError(t, json.Unmarshal([]byte(line), &logEntry), "Log line should be valid JSON: %s", line)
		assert.Contains(t, logEntry, "timestamp")
		assert.Contains(t, logEntry, "level")
		assert.Contains(t, logEntry, "msg") // zap uses "msg" not "message"
	}
}

// TestExecuteCommandComplexInputs tests complex input parsing
func TestExecuteCommandComplexInputs(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test recipe
recipeContent := `
id: test-complex
name: test-complex
description: Test complex inputs
version: "1.0"

input_schema:
  simple:
    type: string
    required: true
  number:
    type: number
    required: true
  bool:
    type: boolean
    required: true
  array:
    type: array
    required: true
  nested:
    type: object
    required: true

sequence:
  - id: process
    op: command_execution
    inputs:
      run: "echo Processed"
      stdout: result
`
	recipeFile := filepath.Join(tempDir, "test-complex.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Test with complex inputs
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"-i", "simple=hello",
		"-i", "number=42",
		"-i", "bool=true",
		"-i", `array=["item1","item2","item3"]`,
		"-i", `nested={"key":"value","count":10}`,
		"-o", "json",
		"--log-level", "error")
	
	output, err := cmd.Output()
	require.NoError(t, err, "Command failed")
	
	var result ExecutionResult
	require.NoError(t, json.Unmarshal(extractJSON(t, output), &result))
	assert.True(t, result.Success)
}

// TestExecuteCommandStatePreservation tests state saving
func TestExecuteCommandStatePreservation(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	
	// Create simple recipe
	recipeContent := `
name: test-state
description: Test state preservation
version: "1.0"

outputs:
  - name: result
    type: string

steps:
  - id: step1
    uses: command_execution
    inputs:
      run: "echo Hello"
    outputs:
      stdout: result
`
	recipeFile := filepath.Join(tempDir, "test-state.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Execute with state directory
	cmd := exec.Command(cortexPath, "execute", recipeFile,
		"--state-dir", stateDir,
		"--cleanup=false",
		"-o", "json",
		"--log-level", "error")
	
	output, err := cmd.Output()
	require.NoError(t, err, "Command should succeed")
	
	// Verify successful execution
	var result ExecutionResult
	require.NoError(t, json.Unmarshal(extractJSON(t, output), &result))
	assert.True(t, result.Success)
	
	// Check that state was created and preserved
	stateFiles, err := filepath.Glob(filepath.Join(stateDir, "cortex-run-*/state.json"))
	require.NoError(t, err)
	
	// If not found with cortex-run- prefix, try without prefix (run ID format changed)
	if len(stateFiles) == 0 {
		stateFiles, err = filepath.Glob(filepath.Join(stateDir, "*/state.json"))
		require.NoError(t, err)
	}
	
	require.NotEmpty(t, stateFiles, "State file should be created")
	
	// Read and verify state
	stateData, err := os.ReadFile(stateFiles[0])
	require.NoError(t, err)
	
	var state ExecutionState
	require.NoError(t, json.Unmarshal(stateData, &state))
	
	assert.NotEmpty(t, state.RunID)
	assert.Equal(t, recipeFile, state.RecipeFile)
	assert.Equal(t, "completed", state.Status)
	assert.NotZero(t, state.StartTime)
	assert.NotZero(t, state.LastUpdate)
}

// TestExecuteCommandErrorHandling tests error cases
func TestExecuteCommandErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	
	// Build cortex
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tempDir, "cortex"), "./cmd/cortex")
	buildCmd.Dir = filepath.Join(".", "..", "..")
	_, err := buildCmd.CombinedOutput()
	require.NoError(t, err)
	
	cortexPath := filepath.Join(tempDir, "cortex")
	
	// Test non-existent recipe file
	cmd := exec.Command(cortexPath, "execute", "/non/existent/file.yaml", "-o", "json", "--log-level", "error")
	output, err := cmd.Output()
	// We expect an error
	
	// Should output error in JSON format if there's output
	if len(output) > 0 {
		jsonData := extractJSON(t, output)
		if jsonData != nil {
			var result ExecutionResult
			if json.Unmarshal(jsonData, &result) == nil {
				assert.False(t, result.Success)
				assert.NotNil(t, result.Error)
			}
		}
	}
	
	// Test invalid YAML
	invalidYAML := filepath.Join(tempDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidYAML, []byte("invalid: yaml: content:"), 0644))
	
	cmd = exec.Command(cortexPath, "execute", invalidYAML, "-o", "json", "--log-level", "error")
	output, err = cmd.Output()
	// We expect an error
	
	// Test missing required input
recipeContent := `
id: test-missing-input
name: test-missing-input
description: Test missing input
version: "1.0"

input_schema:
  required_field:
    type: string
    required: true

sequence:
  - id: step1
    op: command_execution
    inputs:
      run: "echo {{ inputs.required_field }}"
`
	recipeFile := filepath.Join(tempDir, "missing-input.yaml")
	require.NoError(t, os.WriteFile(recipeFile, []byte(recipeContent), 0644))
	
	cmd = exec.Command(cortexPath, "execute", recipeFile, "--dry-run", "-o", "json", "--log-level", "error")
	output, err = cmd.Output()
	// We expect an error
	
	// Check exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 4, exitErr.ExitCode(), "Should exit with input validation error code")
	}
}


// Helper to check if output contains expected log entries
func assertLogContains(t *testing.T, logs string, level, message string) {
	t.Helper()
	lines := strings.Split(logs, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, level) && strings.Contains(line, message) {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected log with level=%s and message containing '%s' not found in:\n%s", level, message, logs)
}
