package setup

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	serverdeps "github.com/colony-2/colony2/server/api/pkg/serverdeps"
	serverdepsops "github.com/colony-2/colony2/server/api/pkg/serverdeps/opssetup"
	"github.com/colony-2/colony2/server/api/pkg/web"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/cortex/internal/config"
	"github.com/colony-2/colony2/server/cortex/internal/static"
	gitpkg "github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/graph/pkg/graph"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"github.com/colony-2/colony2/server/ticket/pkg/database"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	workflowsvc "github.com/colony-2/colony2/server/workflow/pkg/workflow"
	strataclient "github.com/colony-2/strata-go/pkg/client"
)

// InitializeDependencies initializes all application dependencies
func InitializeDependencies(ctx context.Context, cfg config.Config) (web.Dependencies, func(), error) {
	logger := slog.Default()

	dsn := strings.TrimSpace(cfg.DatabaseDSN)
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("NEON_C2_DEV_DSN"))
	}
	if dsn == "" {
		return web.Dependencies{}, nil, fmt.Errorf("NEON_C2_DEV_DSN or --db-dsn must be set for workflow engine")
	}

	pgDB, closeDB, err := database.Open(database.Config{DSN: dsn})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to open database: %w", err)
	}

	cleanupFns := []func(){}
	if closeDB != nil {
		cleanupFns = append(cleanupFns, func() {
			if err := closeDB(); err != nil {
				logger.Warn("database close failed", "error", err)
			}
		})
	}

	projectStore, err := project.NewStoreWithOptions(pgDB, project.StoreOptions{Migrate: cfg.InitializeDB})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create project store: %w", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create project service: %w", err)
	}

	cellStore, err := cell.NewStoreWithOptions(pgDB, cell.StoreOptions{Migrate: cfg.InitializeDB})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create cell store: %w", err)
	}
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create cell service: %w", err)
	}

	recipeStore, err := recipesvc.NewStoreWithOptions(pgDB, recipesvc.StoreOptions{Migrate: cfg.InitializeDB})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create recipe store: %w", err)
	}
	recipeSvc, err := recipesvc.NewService(recipesvc.ServiceConfig{
		Store:        recipeStore,
		GitRepo:      gitpkg.NewRepository(gitpkg.Config{}),
		Projects:     projectSvc,
		IDGen:        recipesvc.NewKSUIDGenerator(),
		Clock:        recipesvc.NewSystemClock(),
		CELValidator: recipesvc.NewRecipeWorkerCELValidator(ops.NewServiceDepsBuilder().Build()),
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create recipe service: %w", err)
	}

	ticketStore, err := ticket.NewStoreWithOptions(pgDB, ticket.StoreOptions{Migrate: cfg.InitializeDB})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create ticket store: %w", err)
	}
	eventStore, err := ticket.NewEventStoreWithOptions(pgDB, ticket.EventStoreOptions{Migrate: cfg.InitializeDB})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create ticket event store: %w", err)
	}

	sseManager := input.NewSimpleSSEManager()

	serverdepsops.RegisterOps()

	tempDeps := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithDatabase(pgDB).
		Build()

	engineSetup, err := serverdeps.NewEngineSetup(serverdeps.EngineConfig{
		Context:      ctx,
		PostgresDB:   pgDB,
		PostgresDSN:  dsn,
		StoragePath:  cfg.StoragePath,
		Dependencies: tempDeps,
		Logger:       logger,
		StrataMode:   serverdeps.StrataMode(cfg.StrataMode),
		StrataURL:    cfg.StrataURL,
		StrataAPIKey: cfg.StrataAPIKey,
		InitializeDB: cfg.InitializeDB,
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to setup workflow engine: %w", err)
	}
	cleanupFns = append(cleanupFns, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := engineSetup.Shutdown(shutdownCtx); err != nil {
			logger.Warn("engine shutdown failed", "error", err)
		}
	})

	embeddedProvider, err := serverdeps.NewEmbeddedProvider()
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create embedded recipe provider: %w", err)
	}
	recipeProviderWithFallback := serverdeps.NewRecipeProjectProviderWithFallback(recipeSvc, embeddedProvider)
	ticketRecipeProvider := ticket.RecipeProjectProvider(func(projectID string, recipeRef string) (*recipecore.Recipe, error) {
		return recipeProviderWithFallback(projectID, recipeRef)
	})

	ticketSvc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      ticketStore,
		EventStore: eventStore,
		Projects:   projectSvc,
		Cells:      cellSvc,
		Engine:     engineSetup.Engine(),
		Recipes:    ticketRecipeProvider,
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create ticket service: %w", err)
	}

	graphFactory := func(ctx context.Context, projectID string) (core.GraphBuilder, error) {
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

	workflowRecipeProvider := workflow.RecipeProjectProvider(func(projectID string, recipeRef string) (*recipecore.Recipe, error) {
		return recipeProviderWithFallback(projectID, recipeRef)
	})

	wfc := workflow.SWFWorkflowControl{
		Engine:   engineSetup.Engine(),
		Registry: workflowRecipeProvider,
	}

	strataAPIKey := cfg.StrataAPIKey
	if strings.TrimSpace(strataAPIKey) == "" {
		strataAPIKey = "local"
	}
	strataClient, err := strataclient.New(strataclient.Config{
		BaseURL: engineSetup.StrataBaseURL(),
		APIKey:  strataAPIKey,
		Logger:  logger,
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create strata client: %w", err)
	}

	workflowSvc, err := workflowsvc.New(workflowsvc.ServiceConfig{
		Engine:   engineSetup.Engine(),
		Strata:   strataClient,
		Tickets:  ticketSvc,
		Cells:    cellSvc,
		Projects: projectSvc,
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to create workflow service: %w", err)
	}

	depContainer := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithWorkflowControl(&wfc).
		WithDatabase(pgDB).
		Build()

	extensionRoutes, cleanupOps, err := serverdepsops.SetupOps(depContainer)
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("ops setup failed: %w", err)
	}
	if len(cleanupOps) > 0 {
		cleanupFns = append(cleanupFns, func() {
			for _, fn := range cleanupOps {
				fn()
			}
		})
	}

	var staticFS http.FileSystem
	if cfg.StaticPath == "" || cfg.StaticPath == "embedded" {
		fs, err := static.GetFileSystem()
		if err != nil {
			return web.Dependencies{}, nil, fmt.Errorf("failed to load embedded static assets: %w", err)
		}
		staticFS = fs
	}

	deps := web.Dependencies{
		StaticFS:        staticFS,
		GraphFactory:    graphFactory,
		Projects:        projectSvc,
		Cells:           cellSvc,
		Tickets:         ticketSvc,
		Workflows:       workflowSvc,
		RecipeSvc:       recipeSvc,
		CellDeps:        cellStore,
		ExtensionRoutes: extensionRoutes,
	}

	cleanup := func() {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
	}

	return deps, cleanup, nil
}

// CreateServer creates the HTTP server
func CreateServer(cfg config.Config, deps web.Dependencies) (*web.Server, error) {
	staticPath := cfg.StaticPath
	if staticPath != "" && staticPath != "embedded" {
		absStaticPath, err := filepath.Abs(staticPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve static path: %w", err)
		}
		staticPath = absStaticPath
	}

	webConfig := web.Config{
		Port:            cfg.Port,
		CORSOrigins:     cfg.CORSOrigins,
		StaticPath:      staticPath,
		EnableWebSocket: false,
		MaxUploadSize:   10 * 1024 * 1024,
	}

	server := web.NewServer(webConfig, deps)
	return server, nil
}
