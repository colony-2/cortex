package devcontainer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/moby/term"
)

// TerminalAttachment handles interactive terminal sessions
type TerminalAttachment struct {
	client      *client.Client
	containerID string
	oldState    *term.State
}

// AttachInteractive attaches an interactive terminal to a container
func (m *Manager) AttachInteractive(ctx context.Context, containerID string) error {
	attachment := &TerminalAttachment{
		client:      m.dockerClient.client,
		containerID: containerID,
	}
	return attachment.Start(ctx)
}

// Start begins an interactive terminal session
func (t *TerminalAttachment) Start(ctx context.Context) error {
	// Check if we have a terminal
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("not running in a terminal")
	}

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	t.oldState = oldState
	
	// Ensure we restore terminal state on exit
	defer t.Cleanup()

	// Create container attach options
	attachOptions := container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	}

	// Attach to container
	var resp types.HijackedResponse
	resp, err = t.client.ContainerAttach(ctx, t.containerID, attachOptions)
	if err != nil {
		return fmt.Errorf("failed to attach to container: %w", err)
	}
	defer resp.Close()

	// Handle terminal resize
	resizeCtx, cancelResize := context.WithCancel(ctx)
	defer cancelResize()
	go t.HandleResize(resizeCtx)

	// Start I/O streaming
	errCh := make(chan error, 2)
	
	// Copy stdin to container
	go func() {
		_, err := io.Copy(resp.Conn, os.Stdin)
		errCh <- err
	}()
	
	// Copy container output to stdout/stderr
	// When TTY is enabled, Docker sends raw output without multiplexing headers
	// So we need to copy directly instead of using stdcopy.StdCopy
	go func() {
		_, err := io.Copy(os.Stdout, resp.Reader)
		errCh <- err
	}()

	// Wait for the container to exit
	statusCh, errWaitCh := t.client.ContainerWait(ctx, t.containerID, container.WaitConditionNotRunning)
	
	// Wait for completion
	select {
	case err := <-errCh:
		if err != nil && err != io.EOF {
			return fmt.Errorf("I/O error: %w", err)
		}
		return nil
	case err := <-errWaitCh:
		if err != nil {
			return fmt.Errorf("container wait error: %w", err)
		}
		return nil
	case <-statusCh:
		// Container exited normally
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HandleResize handles terminal resize events
func (t *TerminalAttachment) HandleResize(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)
	
	// Perform initial resize
	t.resize()
	
	for {
		select {
		case <-sigCh:
			t.resize()
		case <-ctx.Done():
			return
		}
	}
}

// resize updates the container's terminal size
func (t *TerminalAttachment) resize() {
	if t.client == nil || t.containerID == "" {
		return
	}
	
	size, err := term.GetWinsize(os.Stdin.Fd())
	if err != nil {
		// Silently ignore resize errors
		return
	}
	
	options := container.ResizeOptions{
		Height: uint(size.Height),
		Width:  uint(size.Width),
	}
	
	// Best effort resize - ignore errors
	_ = t.client.ContainerResize(context.Background(), t.containerID, options)
}

// Cleanup restores terminal state
func (t *TerminalAttachment) Cleanup() {
	if t.oldState != nil {
		_ = term.RestoreTerminal(os.Stdin.Fd(), t.oldState)
		t.oldState = nil
	}
}