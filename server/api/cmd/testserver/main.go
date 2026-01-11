// Package main provides the test server for e2e testing.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/colony-2/colony2/server/api/internal/engine"
	"github.com/colony-2/colony2/server/api/internal/opssetup"
	"github.com/colony-2/colony2/server/api/internal/recipes"
	"github.com/colony-2/colony2/server/api/pkg/web"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	gitpkg "github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/graph/pkg/graph"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"github.com/colony-2/colony2/server/ticket/pkg/database"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	workflowsvc "github.com/colony-2/colony2/server/workflow/pkg/workflow"
	strataclient "github.com/colony-2/strata-go/pkg/client"
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
		createNew   bool
		storagePath string
	)

	rootCmd := &cobra.Command{
		Use:     "testserver",
		Short:   "Test server for colony2 e2e testing",
		Long:    `A standalone test server for colony2 that can be used for e2e testing.`,
		Version: fmt.Sprintf("%s (built %s)", Version, BuildTime),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// For testserver, -n always means use memory storage
			useMemory := createNew || true

			return runServer(port, corsOrigins, staticPath, useMemory, storagePath)
		},
	}

	rootCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	rootCmd.Flags().BoolVarP(&createNew, "new", "n", false, "Create a new state database if one does not exist (always uses memory storage)")
	rootCmd.Flags().StringSliceVar(&corsOrigins, "cors-origins", []string{"http://localhost:3000", "http://localhost:5173"}, "Allowed CORS origins")
	rootCmd.Flags().StringVar(&staticPath, "static", "", "Path to static files (leave empty to disable)")
	rootCmd.Flags().StringVar(&storagePath, "storage", ".colony2", "Path to storage directory (ignored if --memory is true)")

	return rootCmd.Execute()
}

func runServer(port int, corsOrigins []string, staticPath string, useMemory bool, storagePath string) error {
	startTime := time.Now()
	setupLogger()
	slog.Info("testserver startup initiated", "port", port)

	slog.Info("opening database connection")
	dsn := os.Getenv("NEON_C2_DEV_DSN")
	pgDB, closeDB, err := database.Open(database.Config{DSN: dsn})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	slog.Info("database connection established", "elapsed", time.Since(startTime))
	defer func() {
		if closeDB != nil {
			if closeErr := closeDB(); closeErr != nil {
				fmt.Printf("Warning: database close failed: %v\n", closeErr)
			}
		}
	}()

	// Persistence-backed services (projects, cells, tickets)
	slog.Info("creating project store")
	projectStore, err := project.NewStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create project store: %w", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		return fmt.Errorf("failed to create project service: %w", err)
	}
	slog.Info("project service created", "elapsed", time.Since(startTime))

	slog.Info("creating cell service")
	cellStore, err := cell.NewStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create cell store: %w", err)
	}
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	if err != nil {
		return fmt.Errorf("failed to create cell service: %w", err)
	}
	slog.Info("cell service created", "elapsed", time.Since(startTime))

	// Initialize recipe service
	slog.Info("creating recipe service")
	recipeSvc, err := recipesvc.NewServiceFromDB(pgDB, recipesvc.ServiceConfig{
		GitRepo:  gitpkg.NewRepository(gitpkg.Config{}),
		Projects: projectSvc,
		IDGen:    recipesvc.NewKSUIDGenerator(),
		Clock:    recipesvc.NewSystemClock(),
	})
	if err != nil {
		return fmt.Errorf("failed to create recipe service: %w", err)
	}
	slog.Info("recipe service created", "elapsed", time.Since(startTime))

	slog.Info("creating ticket stores")
	ticketStore, err := ticket.NewStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create ticket store: %w", err)
	}
	eventStore, err := ticket.NewEventStore(pgDB)
	if err != nil {
		return fmt.Errorf("failed to create ticket event store: %w", err)
	}
	slog.Info("ticket stores created", "elapsed", time.Since(startTime))

	// Setup ops management services (input manager etc.)
	slog.Info("creating SSE manager")
	sseManager := input.NewSimpleSSEManager()
	slog.Info("SSE manager created", "elapsed", time.Since(startTime))

	// Validate PostgreSQL DSN is set
	if dsn == "" {
		return fmt.Errorf("NEON_C2_DEV_DSN must be set for real workflow engine")
	}

	// Register all ops globally (required before engine setup)
	slog.Info("registering ops")
	opssetup.RegisterOps()
	slog.Info("ops registered", "elapsed", time.Since(startTime))

	// Create initial dependencies for engine setup
	slog.Info("creating initial dependencies for engine")
	tempDeps := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithDatabase(pgDB).
		Build()

	// Create real workflow engine with PGWF and Strata
	slog.Info("setting up workflow engine (PGWF + Strata)")
	engineSetup, err := engine.NewSetup(engine.Config{
		PostgresDB:   pgDB,
		PostgresDSN:  dsn,
		StoragePath:  storagePath,
		Dependencies: tempDeps,
	})
	if err != nil {
		return fmt.Errorf("failed to setup workflow engine: %w", err)
	}
	slog.Info("workflow engine setup complete", "elapsed", time.Since(startTime))
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := engineSetup.Shutdown(shutdownCtx); err != nil {
			fmt.Printf("Warning: engine shutdown failed: %v\n", err)
		}
	}()

	// Create embedded provider for internal recipes
	slog.Info("creating embedded recipe provider")
	embeddedProvider, err := recipes.NewEmbeddedProvider()
	if err != nil {
		return fmt.Errorf("failed to create embedded recipe provider: %w", err)
	}
	slog.Info("embedded recipe provider created", "elapsed", time.Since(startTime))

	// Create RecipeProjectProvider with fallback to embedded recipes
	recipeProviderWithFallback := recipes.NewRecipeProjectProviderWithFallback(recipeSvc, embeddedProvider)

	// Create ticket service with engine and recipe provider for workflow autostart
	slog.Info("creating ticket service")
	ticketSvc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      ticketStore,
		EventStore: eventStore,
		Projects:   projectSvc,
		Cells:      cellSvc,
		Engine:     engineSetup.Engine(),
		Recipes:    recipeProviderWithFallback,
	})
	if err != nil {
		return fmt.Errorf("failed to create ticket service: %w", err)
	}
	slog.Info("ticket service created", "elapsed", time.Since(startTime))

	slog.Info("creating graph factory")
	graphFactory := func(ctx context.Context, projectID string) (core.GraphBuilder, error) {
		if projectSvc == nil {
			return nil, fmt.Errorf("project service not configured")
		}
		prj, err := projectSvc.GetProject(ctx, project.ID(projectID))
		if err != nil {
			return nil, err
		}
		root := prj.GitRepoPath
		if strings.TrimSpace(root) == "" {
			return nil, fmt.Errorf("project %s has no git repository path configured", projectID)
		}
		return graph.NewBuilder(root), nil
	}

	slog.Info("creating workflow control")
	wfc := workflow.SWFWorkflowControl{
		Engine:   engineSetup.Engine(),
		Registry: recipeProviderWithFallback,
	}

	slog.Info("creating strata client")
	strataClient, err := strataclient.New(strataclient.Config{
		BaseURL: engineSetup.StrataBaseURL(),
		APIKey:  "local",
		Logger:  slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("failed to create strata client: %w", err)
	}
	slog.Info("strata client created", "elapsed", time.Since(startTime))

	slog.Info("creating workflow service")
	workflowSvc, err := workflowsvc.New(workflowsvc.ServiceConfig{
		Engine:   engineSetup.Engine(),
		Strata:   strataClient,
		Tickets:  ticketSvc,
		Cells:    cellSvc,
		Projects: projectSvc,
	})
	if err != nil {
		return fmt.Errorf("failed to create workflow service: %w", err)
	}
	slog.Info("workflow service created", "elapsed", time.Since(startTime))

	slog.Info("building ops dependency container")
	depContainer := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithWorkflowControl(&wfc).
		WithDatabase(pgDB).
		Build()

	slog.Info("setting up ops (extension routes)")
	extensionRoutes, _, err := opssetup.SetupOps(depContainer)
	if err != nil {
		return fmt.Errorf("ops setup failed: %w", err)
	}
	slog.Info("ops setup complete", "elapsed", time.Since(startTime))

	// Create server configuration
	slog.Info("creating web server configuration")
	config := web.Config{
		Port:            port,
		CORSOrigins:     corsOrigins,
		StaticPath:      staticPath,
		EnableWebSocket: false,
		MaxUploadSize:   10 * 1024 * 1024, // 10MB
	}

	// Create dependencies
	deps := web.Dependencies{
		GraphFactory:    graphFactory,
		Projects:        projectSvc,
		Cells:           cellSvc,
		Tickets:         ticketSvc,
		Workflows:       workflowSvc,
		RecipeSvc:       recipeSvc,
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
		slog.Info("serving static files", "path", absStaticPath)
	}

	// Create and start server
	slog.Info("creating web server")
	server := web.NewServer(config, deps)
	slog.Info("web server created", "elapsed", time.Since(startTime))

	slog.Info("starting test server", "port", port, "cors_origins", corsOrigins)

	// Setup graceful shutdown
	errChan := make(chan error, 1)
	readyChan := make(chan struct{})
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		} else {
			close(readyChan)
		}
	}()

	// Wait for server to be ready or error
	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case <-time.After(100 * time.Millisecond):
		// Server likely started successfully
		slog.Info("testserver ready", "total_startup_time", time.Since(startTime))
	}

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
