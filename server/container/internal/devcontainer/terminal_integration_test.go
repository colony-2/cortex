//go:build integration
// +build integration

package devcontainer

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

func TestTerminalAttachment_Integration(t *testing.T) {
	// Skip if Docker is not available
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skip("Docker not available:", err)
	}
	defer cli.Close()

	ctx := context.Background()

	// Create a test container with TTY enabled
	config := &container.Config{
		Image:     "alpine:latest",
		Cmd:       []string{"sh", "-c", "echo 'Hello from container'; sleep 1"},
		Tty:       true,  // This is crucial - TTY must be enabled
		OpenStdin: true,
	}

	hostConfig := &container.HostConfig{
		AutoRemove: true,
	}

	// Pull image if needed
	reader, err := cli.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("Failed to pull image: %v", err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	// Create container
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Start container
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Test attaching to the container
	attachOptions := container.AttachOptions{
		Stream: true,
		Stdin:  false,  // Don't attach stdin for this test
		Stdout: true,
		Stderr: true,
	}

	hijacked, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	if err != nil {
		t.Fatalf("Failed to attach to container: %v", err)
	}
	defer hijacked.Close()

	// Read output - with TTY enabled, output should be raw (not multiplexed)
	buf := make([]byte, 1024)
	n, err := hijacked.Reader.Read(buf)
	if err != nil && err.Error() != "EOF" {
		// Check if the error is the "Unrecognized input header" error
		if strings.Contains(err.Error(), "Unrecognized input header") {
			t.Fatalf("Got multiplexing error with TTY enabled - this indicates the bug: %v", err)
		}
	}

	output := string(buf[:n])
	if !strings.Contains(output, "Hello from container") {
		t.Errorf("Expected output to contain 'Hello from container', got: %q", output)
	}

	// Clean up - wait for container to exit
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case <-statusCh:
		// Container exited
	case err := <-errCh:
		t.Logf("Container wait error: %v", err)
	case <-time.After(5 * time.Second):
		t.Log("Container did not exit in time")
	}
}

func TestTerminalWithEscapeSequences(t *testing.T) {
	// This test verifies that escape sequences (like color codes) don't cause parsing errors
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skip("Docker not available:", err)
	}
	defer cli.Close()

	ctx := context.Background()

	// Create a container that outputs escape sequences
	config := &container.Config{
		Image: "alpine:latest",
		// Echo with color escape sequences (ESC = 27 in ASCII)
		Cmd:       []string{"sh", "-c", "echo -e '\\033[31mRed Text\\033[0m'; sleep 1"},
		Tty:       true,
		OpenStdin: true,
	}

	hostConfig := &container.HostConfig{
		AutoRemove: true,
	}

	// Create and start container
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Attach and read output
	attachOptions := container.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	}

	hijacked, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	if err != nil {
		t.Fatalf("Failed to attach: %v", err)
	}
	defer hijacked.Close()

	// Read the output - should not error on escape sequences
	var output bytes.Buffer
	buf := make([]byte, 1024)
	
	for {
		n, err := hijacked.Reader.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if err != nil {
			if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "closed") {
				// Check for the specific multiplexing error
				if strings.Contains(err.Error(), "Unrecognized input header: 27") {
					t.Fatalf("Got escape sequence parsing error - this is the bug we're fixing: %v", err)
				}
			}
			break
		}
	}

	// The output should contain the text (escape sequences may or may not be visible)
	outputStr := output.String()
	if !strings.Contains(outputStr, "Red Text") {
		t.Errorf("Expected output to contain 'Red Text', got: %q", outputStr)
	}
}