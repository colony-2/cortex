//go:build integration
// +build integration

package devcontainer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

func TestTerminalAttachment_Integration(t *testing.T) {
	// Try to connect to Docker using common socket paths
	cli, err := getDockerClient()
	if err != nil {
		t.Fatalf("Failed to connect to Docker: %v", err)
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

	// Test attaching to the container BEFORE starting so we don't miss output
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

	// Start container after attaching
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

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
	cli, err := getDockerClient()
	if err != nil {
		t.Fatalf("Failed to connect to Docker: %v", err)
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

	// Create container
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Attach BEFORE starting to capture all output
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

	// Start container after attaching
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

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

// getDockerClient returns a Docker client, trying common socket paths
func getDockerClient() (*client.Client, error) {
	// Try common Docker socket paths (macOS first, then Linux)
	socketPaths := []string{
		"unix://" + os.Getenv("HOME") + "/.docker/run/docker.sock", // Docker Desktop on macOS
		"unix:///var/run/docker.sock",                               // Linux default
	}

	for _, socketPath := range socketPaths {
		cli, err := client.NewClientWithOpts(
			client.WithHost(socketPath),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, pingErr := cli.Ping(ctx)
		cancel()

		if pingErr == nil {
			return cli, nil
		}
		cli.Close()
	}

	// Finally try with environment settings as fallback
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err = cli.Ping(ctx); err == nil {
			return cli, nil
		}
		cli.Close()
	}

	return nil, fmt.Errorf("could not connect to Docker daemon")
}