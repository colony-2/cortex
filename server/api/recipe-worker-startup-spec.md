# Recipe Worker Startup API Specification

## Overview

Currently, starting recipe workers with a workflow engine requires significant ceremony and knowledge of internal implementation details. This spec proposes a clean, encapsulated API in the `recipe-worker` package that hides complexity and provides a simple interface for registering recipe workers with workflow engines.

## Problem Statement

### Current Approach (Testserver Example)

```go
// Register all ops globally (must happen before ActivityRegistry creation)
opssetup.RegisterOps()

// Create activity registry for task workers
activityRegistry, err := workerops.NewActivityRegistry()
if err != nil {
	return fmt.Errorf("failed to create activity registry: %w", err)
}

// Build initial ops dependencies for WorkSet creation
opsDeps := ops.NewServiceDepsBuilder().
	WithSSEManager(sseManager).
	WithDatabase(pgDB).
	Build()

// Set dependencies on activity registry
activityRegistry.SetDependencies(opsDeps)

// Create WorkSet (contains JobWorker and TaskWorkers)
workset, err := compiler.NewRecipeWorker(opsDeps, activityRegistry)
if err != nil {
	return fmt.Errorf("failed to create recipe worker: %w", err)
}

// Extract task workers from WorkSet
taskWorkers := make([]swf.TaskWorker, 0, len(workset.TaskWorkers))
for _, tw := range workset.TaskWorkers {
	taskWorkers = append(taskWorkers, tw)
}

// Register with engine
engine.PlusWorkers(workset.JobWorker, taskWorkers...)
// or: engine.RegisterWorkers(workset.JobWorker, taskWorkers...)
```

**Problems:**
1. Too many implementation details exposed to callers
2. Requires knowledge of internal types (ActivityRegistry, WorkSet)
3. Manual worker extraction and registration
4. Easy to get the order wrong (ops must be registered before ActivityRegistry)
5. Duplicated across multiple entry points (testserver, production server, tests)

### Desired Approach

```go
// Option 1: Direct registration
workerManager, err := recipeWorker.StartWorker(deps)
if err != nil {
	return err
}
if err := workerManager.RegisterWithEngine(engine); err != nil {
	return err
}

// Option 2: Prepare and register separately
workers, err := recipeWorker.PrepareWorkers(deps)
if err != nil {
	return err
}
engine.RegisterWorkers(workers.JobWorker(), workers.TaskWorkers()...)

// Option 3: All-in-one (if engine is available upfront)
if err := recipeWorker.RegisterWorkersWithEngine(engine, deps); err != nil {
	return err
}
```

## Proposed API

### Package: `github.com/colony-2/colony2/server/recipe-worker/pkg/startup`

New package that encapsulates worker initialization and registration.

### Core Types

```go
// WorkerManager encapsulates recipe worker lifecycle management
type WorkerManager struct {
	activityRegistry *workerops.ActivityRegistry
	workSet          *swf.WorkSet
	dependencies     ops.ServiceDependencies2
}

// WorkerRegistration holds workers ready for engine registration
type WorkerRegistration struct {
	jobWorker   swf.JobWorker
	taskWorkers []swf.TaskWorker
}
```

### Primary API Functions

#### Option 1: Separate Preparation and Registration

```go
// PrepareWorkers creates and initializes all recipe workers with the given dependencies.
// This function:
// - Ensures ops are registered globally
// - Creates an ActivityRegistry
// - Builds the WorkSet with JobWorker and TaskWorkers
// - Returns a WorkerManager that can register workers with an engine
//
// Dependencies should include at minimum:
// - Database connection (for ops that need persistence)
// - SSEManager (for ops that need real-time updates)
// - WorkflowControl (optional, can be nil initially and updated later)
func PrepareWorkers(deps ops.ServiceDependencies2) (*WorkerManager, error)

// RegisterWithEngine registers all workers with the given engine.
// The engine can be running or not yet started.
// Workers can be registered before or after engine.Run() is called.
func (wm *WorkerManager) RegisterWithEngine(engine swf.SWFEngine) error

// UpdateDependencies updates the dependencies for all registered workers.
// This is useful when dependencies change after initial setup (e.g., when
// WorkflowControl becomes available after engine creation).
func (wm *WorkerManager) UpdateDependencies(deps ops.ServiceDependencies2)

// WorkerCount returns the number of task workers registered
func (wm *WorkerManager) WorkerCount() int

// ActivityRegistry exposes the underlying registry for advanced use cases
func (wm *WorkerManager) ActivityRegistry() *workerops.ActivityRegistry
```

#### Option 2: All-in-One Registration

```go
// RegisterWorkersWithEngine is a convenience function that prepares workers
// and immediately registers them with the given engine.
// Equivalent to: PrepareWorkers(deps).RegisterWithEngine(engine)
func RegisterWorkersWithEngine(engine swf.SWFEngine, deps ops.ServiceDependencies2) (*WorkerManager, error)
```

#### Option 3: Just Get Workers (for custom registration)

```go
// GetWorkerRegistration prepares workers and returns them as a registration object
// that exposes JobWorker and TaskWorkers for manual registration.
// Use this if you need custom control over worker registration.
func (wm *WorkerManager) GetWorkerRegistration() *WorkerRegistration

// JobWorker returns the job worker
func (wr *WorkerRegistration) JobWorker() swf.JobWorker

// TaskWorkers returns all task workers as a slice
func (wr *WorkerRegistration) TaskWorkers() []swf.TaskWorker

// TaskWorkerMap returns task workers as a map (taskType -> worker)
func (wr *WorkerRegistration) TaskWorkerMap() map[string]swf.TaskWorker
```

### Configuration Options

```go
// Config holds optional configuration for worker preparation
type Config struct {
	// SkipOpsRegistration skips the global ops registration step.
	// Use this if ops are already registered elsewhere.
	// Default: false (will call opssetup.RegisterOps())
	SkipOpsRegistration bool

	// Logger for worker initialization messages
	Logger *slog.Logger
}

// PrepareWorkersWithConfig creates workers with custom configuration
func PrepareWorkersWithConfig(deps ops.ServiceDependencies2, config Config) (*WorkerManager, error)
```

## Usage Examples

### Example 1: Testserver (Simple Case)

```go
func runServer(port int, ...) error {
	// ... database setup ...

	// Create application context for engine lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup ops management services
	sseManager := input.NewSimpleSSEManager()

	// Validate PostgreSQL DSN
	dsn := os.Getenv("NEON_C2_DEV_DSN")
	if dsn == "" {
		return fmt.Errorf("NEON_C2_DEV_DSN environment variable must be set")
	}

	// Get underlying sql.DB for PGWF installation
	sqlDB, err := pgDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB from gorm.DB: %w", err)
	}

	// Install PGWF schema
	if err := impl.InstallPGWF(ctx, sqlDB); err != nil {
		return fmt.Errorf("failed to install PGWF schema: %w", err)
	}

	// Start embedded Strata daemon
	strata, err := impl.StartEmbeddedStrata()
	if err != nil {
		return fmt.Errorf("failed to start embedded Strata daemon: %w", err)
	}
	defer strata.Shutdown()

	// Prepare recipe workers (handles ops registration, activity registry, workset)
	opsDeps := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithDatabase(pgDB).
		Build()

	workerManager, err := recipeWorker.PrepareWorkers(opsDeps)
	if err != nil {
		return fmt.Errorf("failed to prepare workers: %w", err)
	}

	// Build real workflow engine
	engineID := ksuid.New().String()
	engine, err := swf.NewEngineBuilder(engineID).
		WithAwaitRecycleThreshold(5 * time.Second).
		WithPostgresDSN(dsn).
		WithStrata(strata.BaseURL).
		WithStrataAPIKey(strata.APIKey).
		WithLogger(slog.Default()).
		WithMaxActive(100).
		Build(impl.Builder)
	if err != nil {
		return fmt.Errorf("failed to build workflow engine: %w", err)
	}

	// Register workers with engine
	if err := workerManager.RegisterWithEngine(engine); err != nil {
		return fmt.Errorf("failed to register workers: %w", err)
	}
	fmt.Printf("Registered %d task workers and 1 job worker\n", workerManager.WorkerCount())

	// Start engine worker loops
	go engine.Run(ctx)

	// Create recipe registry
	recipePath := filepath.Join(absNodesPath, "recipes")
	reg, err := registry.NewRegistry(nil, recipePath)
	if err != nil {
		return fmt.Errorf("failed to create worker registry: %w", err)
	}

	// Create workflow control wrapper
	wfc := workflow.SWFWorkflowControl{
		Engine:   engine,
		Registry: reg,
	}

	// Update dependencies with workflow control
	finalDeps := ops.NewServiceDepsBuilder().
		WithSSEManager(sseManager).
		WithWorkflowControl(&wfc).
		WithDatabase(pgDB).
		Build()
	workerManager.UpdateDependencies(finalDeps)

	// Setup ops routes
	extensionRoutes, _, err := opssetup.SetupOps(finalDeps)
	if err != nil {
		return fmt.Errorf("ops setup failed: %w", err)
	}

	// ... rest of server setup ...
}
```

### Example 2: Test Setup (All-in-One)

```go
func setupTestEngine(t *testing.T, db *gorm.DB) swf.SWFEngine {
	ctx := context.Background()

	// Install PGWF
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, impl.InstallPGWF(ctx, sqlDB))

	// Start Strata
	strata, err := impl.StartEmbeddedStrata()
	require.NoError(t, err)
	t.Cleanup(strata.Shutdown)

	// Build engine
	engine, err := swf.NewEngineBuilder(ksuid.New().String()).
		WithPostgresDSN(getDSN()).
		WithStrata(strata.BaseURL).
		WithStrataAPIKey(strata.APIKey).
		Build(impl.Builder)
	require.NoError(t, err)

	// Register workers with engine (all-in-one)
	deps := ops.NewServiceDepsBuilder().WithDatabase(db).Build()
	_, err = recipeWorker.RegisterWorkersWithEngine(engine, deps)
	require.NoError(t, err)

	// Start engine
	go engine.Run(ctx)

	return engine
}
```

### Example 3: Custom Worker Registration

```go
// Advanced case: need to inspect or modify workers before registration
workerManager, err := recipeWorker.PrepareWorkers(deps)
if err != nil {
	return err
}

registration := workerManager.GetWorkerRegistration()

// Inspect workers
fmt.Printf("Job worker: %s\n", registration.JobWorker().Name())
for taskType, worker := range registration.TaskWorkerMap() {
	fmt.Printf("Task worker: %s -> %s\n", taskType, worker.Name())
}

// Custom registration logic
engine.RegisterWorker(registration.JobWorker())
for _, taskWorker := range registration.TaskWorkers() {
	engine.RegisterWorker(taskWorker)
}
```

## Implementation Notes

### Internal Implementation (in `startup` package)

```go
func PrepareWorkers(deps ops.ServiceDependencies2) (*WorkerManager, error) {
	return PrepareWorkersWithConfig(deps, Config{})
}

func PrepareWorkersWithConfig(deps ops.ServiceDependencies2, config Config) (*WorkerManager, error) {
	// Register ops globally if not skipped
	if !config.SkipOpsRegistration {
		opssetup.RegisterOps()
	}

	// Create activity registry
	activityRegistry, err := workerops.NewActivityRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to create activity registry: %w", err)
	}

	// Set dependencies
	activityRegistry.SetDependencies(deps)

	// Create WorkSet
	workset, err := compiler.NewRecipeWorker(deps, activityRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to create recipe worker: %w", err)
	}

	if config.Logger != nil {
		config.Logger.Info("prepared recipe workers",
			"task_worker_count", len(workset.TaskWorkers),
			"job_worker", workset.JobWorker.Name())
	}

	return &WorkerManager{
		activityRegistry: activityRegistry,
		workSet:          workset,
		dependencies:     deps,
	}, nil
}

func (wm *WorkerManager) RegisterWithEngine(engine swf.SWFEngine) error {
	// Extract task workers
	taskWorkers := make([]swf.TaskWorker, 0, len(wm.workSet.TaskWorkers))
	for _, tw := range wm.workSet.TaskWorkers {
		taskWorkers = append(taskWorkers, tw)
	}

	// Register with engine
	// Note: Assumes engine has RegisterWorkers method or similar
	// If using builder pattern, this would need to be called before Build()
	return engine.RegisterWorkers(wm.workSet.JobWorker, taskWorkers...)
}

func (wm *WorkerManager) UpdateDependencies(deps ops.ServiceDependencies2) {
	wm.dependencies = deps
	wm.activityRegistry.SetDependencies(deps)
}

func (wm *WorkerManager) WorkerCount() int {
	return len(wm.workSet.TaskWorkers)
}

func (wm *WorkerManager) ActivityRegistry() *workerops.ActivityRegistry {
	return wm.activityRegistry
}

func (wm *WorkerManager) GetWorkerRegistration() *WorkerRegistration {
	taskWorkers := make([]swf.TaskWorker, 0, len(wm.workSet.TaskWorkers))
	taskWorkerMap := make(map[string]swf.TaskWorker)

	for taskType, tw := range wm.workSet.TaskWorkers {
		taskWorkers = append(taskWorkers, tw)
		taskWorkerMap[taskType] = tw
	}

	return &WorkerRegistration{
		jobWorker:      wm.workSet.JobWorker,
		taskWorkers:    taskWorkers,
		taskWorkerMap:  taskWorkerMap,
	}
}

func RegisterWorkersWithEngine(engine swf.SWFEngine, deps ops.ServiceDependencies2) (*WorkerManager, error) {
	wm, err := PrepareWorkers(deps)
	if err != nil {
		return nil, err
	}
	if err := wm.RegisterWithEngine(engine); err != nil {
		return nil, err
	}
	return wm, nil
}
```

### Engine Builder Integration

Since workers can be added after engine creation, the builder pattern should support:

```go
// During build (original approach)
engine, err := swf.NewEngineBuilder(id).
	PlusWorkers(jobWorker, taskWorkers...).
	Build(impl.Builder)

// After build (new approach)
engine, err := swf.NewEngineBuilder(id).Build(impl.Builder)
// ... later ...
engine.RegisterWorkers(jobWorker, taskWorkers...)
```

This spec assumes the engine supports post-build worker registration via a method like `RegisterWorkers()`.

## Migration Path

### Phase 1: Introduce New API
- Add `server/recipe-worker/pkg/startup` package
- Implement `PrepareWorkers`, `RegisterWithEngine`, etc.
- Keep existing APIs unchanged

### Phase 2: Update Testserver
- Refactor testserver to use new API
- Validate behavior matches existing implementation

### Phase 3: Update Tests
- Refactor integration tests (e.g., ticket_autostart_test.go)
- Use new simplified API

### Phase 4: Update Production
- Update production server entry points
- Remove old direct usage of ActivityRegistry/WorkSet if no longer needed

### Phase 5: Documentation
- Update docs to recommend new API
- Mark old patterns as "advanced/internal use only"

## Benefits

1. **Encapsulation**: Implementation details hidden from callers
2. **Simplicity**: One function call instead of 7+ steps
3. **Correctness**: Hard to get initialization order wrong
4. **Testability**: Easier to set up test environments
5. **Maintainability**: Changes to worker initialization logic only affect one package
6. **Flexibility**: Multiple API styles (all-in-one, prepare-then-register, manual) support different use cases

## Open Questions

1. **Engine API**: Does `swf.SWFEngine` support `RegisterWorkers()` after construction? If not, should we propose that extension to swf-go?

2. **Dependency Updates**: Is `UpdateDependencies()` the right pattern, or should WorkerManager be immutable and require recreation?

3. **Package Naming**: Should this be `startup`, `manager`, `init`, or something else?

4. **Error Handling**: Should `RegisterWithEngine()` be idempotent (safe to call multiple times)?

5. **Lifecycle Management**: Should WorkerManager have a `Shutdown()` method for cleanup?
