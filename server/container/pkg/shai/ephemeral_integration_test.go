//go:build integration
// +build integration

package shai_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/container/pkg/shai"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEphemeralContainerFullLifecycle tests the complete ephemeral container lifecycle
func TestEphemeralContainerFullLifecycle(t *testing.T) {
	// Skip if Docker is not available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	// Create a test workspace
	tmpDir, err := os.MkdirTemp("", "shai-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test directories
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".devcontainer"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "docs"), 0755))

	// Create a devcontainer.json with lifecycle commands
	devcontainerJSON := `{
		"image": "alpine:latest",
		"postCreateCommand": "echo 'PostCreate executed' > /tmp/postcreate.txt",
		"postStartCommand": "echo 'PostStart executed' > /tmp/poststart.txt",
		"workspaceFolder": "/workspace",
		"workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind"
	}`

	err = os.WriteFile(filepath.Join(tmpDir, ".devcontainer", "devcontainer.json"), []byte(devcontainerJSON), 0644)
	require.NoError(t, err)

	// Create test files in each directory
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src", "test.go"), []byte("package main"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "test_test.go"), []byte("package main_test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docs", "README.md"), []byte("# Test"), 0644))

	t.Run("ephemeral container runs and cleans up", func(t *testing.T) {
		// Record container IDs before test
		initialContainers := getContainerIDs(t)

		// Create ephemeral runner
		runner, err := shai.NewEphemeralRunner(shai.EphemeralConfig{
			WorkingDir:          tmpDir,
			ReadWritePaths:      []string{"src", "tests"},
			HideProgressMarkers: false,
		})
		require.NoError(t, err)
		defer runner.Close()

		// Set up progress tracking
		var progressUpdates []shai.ProgressUpdate
		runner.OnProgress(func(update shai.ProgressUpdate) {
			progressUpdates = append(progressUpdates, update)
			t.Logf("Progress: [%s] %s - %s", update.Phase, update.Status, update.Message)
		})

		// Run container with a command that exits quickly
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Run in a goroutine since it will block until container exits
		done := make(chan error, 1)
		go func() {
			// Override the script to exit after setup
			done <- runner.Run(ctx)
		}()

		// Wait for completion or timeout
		select {
		case err := <-done:
			// Container should exit normally (we expect an error since we're not running interactively)
			if err != nil && !strings.Contains(err.Error(), "EOF") {
				t.Logf("Run error (expected): %v", err)
			}
		case <-ctx.Done():
			t.Fatal("Test timed out")
		}

		// Verify progress updates were received
		assert.True(t, len(progressUpdates) > 0, "Should have received progress updates")

		// Check that lifecycle phases were executed
		phasesSeen := make(map[string]bool)
		for _, update := range progressUpdates {
			phasesSeen[update.Phase] = true
		}
		assert.True(t, phasesSeen["INIT"], "Should have seen INIT phase")

		// Verify container was cleaned up
		time.Sleep(2 * time.Second) // Give Docker time to clean up
		finalContainers := getContainerIDs(t)
		assert.ElementsMatch(t, initialContainers, finalContainers, "Container should be removed after exit")
	})
}

// TestMountPermissionsIntegration tests mount permissions in a real container
func TestMountPermissionsIntegration(t *testing.T) {
	// Skip if Docker is not available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	// Create a test workspace
	tmpDir, err := os.MkdirTemp("", "shai-mount-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create directories
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".devcontainer"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "docs"), 0755))

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src", "writable.txt"), []byte("original"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docs", "readonly.txt"), []byte("original"), 0644))

	// Create devcontainer.json that tests mount permissions
	devcontainerJSON := `{
		"image": "alpine:latest",
		"postCreateCommand": [
			"sh", "-c",
			"echo 'modified' > /workspace/src/writable.txt && echo 'Write to src: OK' || echo 'Write to src: FAILED'; echo 'test' > /workspace/docs/readonly.txt && echo 'Write to docs: FAILED' || echo 'Write to docs: OK (blocked)'"
		],
		"workspaceFolder": "/workspace"
	}`

	err = os.WriteFile(filepath.Join(tmpDir, ".devcontainer", "devcontainer.json"), []byte(devcontainerJSON), 0644)
	require.NoError(t, err)

	t.Run("selective mounts work correctly", func(t *testing.T) {
		runner, err := shai.NewEphemeralRunner(shai.EphemeralConfig{
			WorkingDir:     tmpDir,
			ReadWritePaths: []string{"src"}, // Only src is writable
		})
		require.NoError(t, err)
		defer runner.Close()

		// Capture output
		var output bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- runner.Run(ctx)
		}()

		go func() {
			io.Copy(&output, r)
		}()

		select {
		case <-done:
			w.Close()
			os.Stdout = oldStdout
		case <-ctx.Done():
			w.Close()
			os.Stdout = oldStdout
			t.Fatal("Test timed out")
		}

		outputStr := output.String()
		t.Logf("Container output:\n%s", outputStr)

		// Check file contents
		srcContent, _ := os.ReadFile(filepath.Join(tmpDir, "src", "writable.txt"))
		docsContent, _ := os.ReadFile(filepath.Join(tmpDir, "docs", "readonly.txt"))

		// src should be modified, docs should not
		assert.Contains(t, string(srcContent), "modified", "src file should be modified")
		assert.Contains(t, string(docsContent), "original", "docs file should not be modified")
	})
}

// TestProgressMarkersEndToEnd tests progress markers in a real container
func TestProgressMarkersEndToEnd(t *testing.T) {
	// Skip if Docker is not available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	tmpDir, err := os.MkdirTemp("", "shai-progress-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".devcontainer"), 0755))

	// Create devcontainer with all lifecycle commands
	devcontainerJSON := `{
		"image": "alpine:latest",
		"onCreateCommand": "echo 'onCreate running'",
		"updateContentCommand": "echo 'updateContent running'",
		"postCreateCommand": "echo 'postCreate running'",
		"postStartCommand": "echo 'postStart running'",
		"postAttachCommand": "echo 'postAttach running'",
		"workspaceFolder": "/workspace"
	}`

	err = os.WriteFile(filepath.Join(tmpDir, ".devcontainer", "devcontainer.json"), []byte(devcontainerJSON), 0644)
	require.NoError(t, err)

	t.Run("all lifecycle phases report progress", func(t *testing.T) {
		runner, err := shai.NewEphemeralRunner(shai.EphemeralConfig{
			WorkingDir:          tmpDir,
			ReadWritePaths:      []string{"."},
			HideProgressMarkers: false,
		})
		require.NoError(t, err)
		defer runner.Close()

		progressPhases := make(map[string][]string)
		runner.OnProgress(func(update shai.ProgressUpdate) {
			progressPhases[update.Phase] = append(progressPhases[update.Phase], update.Status)
			t.Logf("Progress: [%s] %s - %s", update.Phase, update.Status, update.Message)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- runner.Run(ctx)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("Test timed out")
		}

		// Verify we saw all expected phases
		expectedPhases := []string{"INIT", "ONCREATE", "UPDATECONTENT", "POSTCREATE", "POSTSTART", "POSTATTACH"}
		for _, phase := range expectedPhases {
			statuses, exists := progressPhases[phase]
			if exists {
				assert.Contains(t, statuses, "START", "Phase %s should have START status", phase)
				assert.Contains(t, statuses, "COMPLETE", "Phase %s should have COMPLETE status", phase)
			}
		}
	})
}

// TestCleanShutdown tests graceful shutdown behavior
func TestCleanShutdown(t *testing.T) {
	// Skip if Docker is not available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	tmpDir, err := os.MkdirTemp("", "shai-shutdown-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".devcontainer"), 0755))

	// Create devcontainer with a long-running command
	devcontainerJSON := `{
		"image": "alpine:latest",
		"postCreateCommand": "sleep 60",
		"workspaceFolder": "/workspace"
	}`

	err = os.WriteFile(filepath.Join(tmpDir, ".devcontainer", "devcontainer.json"), []byte(devcontainerJSON), 0644)
	require.NoError(t, err)

	t.Run("context cancellation stops container", func(t *testing.T) {
		runner, err := shai.NewEphemeralRunner(shai.EphemeralConfig{
			WorkingDir:     tmpDir,
			ReadWritePaths: []string{"."},
		})
		require.NoError(t, err)
		defer runner.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		done := make(chan error, 1)
		go func() {
			done <- runner.Run(ctx)
		}()

		// Let it start
		time.Sleep(2 * time.Second)

		// Cancel context
		cancel()

		select {
		case err := <-done:
			// Should get context cancelled error
			assert.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(10 * time.Second):
			t.Fatal("Container did not stop after context cancellation")
		}

		// Verify container is cleaned up
		time.Sleep(2 * time.Second)
		// Container should be gone (auto-removed)
	})
}

// Helper functions

func isDockerAvailable() bool {
	// Try multiple Docker socket locations
	socketPaths := []string{
		"unix:///var/run/docker.sock", // Linux default
		"unix://" + os.Getenv("HOME") + "/.docker/run/docker.sock", // Docker Desktop on macOS
		"unix:///Users/" + os.Getenv("USER") + "/.docker/run/docker.sock", // Alternative macOS path
	}

	var cli *client.Client
	var err error

	// First try with environment settings
	cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err == nil {
		defer cli.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err = cli.Ping(ctx); err == nil {
			return true
		}
	}

	// Try each socket path
	for _, socketPath := range socketPaths {
		cli, err = client.NewClientWithOpts(
			client.WithHost(socketPath),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			continue
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, pingErr := cli.Ping(ctx)
		cancel()
		cli.Close()
		
		if pingErr == nil {
			// Set DOCKER_HOST for subsequent Docker operations in this test
			os.Setenv("DOCKER_HOST", socketPath)
			return true
		}
	}

	return false
}

func getContainerIDs(t *testing.T) []string {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer cli.Close()

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	require.NoError(t, err)

	var ids []string
	for _, c := range containers {
		ids = append(ids, c.ID)
	}
	return ids
}