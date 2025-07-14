//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_RecipeCommands tests recipe-related CLI commands
func TestCLI_RecipeCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Build the CLI binary
	binPath := buildCLI(t)

	// Create test recipes directory
	testDir := t.TempDir()
	recipesDir := filepath.Join(testDir, "recipes")
	require.NoError(t, os.MkdirAll(recipesDir, 0755))

	// Start Temporal dev server with recipes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverCmd := exec.CommandContext(ctx, binPath, "start",
		"--recipes", recipesDir,
		"--port", "7234", // Use different port to avoid conflicts
		"--namespace", "test-namespace",
		"--db-filename", filepath.Join(testDir, "temporal.db"),
	)

	// Capture server output
	var serverOut bytes.Buffer
	serverCmd.Stdout = &serverOut
	serverCmd.Stderr = &serverOut

	// Start server in background
	require.NoError(t, serverCmd.Start())
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	// Wait for server to start
	time.Sleep(3 * time.Second)

	// Create test environment
	env := []string{
		"TEMPORAL_ADDRESS=localhost:7234",
		"TEMPORAL_NAMESPACE=test-namespace",
	}

	// Test: List recipes (should be empty initially)
	out := runCommand(t, binPath, env, "recipe", "list")
	assert.Contains(t, out, "No recipes found")

	// Add a test recipe
	recipeContent := `workflow:
  name: cli-test-recipe
  version: 1.0.0
  description: CLI integration test recipe
  
  inputs:
    - name: message
      type: string
      required: true
      
  workflow:
    type: sequential
    steps:
      - id: print
        activity: print-message
        inputs:
          text: "{{ .Inputs.message }}"
          
activities:
  - name: print-message
    description: Print a message
    inputs:
      - name: text
        type: string
        required: true
    outputs:
      - name: result
        type: string
`

	recipePath := filepath.Join(recipesDir, "test-recipe.yaml")
	require.NoError(t, os.WriteFile(recipePath, []byte(recipeContent), 0644))

	// Wait for recipe discovery
	time.Sleep(2 * time.Second)

	// Test: List recipes
	out = runCommand(t, binPath, env, "recipe", "list")
	assert.Contains(t, out, "cli-test-recipe")
	assert.Contains(t, out, "1.0.0")
	assert.Contains(t, out, "CLI integration test recipe")
	assert.Contains(t, out, "Running") // Worker should be running

	// Test: Describe recipe
	out = runCommand(t, binPath, env, "recipe", "describe", "cli-test-recipe")
	assert.Contains(t, out, "Recipe Details")
	assert.Contains(t, out, "Name:")
	assert.Contains(t, out, "cli-test-recipe")
	assert.Contains(t, out, "Version:")
	assert.Contains(t, out, "1.0.0")
	assert.Contains(t, out, "Worker Status:")
	assert.Contains(t, out, "Running")

	// Test: Run recipe
	out = runCommand(t, binPath, env, "recipe", "run", "cli-test-recipe",
		"--input", "message=Hello from CLI test")
	assert.Contains(t, out, "Started job")
	
	// Extract job ID
	lines := strings.Split(out, "\n")
	var jobID string
	for _, line := range lines {
		if strings.Contains(line, "Started job") {
			parts := strings.Fields(line)
			jobID = parts[len(parts)-1]
			break
		}
	}
	require.NotEmpty(t, jobID)

	// Wait for job to complete
	time.Sleep(2 * time.Second)

	// Test: Job history
	out = runCommand(t, binPath, env, "recipe", "history", "cli-test-recipe")
	assert.Contains(t, out, jobID)
	assert.Contains(t, out, "Completed")

	// Test: Job describe
	out = runCommand(t, binPath, env, "job", "describe", jobID)
	assert.Contains(t, out, "Job Details")
	assert.Contains(t, out, "Recipe:")
	assert.Contains(t, out, "cli-test-recipe")
	assert.Contains(t, out, "Status:")
	assert.Contains(t, out, "Completed")
}

// TestCLI_JobCommands tests job-related CLI commands
func TestCLI_JobCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Build the CLI binary
	binPath := buildCLI(t)

	// Create test recipes directory
	testDir := t.TempDir()
	recipesDir := filepath.Join(testDir, "recipes")
	require.NoError(t, os.MkdirAll(recipesDir, 0755))

	// Add a long-running recipe
	recipeContent := `workflow:
  name: long-running-recipe
  version: 1.0.0
  description: Long running recipe for job testing
  
  inputs:
    - name: duration
      type: int
      required: true
      
  workflow:
    type: sequential
    steps:
      - id: sleep
        activity: sleep-activity
        inputs:
          seconds: "{{ .Inputs.duration }}"
          
activities:
  - name: sleep-activity
    description: Sleep for specified duration
    inputs:
      - name: seconds
        type: int
        required: true
`

	recipePath := filepath.Join(recipesDir, "long-running.yaml")
	require.NoError(t, os.WriteFile(recipePath, []byte(recipeContent), 0644))

	// Start Temporal dev server with recipes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverCmd := exec.CommandContext(ctx, binPath, "start",
		"--recipes", recipesDir,
		"--port", "7235",
		"--namespace", "test-namespace-2",
		"--db-filename", filepath.Join(testDir, "temporal2.db"),
	)

	var serverOut bytes.Buffer
	serverCmd.Stdout = &serverOut
	serverCmd.Stderr = &serverOut

	require.NoError(t, serverCmd.Start())
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	// Wait for server to start
	time.Sleep(3 * time.Second)

	env := []string{
		"TEMPORAL_ADDRESS=localhost:7235",
		"TEMPORAL_NAMESPACE=test-namespace-2",
	}

	// Start a long-running job
	out := runCommand(t, binPath, env, "recipe", "run", "long-running-recipe",
		"--input", "duration=30")
	assert.Contains(t, out, "Started job")

	// Extract job ID
	lines := strings.Split(out, "\n")
	var jobID string
	for _, line := range lines {
		if strings.Contains(line, "Started job") {
			parts := strings.Fields(line)
			jobID = parts[len(parts)-1]
			break
		}
	}
	require.NotEmpty(t, jobID)

	// Test: Job describe while running
	out = runCommand(t, binPath, env, "job", "describe", jobID)
	assert.Contains(t, out, "Status:")
	assert.Contains(t, out, "Running")

	// Test: Cancel job
	out = runCommand(t, binPath, env, "job", "cancel", jobID)
	assert.Contains(t, out, "Canceled job")

	// Wait for cancellation
	time.Sleep(1 * time.Second)

	// Verify job was canceled
	out = runCommand(t, binPath, env, "job", "describe", jobID)
	assert.Contains(t, out, "Status:")
	assert.Contains(t, out, "Canceled")

	// Test: Restart job
	out = runCommand(t, binPath, env, "job", "restart", jobID)
	assert.Contains(t, out, "Restarted job")
	
	// Should get a new job ID
	lines = strings.Split(out, "\n")
	var newJobID string
	for _, line := range lines {
		if strings.Contains(line, "new job ID:") {
			parts := strings.Fields(line)
			newJobID = parts[len(parts)-1]
			break
		}
	}
	require.NotEmpty(t, newJobID)
	assert.NotEqual(t, jobID, newJobID)
}

// TestCLI_ErrorHandling tests error scenarios
func TestCLI_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	binPath := buildCLI(t)
	testDir := t.TempDir()

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverCmd := exec.CommandContext(ctx, binPath, "start",
		"--recipes", testDir,
		"--port", "7236",
		"--namespace", "test-namespace-3",
		"--db-filename", filepath.Join(testDir, "temporal3.db"),
	)

	var serverOut bytes.Buffer
	serverCmd.Stdout = &serverOut
	serverCmd.Stderr = &serverOut

	require.NoError(t, serverCmd.Start())
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	time.Sleep(3 * time.Second)

	env := []string{
		"TEMPORAL_ADDRESS=localhost:7236",
		"TEMPORAL_NAMESPACE=test-namespace-3",
	}

	// Test: Describe non-existent recipe
	out, err := runCommandExpectError(t, binPath, env, "recipe", "describe", "non-existent")
	assert.Error(t, err)
	assert.Contains(t, out, "recipe not found")

	// Test: Run non-existent recipe
	out, err = runCommandExpectError(t, binPath, env, "recipe", "run", "non-existent")
	assert.Error(t, err)
	assert.Contains(t, out, "recipe not found")

	// Test: Describe non-existent job
	out, err = runCommandExpectError(t, binPath, env, "job", "describe", "job-non-existent")
	assert.Error(t, err)
	assert.Contains(t, out, "workflow not found")

	// Test: Cancel non-existent job
	out, err = runCommandExpectError(t, binPath, env, "job", "cancel", "job-non-existent")
	assert.Error(t, err)
	assert.Contains(t, out, "workflow not found")
}

// TestCLI_Formatting tests output formatting
func TestCLI_Formatting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	binPath := buildCLI(t)
	testDir := t.TempDir()
	recipesDir := filepath.Join(testDir, "recipes")
	require.NoError(t, os.MkdirAll(recipesDir, 0755))

	// Create multiple recipes
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf(`workflow:
  name: format-test-%d
  version: %d.0.0
  description: Formatting test recipe %d
  workflow:
    type: sequential
`, i, i, i)

		path := filepath.Join(recipesDir, fmt.Sprintf("recipe-%d.yaml", i))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverCmd := exec.CommandContext(ctx, binPath, "start",
		"--recipes", recipesDir,
		"--port", "7237",
		"--namespace", "test-namespace-4",
		"--db-filename", filepath.Join(testDir, "temporal4.db"),
	)

	var serverOut bytes.Buffer
	serverCmd.Stdout = &serverOut
	serverCmd.Stderr = &serverOut

	require.NoError(t, serverCmd.Start())
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	time.Sleep(3 * time.Second)

	env := []string{
		"TEMPORAL_ADDRESS=localhost:7237",
		"TEMPORAL_NAMESPACE=test-namespace-4",
	}

	// Test table formatting
	out := runCommand(t, binPath, env, "recipe", "list")
	
	// Check for table headers
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "VERSION")
	assert.Contains(t, out, "DESCRIPTION")
	assert.Contains(t, out, "STATUS")

	// Check for proper alignment
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "format-test-") {
			// Each data line should have consistent spacing
			assert.Regexp(t, `format-test-\d\s+\d\.0\.0\s+Formatting test recipe \d\s+Running`, line)
		}
	}

	// Test color output (if terminal supports it)
	// This would be environment-specific, so we just check basic output
	out = runCommand(t, binPath, env, "recipe", "describe", "format-test-1")
	assert.NotEmpty(t, out)
}

// Helper functions

func buildCLI(t *testing.T) string {
	// Build the CLI binary
	binPath := filepath.Join(t.TempDir(), "ono-test")
	cmd := exec.Command("go", "build", "-o", binPath, "../../cmd/ono")
	cmd.Dir = filepath.Join(os.Getenv("PWD"), "../..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, out)
	}
	return binPath
}

func runCommand(t *testing.T, binPath string, env []string, args ...string) string {
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nArgs: %v\nOutput: %s", err, args, out)
	}
	return string(out)
}

func runCommandExpectError(t *testing.T, binPath string, env []string, args ...string) (string, error) {
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestCLI_EndToEnd tests a complete workflow from recipe creation to job execution
func TestCLI_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	binPath := buildCLI(t)
	testDir := t.TempDir()
	recipesDir := filepath.Join(testDir, "recipes")
	require.NoError(t, os.MkdirAll(recipesDir, 0755))

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverCmd := exec.CommandContext(ctx, binPath, "start",
		"--recipes", recipesDir,
		"--port", "7238",
		"--namespace", "test-namespace-5",
		"--db-filename", filepath.Join(testDir, "temporal5.db"),
	)

	var serverOut bytes.Buffer
	serverCmd.Stdout = &serverOut
	serverCmd.Stderr = &serverOut

	require.NoError(t, serverCmd.Start())
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	time.Sleep(3 * time.Second)

	env := []string{
		"TEMPORAL_ADDRESS=localhost:7238",
		"TEMPORAL_NAMESPACE=test-namespace-5",
	}

	// Create a recipe that processes data
	recipeContent := `workflow:
  name: data-processor
  version: 1.0.0
  description: Process data through multiple stages
  
  inputs:
    - name: inputData
      type: string
      required: true
    - name: stages
      type: int
      required: false
      default: 3
      
  outputs:
    - name: result
      type: string
      
  workflow:
    type: sequential
    steps:
      - id: stage1
        activity: transform-data
        inputs:
          data: "{{ .Inputs.inputData }}"
          stage: 1
      - id: stage2
        activity: transform-data
        inputs:
          data: "{{ .Steps.stage1.outputs.transformed }}"
          stage: 2
      - id: stage3
        activity: transform-data
        inputs:
          data: "{{ .Steps.stage2.outputs.transformed }}"
          stage: 3
    outputs:
      result: "{{ .Steps.stage3.outputs.transformed }}"
          
activities:
  - name: transform-data
    description: Transform data for a given stage
    inputs:
      - name: data
        type: string
        required: true
      - name: stage
        type: int
        required: true
    outputs:
      - name: transformed
        type: string
`

	recipePath := filepath.Join(recipesDir, "data-processor.yaml")
	require.NoError(t, os.WriteFile(recipePath, []byte(recipeContent), 0644))

	// Wait for discovery
	time.Sleep(2 * time.Second)

	// Verify recipe discovered
	out := runCommand(t, binPath, env, "recipe", "list")
	assert.Contains(t, out, "data-processor")

	// Run the recipe
	out = runCommand(t, binPath, env, "recipe", "run", "data-processor",
		"--input", "inputData=test-data-123")
	assert.Contains(t, out, "Started job")

	// Extract job ID
	lines := strings.Split(out, "\n")
	var jobID string
	for _, line := range lines {
		if strings.Contains(line, "Started job") {
			parts := strings.Fields(line)
			jobID = parts[len(parts)-1]
			break
		}
	}
	require.NotEmpty(t, jobID)

	// Monitor job progress
	time.Sleep(2 * time.Second)

	// Check job status
	out = runCommand(t, binPath, env, "job", "describe", jobID)
	assert.Contains(t, out, "data-processor")
	assert.Contains(t, out, "Activities")

	// Update the recipe
	updatedContent := strings.Replace(recipeContent, "1.0.0", "1.1.0", 1)
	updatedContent = strings.Replace(updatedContent, "Process data through multiple stages",
		"Process data through multiple stages (updated)", 1)
	require.NoError(t, os.WriteFile(recipePath, []byte(updatedContent), 0644))

	// Wait for update
	time.Sleep(2 * time.Second)

	// Verify update
	out = runCommand(t, binPath, env, "recipe", "describe", "data-processor")
	assert.Contains(t, out, "1.1.0")
	assert.Contains(t, out, "(updated)")

	// Run with the updated recipe
	out = runCommand(t, binPath, env, "recipe", "run", "data-processor",
		"--input", "inputData=updated-test-data")
	assert.Contains(t, out, "Started job")

	// Check history shows both jobs
	time.Sleep(2 * time.Second)
	out = runCommand(t, binPath, env, "recipe", "history", "data-processor")
	assert.Contains(t, out, jobID) // First job
	lines = strings.Split(out, "\n")
	jobCount := 0
	for _, line := range lines {
		if strings.Contains(line, "job-") || strings.Contains(line, "recipe-data-processor-job-") {
			jobCount++
		}
	}
	assert.GreaterOrEqual(t, jobCount, 2)
}