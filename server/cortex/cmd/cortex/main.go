// Package main provides the entry point for the vibethis application.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/divisive-ai/vibethis/server/cortex/internal/config"
	"github.com/divisive-ai/vibethis/server/cortex/internal/setup"

	"github.com/spf13/cobra"
)

var (
	// Version is set at build time
	Version = "dev"

	// BuildTime is set at build time
	BuildTime = "unknown"

	// defaultPort is set at build time via ldflags
	defaultPort = "8080"
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Execute runs the CLI application.
func Execute() error {
	var cfg config.Config
	var createNew bool

	rootCmd := &cobra.Command{
		Use:   "github.com/divisive-ai/vibethis/server [path]",
		Short: "A visual graph-based project manager",
		Long: `vibethis is a tool for visualizing and managing project dependencies
as an interactive graph. It provides a web interface for browsing files,
managing dependencies, and working with development containers.`,
		Version: fmt.Sprintf("%s (built %s)", Version, BuildTime),
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set path from positional argument
			if len(args) > 0 {
				cfg.RootPath = args[0]
			} else {
				cfg.RootPath = "."
			}

			// Handle --new flag
			cfg.CreateNew = createNew

			return run(cfg)
		},
	}

	// Define flags
	defaultPortInt, _ := strconv.Atoi(defaultPort)
	rootCmd.Flags().IntVarP(&cfg.Port, "port", "p", defaultPortInt, "Port to listen on")
	rootCmd.Flags().BoolVarP(&createNew, "new", "n", false, "Create a new state database if one does not exist")

	return rootCmd.Execute()
}

func run(cfg config.Config) error {
	// Validate and complete configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Initialize dependencies
	deps, cleanup, err := setup.InitializeDependencies(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize dependencies: %w", err)
	}
	defer cleanup()

	// Create and start server
	server, err := setup.CreateServer(cfg, deps)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		fmt.Printf("Server starting on :%d\n", cfg.Port)
		fmt.Printf("Scanning path: %s\n", cfg.RootPath)
		errChan <- server.Start()
	}()

	// Wait for signal or error
	select {
	case <-sigChan:
		fmt.Println("\nShutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		return server.Stop(shutdownCtx)
	case err := <-errChan:
		return err
	}
}
