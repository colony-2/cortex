package serverdeps

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
	"gorm.io/gorm"
)

type StrataMode string

const (
	StrataEmbedded StrataMode = "embedded"
	StrataRemote   StrataMode = "remote"
)

// EngineSetup encapsulates a fully configured workflow engine with PGWF and Strata.
type EngineSetup struct {
	engine   swf.SWFEngine
	strata   *daemon.Daemon
	baseURL  string
	cancelFn context.CancelFunc
}

// EngineConfig holds configuration for creating a workflow engine.
type EngineConfig struct {
	Context               context.Context
	PostgresDB            *gorm.DB
	PostgresDSN           string
	StoragePath           string
	Dependencies          ops2.ServiceDependencies2
	Logger                *slog.Logger
	MaxActive             int
	AwaitRecycleThreshold time.Duration
	StrataMode            StrataMode
	StrataURL             string
	StrataAPIKey          string
	InitializeDB          bool
}

// NewEngineSetup creates and starts a workflow engine with PGWF and Strata.
func NewEngineSetup(cfg EngineConfig) (*EngineSetup, error) {
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
	if cfg.StrataMode == "" {
		cfg.StrataMode = StrataEmbedded
	}
	if cfg.StrataMode != StrataEmbedded && cfg.StrataMode != StrataRemote {
		return nil, fmt.Errorf("invalid StrataMode %q", cfg.StrataMode)
	}
	if cfg.StrataAPIKey == "" {
		cfg.StrataAPIKey = "local"
	}
	if cfg.PostgresDB == nil {
		return nil, fmt.Errorf("PostgresDB is required")
	}
	if cfg.PostgresDSN == "" {
		return nil, fmt.Errorf("PostgresDSN is required")
	}
	if cfg.Dependencies == nil {
		return nil, fmt.Errorf("Dependencies is required")
	}
	if cfg.StrataMode == StrataEmbedded && cfg.StoragePath == "" {
		return nil, fmt.Errorf("StoragePath is required for embedded Strata")
	}
	if cfg.StrataMode == StrataRemote && cfg.StrataURL == "" {
		return nil, fmt.Errorf("StrataURL is required for remote Strata")
	}

	ctx, cancel := context.WithCancel(cfg.Context)

	sqlDB, err := cfg.PostgresDB.DB()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to get sql.DB from gorm.DB: %w", err)
	}

	if cfg.InitializeDB {
		cfg.Logger.Info("installing PGWF schema")
		if err := impl.InstallPGWF(ctx, sqlDB); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to install PGWF schema: %w", err)
		}
	} else {
		cfg.Logger.Info("skipping PGWF schema install")
	}

	strataBaseURL := cfg.StrataURL
	var strata *daemon.Daemon
	if cfg.StrataMode == StrataEmbedded {
		absStoragePath, err := filepath.Abs(cfg.StoragePath)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to resolve storage path: %w", err)
		}
		strataRowsPath := filepath.Join(absStoragePath, "strata", "rows")
		strataBlobsPath := filepath.Join(absStoragePath, "strata", "blobs")

		cfg.Logger.Info("starting Strata daemon", "rows_path", strataRowsPath, "blobs_path", strataBlobsPath)
		strataCfg := daemon.Config{
			ListenAddr:             "127.0.0.1:0",
			RowStoreURI:            fmt.Sprintf("pebble://%s", filepath.ToSlash(strataRowsPath)),
			BlobStoreURI:           fmt.Sprintf("blobfs://%s", filepath.ToSlash(strataBlobsPath)),
			MaxInlineArtifactBytes: daemon.DefaultMaxInlineArtifactBytes,
			Logger:                 cfg.Logger,
		}
		strata, err = daemon.New(strataCfg)
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
		strataBaseURL = fmt.Sprintf("http://%s", strataAddr)
		cfg.Logger.Info("Strata daemon started", "url", strataBaseURL)
	} else {
		cfg.Logger.Info("using remote Strata", "url", strataBaseURL)
	}

	cfg.Logger.Info("building workflow engine")
	engine, err := swf.NewEngineBuilder().
		WithAwaitRecycleThreshold(cfg.AwaitRecycleThreshold).
		WithPostgresDSN(cfg.PostgresDSN).
		WithStrata(strataBaseURL).
		WithStrataAPIKey(cfg.StrataAPIKey).
		WithLogger(cfg.Logger).
		WithMaxActive(cfg.MaxActive).
		Build(impl.Builder)
	if err != nil {
		cancel()
		if strata != nil {
			strata.Shutdown(context.Background())
		}
		return nil, fmt.Errorf("failed to build workflow engine: %w", err)
	}

	cfg.Logger.Info("creating activity registry")
	activityRegistry, err := ops.NewActivityRegistry()
	if err != nil {
		cancel()
		if strata != nil {
			strata.Shutdown(context.Background())
		}
		return nil, fmt.Errorf("failed to create activity registry: %w", err)
	}
	activityRegistry.SetDependencies(cfg.Dependencies)

	cfg.Logger.Info("creating recipe worker")
	workset, err := compiler.NewRecipeWorker(cfg.Dependencies, activityRegistry)
	if err != nil {
		cancel()
		if strata != nil {
			strata.Shutdown(context.Background())
		}
		return nil, fmt.Errorf("failed to create workset: %w", err)
	}

	cfg.Logger.Info("registering workers with engine")
	engine.RegisterWorkers(workset)

	cfg.Logger.Info("starting engine worker loops")
	go engine.Run(ctx)

	return &EngineSetup{
		engine:   engine,
		strata:   strata,
		baseURL:  strataBaseURL,
		cancelFn: cancel,
	}, nil
}

// Engine returns the workflow engine.
func (s *EngineSetup) Engine() swf.SWFEngine {
	return s.engine
}

// StrataBaseURL returns the base URL for Strata.
func (s *EngineSetup) StrataBaseURL() string {
	return s.baseURL
}

// Shutdown gracefully shuts down the engine and Strata daemon.
func (s *EngineSetup) Shutdown(ctx context.Context) error {
	if s.cancelFn != nil {
		s.cancelFn()
	}
	if s.strata != nil {
		if err := s.strata.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown Strata daemon: %w", err)
		}
	}
	return nil
}
