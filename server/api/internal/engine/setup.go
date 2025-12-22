package engine

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	ops2 "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/strata-go/pkg/daemon"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/impl"
	"github.com/segmentio/ksuid"
	"gorm.io/gorm"
)

// Setup encapsulates a fully configured real workflow engine with PGWF and Strata.
type Setup struct {
	engine   swf.SWFEngine
	strata   *daemon.Daemon
	cancelFn context.CancelFunc
}

// Config holds configuration for creating a real workflow engine.
type Config struct {
	// Context is the parent context for the engine lifecycle.
	// If nil, a background context will be used.
	Context context.Context

	// PostgresDB is the GORM database connection for PGWF schema installation.
	PostgresDB *gorm.DB

	// PostgresDSN is the connection string for the workflow engine's persistence layer.
	PostgresDSN string

	// StoragePath is the directory where Strata will store artifacts persistently.
	// Row store and blob store will be created under {StoragePath}/strata/
	StoragePath string

	// Dependencies provides service dependencies for creating the WorkSet.
	Dependencies ops2.ServiceDependencies2

	// Logger is the structured logger for the engine.
	// If nil, slog.Default() will be used.
	Logger *slog.Logger

	// MaxActive is the maximum number of concurrent workflow executions.
	// If 0, defaults to 100.
	MaxActive int

	// AwaitRecycleThreshold is how long to wait before recycling workers.
	// If 0, defaults to 5 seconds.
	AwaitRecycleThreshold time.Duration
}

// NewSetup creates and starts a real workflow engine with PGWF and persistent Strata.
//
// This function:
//   - Installs PGWF schema to PostgreSQL
//   - Starts an embedded Strata daemon with persistent storage
//   - Builds a real swf.impl workflow engine
//   - Registers all workers from the ActivityRegistry
//   - Starts the engine in a background goroutine
//
// Call Shutdown() when done to clean up resources.
func NewSetup(cfg Config) (*Setup, error) {
	// Set defaults
	if cfg.Context == nil {
		cfg.Context = context.Background()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxActive == 0 {
		cfg.MaxActive = 100
	}
	if cfg.AwaitRecycleThreshold == 0 {
		cfg.AwaitRecycleThreshold = 5 * time.Second
	}

	// Validate required fields
	if cfg.PostgresDB == nil {
		return nil, fmt.Errorf("PostgresDB is required")
	}
	if cfg.PostgresDSN == "" {
		return nil, fmt.Errorf("PostgresDSN is required")
	}
	if cfg.StoragePath == "" {
		return nil, fmt.Errorf("StoragePath is required")
	}
	if cfg.Dependencies == nil {
		return nil, fmt.Errorf("Dependencies is required")
	}

	// Create cancellable context for engine lifecycle
	ctx, cancel := context.WithCancel(cfg.Context)

	// Install PGWF schema
	// Note: We use the existing database connection which should already be connected
	// to the correct database. If PGWF installation fails due to extension permissions,
	// ensure the database has CREATE EXTENSION privileges or connect to the postgres database.
	sqlDB, err := cfg.PostgresDB.DB()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to get sql.DB from gorm.DB: %w", err)
	}

	if err := impl.InstallPGWF(ctx, sqlDB); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to install PGWF schema: %w (ensure database has CREATE EXTENSION privileges)", err)
	}

	// Setup persistent Strata storage paths
	absStoragePath, err := filepath.Abs(cfg.StoragePath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to resolve storage path: %w", err)
	}
	strataRowsPath := filepath.Join(absStoragePath, "strata", "rows")
	strataBlobsPath := filepath.Join(absStoragePath, "strata", "blobs")

	// Start embedded Strata daemon with persistent storage
	strataCfg := daemon.Config{
		ListenAddr:             "127.0.0.1:0",
		RowStoreURI:            fmt.Sprintf("pebble://%s", filepath.ToSlash(strataRowsPath)),
		BlobStoreURI:           fmt.Sprintf("blobfs://%s", filepath.ToSlash(strataBlobsPath)),
		MaxInlineArtifactBytes: daemon.DefaultMaxInlineArtifactBytes,
	}
	strata, err := daemon.New(strataCfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create Strata daemon: %w", err)
	}
	if err := strata.Start(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start Strata daemon: %w", err)
	}

	strataAddr, err := strata.Addr()
	if err != nil {
		cancel()
		strata.Shutdown(context.Background())
		return nil, fmt.Errorf("failed to get Strata address: %w", err)
	}
	strataBaseURL := fmt.Sprintf("http://%s", strataAddr)

	// Build real workflow engine
	engineID := ksuid.New().String()
	engine, err := swf.NewEngineBuilder(engineID).
		WithAwaitRecycleThreshold(cfg.AwaitRecycleThreshold).
		WithPostgresDSN(cfg.PostgresDSN).
		WithStrata(strataBaseURL).
		WithStrataAPIKey("local").
		WithLogger(cfg.Logger).
		WithMaxActive(cfg.MaxActive).
		Build(impl.Builder)
	if err != nil {
		cancel()
		strata.Shutdown(context.Background())
		return nil, fmt.Errorf("failed to build workflow engine: %w", err)
	}

	// Create activity registry and workset
	activityRegistry, err := ops.NewActivityRegistry()
	if err != nil {
		cancel()
		strata.Shutdown(context.Background())
		return nil, fmt.Errorf("failed to create activity registry: %w", err)
	}
	activityRegistry.SetDependencies(cfg.Dependencies)

	workset, err := compiler.NewRecipeWorker(cfg.Dependencies, activityRegistry)
	if err != nil {
		cancel()
		strata.Shutdown(context.Background())
		return nil, fmt.Errorf("failed to create workset: %w", err)
	}

	// Register workers with the engine
	engine.RegisterWorkers(workset)

	// Start engine worker loops
	go engine.Run(ctx)

	return &Setup{
		engine:   engine,
		strata:   strata,
		cancelFn: cancel,
	}, nil
}

// Engine returns the workflow engine.
func (s *Setup) Engine() swf.SWFEngine {
	return s.engine
}

// Shutdown gracefully shuts down the engine and Strata daemon.
// It cancels the engine context, waits briefly for work to complete,
// then shuts down the Strata daemon.
func (s *Setup) Shutdown(ctx context.Context) error {
	// Cancel engine context (stops accepting new work)
	if s.cancelFn != nil {
		s.cancelFn()
	}

	// Brief sleep to allow current work to complete
	time.Sleep(100 * time.Millisecond)

	// Shutdown Strata daemon
	if s.strata != nil {
		if err := s.strata.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown Strata daemon: %w", err)
		}
	}

	return nil
}
