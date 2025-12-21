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

	"github.com/colony-2/colony2/server/api/internal/opssetup"
	"github.com/colony-2/colony2/server/api/pkg/web"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/graph/pkg/graph"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/colony2/server/registry/pkg/registry"
	"github.com/colony-2/colony2/server/ticket/pkg/database"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
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
		Short: "Test server for colony2 e2e testing",
		Long: `A standalone test server for colony2 that can be used for e2e testing.
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
	rootCmd.Flags().StringSliceVar(&corsOrigins, "cors-origins", []string{"http://localhost:3000", "http://localhost:5173"}, "Allowed CORS origins")
	rootCmd.Flags().StringVar(&staticPath, "static", "", "Path to static files (leave empty to disable)")
	rootCmd.Flags().StringVar(&storagePath, "storage", ".colony2", "Path to storage directory (ignored if --memory is true)")

	return rootCmd.Execute()
}

func runServer(port int, corsOrigins []string, staticPath, nodesPath string, useMemory bool, storagePath string) error {
	// Resolve absolute paths
	absNodesPath, err := filepath.Abs(nodesPath)

	pgDB, closeDB, err := database.Open(database.Config{DSN: os.Getenv("NEON_C2_DEV_DSN")})
	if err != nil {
		return fmt.Errorf("failed to open ticket database: %w", err)
	}
	defer func() {
		if closeDB != nil {
			if closeErr := closeDB(); closeErr != nil {
				fmt.Printf("Warning: ticket database close failed: %v\n", closeErr)
			}
		}
	}()

	// Create dependencies
	graphBuilder := graph.NewBuilder(absNodesPath)

	// Persistence-backed services (projects, cells, tickets)
	projectStore, err := project.NewStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create project store: %w", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		return fmt.Errorf("failed to create project service: %w", err)
	}

	cellStore, err := cell.NewStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create cell store: %w", err)
	}
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	if err != nil {
		return fmt.Errorf("failed to create cell service: %w", err)
	}

	ticketStore, err := ticket.NewStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create ticket store: %w", err)
	}
	eventStore, err := ticket.NewEventStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create ticket event store: %w", err)
	}
	ticketSvc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      ticketStore,
		EventStore: eventStore,
		Projects:   projectSvc,
		Cells:      cellSvc,
	})
	if err != nil {
		return fmt.Errorf("failed to create ticket service: %w", err)
	}

	graphFactory := func(ctx context.Context, projectID string) (core.GraphBuilder, error) {
		if projectSvc == nil {
			return graphBuilder, nil
		}
		prj, err := projectSvc.GetProject(ctx, project.ID(projectID))
		if err != nil {
			return nil, err
		}
		root := prj.GitRepoPath
		if strings.TrimSpace(root) == "" {
			root = absNodesPath
		}
		return graph.NewBuilder(root), nil
	}

	// Setup ops management services (input manager etc.)
	sseManager := input.NewSimpleSSEManager()

	recipePath := filepath.Join(absNodesPath, "recipes")
	eng := toy.NewToyEngine([]swf.WorkSet{})
	reg, err := registry.NewRegistry(nil, recipePath)
	if err != nil {
		return fmt.Errorf("failed to create worker registry: %w", err)
	}
	wfc := workflow.SWFWorkflowControl{
		Engine:   eng,
		Registry: reg,
	}
	depContainer := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithWorkflowControl(&wfc).
		WithDatabase(pgDB).
		Build()

	extensionRoutes, _, err := opssetup.SetupOps(depContainer)
	if err != nil {
		return fmt.Errorf("ops setup failed: %w", err)
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
		Graph:           graphBuilder,
		GraphFactory:    graphFactory,
		Projects:        projectSvc,
		Cells:           cellSvc,
		Tickets:         ticketSvc,
		CellDeps:        cellStore,
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
