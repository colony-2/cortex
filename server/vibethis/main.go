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

	"vibethis/vibethis/internal/config"
	"vibethis/vibethis/internal/setup"

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

	rootCmd := &cobra.Command{
		Use:   "vibethis",
		Short: "A visual graph-based project manager",
		Long: `vibethis is a tool for visualizing and managing project dependencies
as an interactive graph. It provides a web interface for browsing files,
managing dependencies, and working with development containers.`,
		Version: fmt.Sprintf("%s (built %s)", Version, BuildTime),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cfg)
		},
	}

	// Define flags
	defaultPortInt, _ := strconv.Atoi(defaultPort)
	rootCmd.Flags().IntVarP(&cfg.Port, "port", "p", defaultPortInt, "Port to listen on")
	rootCmd.Flags().StringVarP(&cfg.RootPath, "nodes", "n", ".", "Path to nodes directory")
	rootCmd.Flags().StringVar(&cfg.DatabasePath, "db", "", "Path to database file (default: .vibethis.db in nodes directory)")
	rootCmd.Flags().StringSliceVar(&cfg.CORSOrigins, "cors", []string{"http://localhost:3000", "http://localhost:5173"}, "Allowed CORS origins")
	rootCmd.Flags().BoolVar(&cfg.Production, "prod", false, "Run in production mode")
	rootCmd.Flags().StringVar(&cfg.StaticPath, "static", "", "Path to static assets (production mode)")

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
		if cfg.Production {
			fmt.Println("Running in production mode")
		} else {
			fmt.Println("Running in development mode")
		}
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
