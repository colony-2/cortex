package devcontainer

import (
	"context"
	"testing"
	"time"
)

func TestDockerSDKIntegration(t *testing.T) {
	// Check if Docker is available
	if err := checkDockerAvailable(); err != nil {
		t.Skip("Docker is not available:", err)
	}
	
	client, err := NewDockerClient()
	if err != nil {
		t.Fatal("Failed to create Docker client:", err)
	}
	defer client.Close()
	
	ctx := context.Background()
	
	t.Run("ValidateImage", func(t *testing.T) {
		// Test with a small image
		err := client.ValidateImage(ctx, "alpine:latest")
		if err != nil {
			t.Errorf("Failed to validate alpine image: %v", err)
		}
	})
	
	t.Run("CreateAndRemoveContainer", func(t *testing.T) {
		config := &DockerRunConfig{
			Image:           "alpine:latest",
			Name:            "test-container-" + time.Now().Format("20060102150405"),
			Command:         []string{"sleep", "300"},
			WorkspaceFolder: "/workspace",
			Environment: map[string]string{
				"TEST_ENV": "test_value",
			},
		}
		
		// Create container
		containerID, err := client.CreateContainer(ctx, config)
		if err != nil {
			t.Fatal("Failed to create container:", err)
		}
		
		// Ensure cleanup
		defer func() {
			err := client.RemoveContainer(ctx, containerID)
			if err != nil {
				t.Errorf("Failed to remove container: %v", err)
			}
		}()
		
		// Check status
		status, err := client.GetContainerStatus(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to get container status:", err)
		}
		
		if status != "created" {
			t.Errorf("Expected status 'created', got '%s'", status)
		}
	})
	
	t.Run("StartStopContainer", func(t *testing.T) {
		config := &DockerRunConfig{
			Image:           "alpine:latest",
			Name:            "test-start-stop-" + time.Now().Format("20060102150405"),
			Command:         []string{"sleep", "300"},
			WorkspaceFolder: "/workspace",
		}
		
		// Create container
		containerID, err := client.CreateContainer(ctx, config)
		if err != nil {
			t.Fatal("Failed to create container:", err)
		}
		
		// Ensure cleanup
		defer func() {
			client.RemoveContainer(ctx, containerID)
		}()
		
		// Start container
		err = client.StartContainer(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to start container:", err)
		}
		
		// Wait a bit and check status
		time.Sleep(1 * time.Second)
		status, err := client.GetContainerStatus(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to get container status:", err)
		}
		
		if status != "running" {
			t.Errorf("Expected status 'running', got '%s'", status)
		}
		
		// Stop container
		err = client.StopContainer(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to stop container:", err)
		}
		
		// Check status again
		status, err = client.GetContainerStatus(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to get container status after stop:", err)
		}
		
		if status != "exited" {
			t.Errorf("Expected status 'exited', got '%s'", status)
		}
	})
	
	t.Run("ExecInContainer", func(t *testing.T) {
		config := &DockerRunConfig{
			Image:           "alpine:latest",
			Name:            "test-exec-" + time.Now().Format("20060102150405"),
			Command:         []string{"sleep", "300"},
			WorkspaceFolder: "/workspace",
		}
		
		// Create and start container
		containerID, err := client.CreateContainer(ctx, config)
		if err != nil {
			t.Fatal("Failed to create container:", err)
		}
		
		defer client.RemoveContainer(ctx, containerID)
		
		err = client.StartContainer(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to start container:", err)
		}
		
		// Execute command
		output, err := client.ExecInContainer(ctx, containerID, []string{"echo", "hello from exec"})
		if err != nil {
			t.Fatal("Failed to exec in container:", err)
		}
		
		if output != "hello from exec\n" {
			t.Errorf("Expected 'hello from exec\\n', got '%s'", output)
		}
		
		// Stop container
		err = client.StopContainer(ctx, containerID)
		if err != nil {
			t.Logf("Warning: Failed to stop container: %v", err)
		}
	})
	
	t.Run("ContainerWithMounts", func(t *testing.T) {
		// Create a temp directory for bind mount
		tmpDir := t.TempDir()
		
		config := &DockerRunConfig{
			Image:           "alpine:latest",
			Name:            "test-mounts-" + time.Now().Format("20060102150405"),
			Command:         []string{"sleep", "300"},
			WorkspaceFolder: "/workspace",
			Mounts: []string{
				"type=bind,source=" + tmpDir + ",target=/host-data",
				"type=volume,source=test-volume,target=/volume-data",
			},
		}
		
		// Create and start container
		containerID, err := client.CreateContainer(ctx, config)
		if err != nil {
			t.Fatal("Failed to create container with mounts:", err)
		}
		
		defer client.RemoveContainer(ctx, containerID)
		
		err = client.StartContainer(ctx, containerID)
		if err != nil {
			t.Fatal("Failed to start container with mounts:", err)
		}
		
		// Test bind mount by checking if directory exists
		output, err := client.ExecInContainer(ctx, containerID, []string{"ls", "-la", "/host-data"})
		if err != nil {
			t.Fatal("Failed to list bind mount:", err)
		}
		
		if output == "" {
			t.Error("Bind mount directory not accessible")
		}
		
		// Test volume mount
		output, err = client.ExecInContainer(ctx, containerID, []string{"ls", "-la", "/volume-data"})
		if err != nil {
			t.Fatal("Failed to list volume mount:", err)
		}
		
		if output == "" {
			t.Error("Volume mount directory not accessible")
		}
		
		// Stop container
		err = client.StopContainer(ctx, containerID)
		if err != nil {
			t.Logf("Warning: Failed to stop container: %v", err)
		}
	})
	
	t.Run("GetContainerLogs", func(t *testing.T) {
		config := &DockerRunConfig{
			Image:   "alpine:latest",
			Name:    "test-logs-" + time.Now().Format("20060102150405"),
			Command: []string{"sh", "-c", "echo 'Line 1'; echo 'Line 2'; echo 'Line 3'"},
		}
		
		// Run container (create and start)
		err := client.RunContainer(ctx, config)
		if err != nil {
			t.Fatal("Failed to run container:", err)
		}
		
		// Give it time to complete
		time.Sleep(2 * time.Second)
		
		// Get container ID by name (we need to implement this or track it)
		// For now, just skip the logs test
		t.Skip("Need to implement container lookup by name")
	})
}