// Package main provides the entry point for the colony2 application.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/colony-2/colony2/server/cortex/internal/config"
	"github.com/colony-2/colony2/server/cortex/internal/setup"

	"github.com/spf13/cobra"
)

var (
	// Version is set at build time
	Version = "dev"

	// BuildTime is set at build time
	BuildTime = "unknown"

	// defaultPort is set at build time via ldflags
	defaultPort = "8080"

	cfg config.Config
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Execute runs the CLI application.
func Execute() error {
	rootCmd := &cobra.Command{
		Use:           "cortex",
		Short:         "Cortex - Recipe management and visualization tool",
		Version:       fmt.Sprintf("%s (built %s)", Version, BuildTime),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no subcommand is provided, run the server (default behavior)
			return run(cfg)
		},
	}

	// Define root flags (for default server behavior)
	defaultPortInt, _ := strconv.Atoi(defaultPort)
	rootCmd.PersistentFlags().IntVarP(&cfg.Port, "port", "p", defaultPortInt, "Port to listen on")
	rootCmd.PersistentFlags().StringSliceVar(&cfg.CORSOrigins, "cors-origins", []string{"http://localhost:3000", "http://localhost:5173"}, "Allowed CORS origins")
	rootCmd.PersistentFlags().StringVar(&cfg.StaticPath, "static", "embedded", "Path to static files (use 'embedded' for built-in assets)")
	rootCmd.PersistentFlags().StringVar(&cfg.StoragePath, "storage", "", "Path to storage directory (default: <root>/.colony2)")
	rootCmd.PersistentFlags().StringVar(&cfg.DatabaseDSN, "db-dsn", "", "PostgreSQL DSN for workflow engine (defaults to NEON_C2_DEV_DSN)")
	rootCmd.PersistentFlags().BoolVarP(&cfg.CreateNew, "new", "n", false, "Create a new state database if one does not exist (currently no-op)")
	rootCmd.PersistentFlags().BoolVar(&cfg.InitializeDB, "initializedb", false, "Run database migrations and workflow schema setup")
	rootCmd.PersistentFlags().StringVar(&cfg.StrataMode, "strata-mode", "embedded", "Strata mode: embedded or remote")
	rootCmd.PersistentFlags().StringVar(&cfg.StrataURL, "strata-url", "", "Remote Strata base URL (required when strata-mode=remote)")
	rootCmd.PersistentFlags().StringVar(&cfg.StrataAPIKey, "strata-api-key", "", "Strata API key (default: local)")

	// Server command (explicit subcommand)
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Start the web server for visualization",
		Long: `Start a web server that provides an interactive graph interface for browsing files,
managing dependencies, and working with development containers.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cfg)
		},
	}

	// Add subcommands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(executeCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(schemaCmd)
	rootCmd.AddCommand(strataCmd)

	return rootCmd.Execute()
}

func run(cfg config.Config) error {
	const (
		shutdownBudget = 250 * time.Millisecond
		cleanupBudget  = 250 * time.Millisecond
	)

	// Validate and complete configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Setup signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize dependencies
	deps, cleanup, err := setup.InitializeDependencies(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize dependencies: %w", err)
	}

	// Create and start server
	server, err := setup.CreateServer(cfg, deps)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create server: %w", err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: server,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		fmt.Printf("Server starting on :%d\n", cfg.Port)
		errChan <- httpServer.ListenAndServe()
	}()

	runCleanup := func(budget time.Duration) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			cleanup()
		}()
		select {
		case <-done:
		case <-time.After(budget):
		}
	}

	// Wait for signal or error
	select {
	case <-ctx.Done():
		fmt.Println("\nShutting down...")
		// We may have long-lived connections (e.g. SSE streams). Don't wait around:
		// attempt a short graceful shutdown then force-close remaining connections.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownBudget)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
			// Treat Ctrl-C shutdown timeouts/cancels as a clean exit.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				runCleanup(cleanupBudget)
				return nil
			}
			runCleanup(cleanupBudget)
			return err
		}
		runCleanup(cleanupBudget)
		return nil
	case err := <-errChan:
		cleanup()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
