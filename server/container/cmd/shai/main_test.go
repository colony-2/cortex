package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
	"github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

func TestShaiInitialization(t *testing.T) {
	// Skip if Docker is not available (check by trying to create a client)
	testClient, err := devcontainer.NewManager()
	if err != nil {
		t.Skip("Skipping integration test - Docker not available:", err)
	}
	testClient.Close()

	// Create temporary directory for test
	tmpDir := t.TempDir()
	
	// Create a minimal .devcontainer/devcontainer.json
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}
	
    devcontainerJSON := `{
        "image": "alpine:latest",
        "command": "/bin/sh"
    }`
	
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}
	
	// Create manager
	manager, err := devcontainer.NewManager()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()
	
	// Create shai runner
	config := shai.Config{
		WorkingDir:     tmpDir,
		ReadWritePaths: []string{"."},
		ContainerName:  "test-shai-" + time.Now().Format("20060102150405"),
	}
	
	runner, err := shai.New(config, manager)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()
	
	// Set up progress callback to verify phases
	progressPhases := []shai.Phase{}
	runner.OnProgress(func(phase shai.Phase, message string) {
		progressPhases = append(progressPhases, phase)
		t.Logf("Progress: [%s] %s", phase, message)
	})
	
	// Start container
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	info, err := runner.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}
	
	// Verify container was created
	if info.ID == "" {
		t.Error("Container ID should not be empty")
	}
	
	// Verify progress phases were reported
	expectedPhases := []shai.Phase{
		shai.PhaseValidating,
		shai.PhaseCreating,
		shai.PhaseStarting,
	}
	
	if len(progressPhases) < len(expectedPhases) {
		t.Errorf("Expected at least %d progress phases, got %d", len(expectedPhases), len(progressPhases))
	}
	
	// Clean up container
	if err := manager.Stop(ctx, info.ID); err != nil {
		t.Logf("Warning: Failed to stop container: %v", err)
	}
	
	if err := manager.Remove(ctx, info.ID); err != nil {
		t.Logf("Warning: Failed to remove container: %v", err)
	}
}

func TestMultiStringFlag(t *testing.T) {
	var flag MultiStringFlag
	
	// Test setting values
	if err := flag.Set("dir1"); err != nil {
		t.Errorf("Failed to set first value: %v", err)
	}
	
	if err := flag.Set("dir2"); err != nil {
		t.Errorf("Failed to set second value: %v", err)
	}
	
	if err := flag.Set("dir3"); err != nil {
		t.Errorf("Failed to set third value: %v", err)
	}
	
	// Verify all values are stored
	if len(flag) != 3 {
		t.Errorf("Expected 3 values, got %d", len(flag))
	}
	
	// Verify values are correct
	expected := []string{"dir1", "dir2", "dir3"}
	for i, v := range flag {
		if v != expected[i] {
			t.Errorf("Expected value %d to be %q, got %q", i, expected[i], v)
		}
	}
	
	// Test String() method
	str := flag.String()
	if str != "[dir1 dir2 dir3]" {
		t.Errorf("Expected string representation to be '[dir1 dir2 dir3]', got %q", str)
	}
}
