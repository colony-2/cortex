// Package main provides the test server for e2e testing.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	"github.com/divisive-ai/vibethis/server/container/pkg/container"
	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	"github.com/divisive-ai/vibethis/server/ops/pkg/input"
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time
	Version = "dev"

	// BuildTime is set at build time
	BuildTime = "unknown"
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Execute runs the test server CLI.
func Execute() error {
	var (
		port        int
		corsOrigins []string
		staticPath  string
		nodesPath   string
		createNew   bool
		storagePath string
	)

	rootCmd := &cobra.Command{
		Use:   "testserver [path]",
		Short: "Test server for vibethis e2e testing",
		Long: `A standalone test server for vibethis that can be used for e2e testing.
Supports memory storage and configurable node directories.`,
		Version: fmt.Sprintf("%s (built %s)", Version, BuildTime),
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set path from positional argument
			if len(args) > 0 {
				nodesPath = args[0]
			} else {
				nodesPath = "."
			}
			
			// For testserver, -n always means use memory storage
			useMemory := createNew || true
			
			return runServer(port, corsOrigins, staticPath, nodesPath, useMemory, storagePath)
		},
	}

	rootCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	rootCmd.Flags().BoolVarP(&createNew, "new", "n", false, "Create a new state database if one does not exist (always uses memory storage)")
	rootCmd.Flags().StringSliceVar(&corsOrigins, "cors-origins", []string{"http://localhost:3000"}, "Allowed CORS origins")
	rootCmd.Flags().StringVar(&staticPath, "static", "", "Path to static files (leave empty to disable)")
	rootCmd.Flags().StringVar(&storagePath, "storage", ".vibethis", "Path to storage directory (ignored if --memory is true)")

	return rootCmd.Execute()
}

func runServer(port int, corsOrigins []string, staticPath, nodesPath string, useMemory bool, storagePath string) error {
	// Resolve absolute paths
	absNodesPath, err := filepath.Abs(nodesPath)
	if err != nil {
		return fmt.Errorf("failed to resolve nodes path: %w", err)
	}

	// Create storage
	var store core.Storage
	if useMemory {
		fmt.Println("Using in-memory storage")
		store = storage.NewMemoryStorage()
	} else {
		absStoragePath, err := filepath.Abs(storagePath)
		if err != nil {
			return fmt.Errorf("failed to resolve storage path: %w", err)
		}
		fmt.Printf("Using file storage at: %s\n", absStoragePath)
		store, err = storage.NewBoltStorage(storage.Config{
			DatabasePath: filepath.Join(absStoragePath, "vibethis.db"),
			ReadOnly:     false,
		})
		if err != nil {
			return fmt.Errorf("failed to create file storage: %w", err)
		}
	}

	// Create dependencies
	graphBuilder := graph.NewBuilder(absNodesPath)
	fileBrowser := files.NewBrowser(files.Config{})
	gitRepo := git.NewRepository(git.Config{
		DefaultAuthor: "Test User",
		DefaultEmail:  "test@example.com",
	})
	containerManager := container.NewManager(container.Config{})

	// Create input management service
	inputService := input.NewInputManagementService()
	inputService.Initialize(input.ServiceDependencies{})

	// Convert input service routes to extension routes
	var extensionRoutes []web.ExtensionRoute
	for _, route := range inputService.GetRoutes() {
		path := route.Path
		if strings.HasPrefix(path, "/api") {
			path = strings.TrimPrefix(path, "/api")
		}
		extensionRoutes = append(extensionRoutes, web.ExtensionRoute{
			Method:  route.Method,
			Path:    path,
			Handler: route.Handler,
		})
	}

	// Create server configuration
	config := web.Config{
		Port:            port,
		CORSOrigins:     corsOrigins,
		StaticPath:      staticPath,
		EnableWebSocket: false,
		MaxUploadSize:   10 * 1024 * 1024, // 10MB
	}

	// Create dependencies
	deps := web.Dependencies{
		Storage:         store,
		Graph:           graphBuilder,
		Files:           fileBrowser,
		Git:             gitRepo,
		Container:       containerManager,
		ExtensionRoutes: extensionRoutes,
	}

	// If static path is set, configure static file serving
	if staticPath != "" && staticPath != "embedded" {
		absStaticPath, err := filepath.Abs(staticPath)
		if err != nil {
			return fmt.Errorf("failed to resolve static path: %w", err)
		}
		config.StaticPath = absStaticPath
		fmt.Printf("Serving static files from: %s\n", absStaticPath)
	}

	// Create and start server
	server := web.NewServer(config, deps)

	fmt.Printf("Starting test server on port %d\n", port)
	fmt.Printf("Nodes directory: %s\n", absNodesPath)
	fmt.Printf("CORS origins: %v\n", corsOrigins)

	// Setup graceful shutdown
	errChan := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case <-sigChan:
		fmt.Println("\nShutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
		fmt.Println("Server stopped")
	}

	return nil
}