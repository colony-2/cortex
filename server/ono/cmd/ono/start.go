package ono

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	port          int
	uiPort        int
	namespace     string
	dbPath        string
	logLevel      string
	ip            string
	sqlitePragmas map[string]string
)

var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a Temporal development server",
	RunE:  runDevServer,
}

func init() {
	StartCmd.Flags().IntVarP(&port, "port", "p", 7233, "Port for the Temporal frontend service")
	StartCmd.Flags().IntVar(&uiPort, "ui-port", 8233, "Port for the Temporal Web UI")
	StartCmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Default namespace to create")
	StartCmd.Flags().StringVar(&dbPath, "db-filename", "./ono.db", "Path to persistent SQLite database file")
	StartCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	StartCmd.Flags().StringVar(&ip, "ip", "127.0.0.1", "IP address to bind to")
	StartCmd.Flags().StringToStringVar(&sqlitePragmas, "sqlite-pragma", map[string]string{}, "SQLite pragma statements in key=value format")
}

func runDevServer(cmd *cobra.Command, args []string) error {
	// Create dev server options
	opts := DevServerOptions{
		FrontendIP:    ip,
		FrontendPort:  port,
		UIPort:        uiPort,
		Namespaces:    []string{namespace},
		DatabaseFile:  dbPath,
		LogLevel:      logLevel,
		SQLitePragmas: sqlitePragmas,
		EnableUI:      true,
	}

	// Ensure absolute path for database file
	if opts.DatabaseFile != "" {
		absPath, err := filepath.Abs(opts.DatabaseFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		opts.DatabaseFile = absPath
	}

	fmt.Printf("Starting Temporal development server...\n")
	fmt.Printf("Frontend address: %s:%d\n", opts.FrontendIP, opts.FrontendPort)
	fmt.Printf("UI address: http://%s:%d\n", opts.FrontendIP, opts.UIPort)
	fmt.Printf("Namespace: %s\n", namespace)
	fmt.Printf("Database: %s\n", opts.DatabaseFile)

	// Create and start dev server
	devServer, err := NewDevServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create dev server: %w", err)
	}

	if err := devServer.Start(); err != nil {
		return fmt.Errorf("failed to start dev server: %w", err)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-interrupt:
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		if err := devServer.Stop(); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
	case <-ctx.Done():
		if err := devServer.Stop(); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
	}

	return nil
}