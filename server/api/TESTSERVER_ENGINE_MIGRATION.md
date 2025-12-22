# Replace ToyEngine with Real swf.impl Engine in Testserver

## Summary

Replace the in-memory ToyEngine with the real swf.impl workflow engine in the testserver. This requires:
- Creating a reusable `EngineSetup` abstraction for engine initialization
- Installing PGWF schema to PostgreSQL
- Starting an embedded Strata daemon with persistent storage
- Using the engine builder pattern with proper configuration
- Registering workers after engine creation using `RegisterWorkers()`
- Managing engine lifecycle with context and goroutines

## Critical Files to Modify

- `/src/server/recipe-worker/pkg/executor/engine.go` (NEW - reusable engine setup)
- `/src/server/api/cmd/testserver/main.go` (use the new EngineSetup)

## Implementation Steps

### Part A: Create Reusable Engine Setup (New File)

Create `/src/server/recipe-worker/pkg/executor/engine.go` with a reusable `EngineSetup` struct:

```go
package executor

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

// EngineSetup encapsulates a fully configured real workflow engine with PGWF and Strata.
type EngineSetup struct {
	engine   swf.SWFEngine
	strata   *daemon.Daemon
	cancelFn context.CancelFunc
}

// EngineConfig holds configuration for creating a real workflow engine.
type EngineConfig struct {
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

// NewEngineSetup creates and starts a real workflow engine with PGWF and persistent Strata.
//
// This function:
//   - Installs PGWF schema to PostgreSQL
//   - Starts an embedded Strata daemon with persistent storage
//   - Builds a real swf.impl workflow engine
//   - Registers all workers from the ActivityRegistry
//   - Starts the engine in a background goroutine
//
// Call Shutdown() when done to clean up resources.
func NewEngineSetup(cfg EngineConfig) (*EngineSetup, error) {
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
	sqlDB, err := cfg.PostgresDB.DB()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to get sql.DB from gorm.DB: %w", err)
	}

	if err := impl.InstallPGWF(ctx, sqlDB); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to install PGWF schema: %w", err)
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

	return &EngineSetup{
		engine:   engine,
		strata:   strata,
		cancelFn: cancel,
	}, nil
}

// Engine returns the workflow engine.
func (s *EngineSetup) Engine() swf.SWFEngine {
	return s.engine
}

// Shutdown gracefully shuts down the engine and Strata daemon.
// It cancels the engine context, waits briefly for work to complete,
// then shuts down the Strata daemon.
func (s *EngineSetup) Shutdown(ctx context.Context) error {
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
```

This provides a clean, reusable abstraction that can be used in testserver, production servers, and other tools.

### Part B: Update Testserver to Use EngineSetup

### 1. Add Required Imports

Add to `/src/server/api/cmd/testserver/main.go`:

```go
"github.com/colony-2/colony2/server/recipe-worker/pkg/executor"
```

Remove:
```go
"github.com/colony-2/swf-go/pkg/swf/toy"
```

### 2. Replace Engine Setup (Replace lines 160-182)

Replace the ToyEngine initialization with the new EngineSetup:

```go
// Setup ops management services (input manager etc.)
sseManager := input.NewSimpleSSEManager()

// Validate DSN
dsn := os.Getenv("NEON_C2_DEV_DSN")
if dsn == "" {
	return fmt.Errorf("NEON_C2_DEV_DSN must be set")
}

// Register all ops globally (required before engine setup)
opssetup.RegisterOps()

// Create initial dependencies for engine setup
tempDeps := ops.NewServiceDepsBuilder().
	WithSSEManager(sseManager).
	WithDatabase(pgDB).
	Build()

// Create real workflow engine with PGWF and Strata
engineSetup, err := executor.NewEngineSetup(executor.EngineConfig{
	PostgresDB:  pgDB,
	PostgresDSN: dsn,
	StoragePath: storagePath,
	Dependencies: tempDeps,
})
if err != nil {
	return fmt.Errorf("failed to setup workflow engine: %w", err)
}
defer func() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := engineSetup.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("Warning: engine shutdown failed: %v\n", err)
	}
}()

// Continue with existing flow
recipePath := filepath.Join(absNodesPath, "recipes")
reg, err := registry.NewRegistry(nil, recipePath)
if err != nil {
	return fmt.Errorf("failed to create worker registry: %w", err)
}

wfc := workflow.SWFWorkflowControl{
	Engine:   engineSetup.Engine(),
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
```

### 3. Update Shutdown Sequence (Replace lines 236-243)

Simplify the shutdown since EngineSetup handles cleanup via defer:

```go
case <-sigChan:
	fmt.Println("\nShutting down server...")

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	// Engine and Strata shutdown happens via defer

	fmt.Println("Server stopped")
```

## Key Design Decisions

### Reusable EngineSetup Abstraction

The `EngineSetup` struct in `/src/server/recipe-worker/pkg/executor/engine.go` provides a clean, reusable abstraction for real engine initialization. This:
- Encapsulates all setup complexity (PGWF, Strata, worker registration, engine lifecycle)
- Can be reused in testserver, production servers, CLI tools, and tests
- Provides clear lifecycle management with `Shutdown()`
- Handles error cleanup properly in all failure paths
- Makes downstream code much simpler

### Persistent Strata Storage

Uses the `storagePath` CLI parameter (defaults to `.colony2`) to store Strata artifacts persistently:
- **Row Store** (metadata): `{storagePath}/strata/rows` using Pebble LSM
- **Blob Store** (artifacts): `{storagePath}/strata/blobs` using filesystem

This allows workflow artifacts to persist across testserver restarts, enabling reruns and debugging.

### Worker Registration Inside EngineSetup

The `EngineSetup` handles all worker registration internally:
- Ops must be registered globally *before* calling `NewEngineSetup()`
- `EngineSetup` creates `ActivityRegistry` and `WorkSet` internally
- `RegisterWorkers()` is called before `Engine()` is available
- Engine is already running when `NewEngineSetup()` returns

This means testserver just needs to call `opssetup.RegisterOps()` before creating the engine.

### Context Lifecycle

`EngineSetup` manages its own internal context:
- Created from the `Context` field in `EngineConfig` (or background if nil)
- Cancelled during `Shutdown()`
- Brief grace period for work to complete before Strata shutdown

### Graceful Shutdown Order

With `EngineSetup`, shutdown is simple:
1. Stop HTTP server (testserver's responsibility)
2. Call `engineSetup.Shutdown()` (or rely on defer)
   - Cancels engine context (stops accepting new work)
   - 100ms grace period for current work
   - Shuts down Strata daemon
3. Database close (existing defer in testserver)

## Reference Implementation

Based on:
- `/src/server/api/internal/handlers/ticket_autostart_test.go` (lines 60-106) - Engine setup with PGWF/Strata
- `/strata-go/pkg/daemon/` - Persistent Strata daemon configuration
- Existing testserver CLI flow with minimal disruption

## Testing Verification

After implementation, verify:
1. Engine starts successfully (check logs for creation message with engine ID)
2. PGWF tables exist in PostgreSQL
3. Strata daemon is accessible at logged BaseURL
4. Strata storage directories created: `.colony2/strata/rows` and `.colony2/strata/blobs`
5. Workers are registered (check log shows expected count)
6. Recipes can be started via API
7. Artifacts persist across testserver restarts
8. Graceful shutdown works with SIGTERM/SIGINT
9. Clear error if `NEON_C2_DEV_DSN` is missing
