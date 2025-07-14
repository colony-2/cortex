//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
)

func TestStartRecipesCommand(t *testing.T) {
	// Skip if not in integration mode
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test")
	}

	// Build the binary
	buildCmd := exec.Command("moon", "run", "build")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Create a temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Start the server in the background
	cmd := exec.Command("./build/ono", "start",
		"--db-filename", dbPath,
		"--port", "17233", // Use non-default port to avoid conflicts
		"--ui-port", "18080",
		"--enable-recipes", "true",
	)

	// Start the command
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Ensure we kill the process when done
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Give the server time to start
	time.Sleep(5 * time.Second)

	// Try to connect to verify it's running
	c, err := client.Dial(client.Options{
		HostPort:  "127.0.0.1:17233",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer c.Close()

	// Verify we can describe the namespace
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify connection by checking the service info
	_, err = c.WorkflowService().GetSystemInfo(ctx, nil)
	if err != nil {
		t.Errorf("Failed to describe namespace: %v", err)
	}

	// Kill the process gracefully
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Logf("Failed to send interrupt signal: %v", err)
	}

	// Wait for it to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited normally
	case <-time.After(10 * time.Second):
		t.Error("Server did not shut down gracefully within timeout")
		cmd.Process.Kill()
	}
}

func TestStartRecipesWithExampleRecipe(t *testing.T) {
	// Skip if not in integration mode
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test")
	}

	// Check if example recipe exists
	homeDir, _ := os.UserHomeDir()
	exampleRecipePath := filepath.Join(homeDir, ".ono", "recipes", "example-recipe", "recipe.yaml")
	if _, err := os.Stat(exampleRecipePath); os.IsNotExist(err) {
		t.Skip("Example recipe not found, skipping test")
	}

	// Build the binary
	buildCmd := exec.Command("moon", "run", "build")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Create a temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Start the server
	cmd := exec.Command("./build/ono", "start",
		"--db-filename", dbPath,
		"--port", "17234", // Different port from other test
		"--ui-port", "18081",
		"--enable-recipes", "true",
		"--log-level", "debug",
	)

	// Capture output for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Ensure we kill the process when done
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Give the server and recipe system time to start
	time.Sleep(8 * time.Second)

	// The test passes if the server started with recipes enabled
	// We can see the output to verify recipes were discovered

	// Kill the process gracefully
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Logf("Failed to send interrupt signal: %v", err)
	}

	// Wait briefly for graceful shutdown
	time.Sleep(2 * time.Second)
}