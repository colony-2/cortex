package shai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/divisive-ai/vibethis/server/container/pkg/container"
	"github.com/docker/docker/api/types/mount"
)

// Config represents shai configuration
type Config struct {
	WorkingDir     string   // Directory containing .devcontainer
	ReadWritePaths []string // Paths to mount as read-write
	ContainerName  string   // Optional container name
	NoCache        bool     // Force rebuild
}

// Runner coordinates shai operations
type Runner struct {
	config           Config
	manager          container.Manager
	mountBuilder     *MountBuilder
	progressReporter *ProgressReporter
	// Store mount configuration to pass to manager
	mounts           []mount.Mount
}

// New creates a new shai runner with a provided container manager
func New(config Config, manager container.Manager) (*Runner, error) {
	// Use current directory if not specified
	if config.WorkingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		config.WorkingDir = wd
	}
	
	// Validate .devcontainer exists
	devPath := filepath.Join(config.WorkingDir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(devPath); err != nil {
		// Try alternate location
		devPath = filepath.Join(config.WorkingDir, ".devcontainer.json")
		if _, err := os.Stat(devPath); err != nil {
			return nil, fmt.Errorf("no devcontainer.json found in %s", config.WorkingDir)
		}
	}
	
	// Create mount builder
	mountBuilder, err := NewMountBuilder(config.WorkingDir, config.ReadWritePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to create mount builder: %w", err)
	}
	
	return &Runner{
		config:           config,
		manager:          manager,
		mountBuilder:     mountBuilder,
		progressReporter: NewProgressReporter(),
		mounts:           mountBuilder.BuildMounts(),
	}, nil
}

// OnProgress sets the progress callback
func (r *Runner) OnProgress(cb ProgressCallback) {
	r.progressReporter.SetCallback(cb)
}

// Start creates and starts the container
func (r *Runner) Start(ctx context.Context) (*container.Info, error) {
	r.progressReporter.Report(PhaseValidating, "Loading devcontainer configuration...")
	
	// Create container with progress reporting
	r.progressReporter.Report(PhaseCreating, "Creating container with mounts...")
	
	// Note: The manager implementation is responsible for:
	// - Loading devcontainer configuration
	// - Applying mounts
	// - Setting auto-remove
	// The shai runner just coordinates the high-level flow
	
	containerID, err := r.manager.Create(ctx, r.config.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	
	r.progressReporter.Report(PhaseStarting, "Starting container...")
	
	// Start container
	err = r.manager.Start(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}
	
	r.progressReporter.Report(PhaseStarting, "Starting interactive shell")
	
	// Get container info
	info, err := r.manager.GetInfo(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}
	
	return info, nil
}

// AttachInteractive attaches an interactive shell
func (r *Runner) AttachInteractive(ctx context.Context, containerID string) error {
	// Check if the manager supports interactive attachment
	if attachable, ok := r.manager.(interface {
		AttachInteractive(context.Context, string) error
	}); ok {
		return attachable.AttachInteractive(ctx, containerID)
	}
	return fmt.Errorf("manager does not support interactive attachment")
}

// Close cleans up resources
func (r *Runner) Close() error {
	// Check if the manager implements Close
	if closer, ok := r.manager.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

