# Logging Implementation Plan

## Overview

This document outlines the plan to introduce structured logging using `log/slog` across all main Go projects in the Colony2 server codebase. The implementation will follow the pattern established in the workflow service (documented in `workflow/LOGGING.md`) and the testserver.

## Goals

1. **Per-component loggers**: Each service/component has its own `*slog.Logger` field
2. **Consistent defaults**: All loggers default to `slog.Default()` if not provided
3. **Independent configuration**: Each logger can be set to different levels/handlers
4. **Structured logging**: Use key-value pairs for machine-readable, searchable logs
5. **Zero breaking changes**: Backward compatible with existing code

## Current State

### Already Implemented
- **workflow service** (`server/workflow`): Full slog implementation with comprehensive documentation
  - Logger field in Service struct
  - Configurable via ServiceConfig
  - Extensive logging of SWF engine and Strata client operations
  - Documented patterns and best practices in LOGGING.md

- **testserver** (`server/api/cmd/testserver`): Global logger setup
  - `setupLogger()` function configures JSON handler with Debug level
  - Custom error handler that expands errors with stacktraces
  - AddSource: true for file:line information
  - Uses `slog.Info` throughout startup process

### Not Yet Implemented
The following services need logging added following the workflow pattern:
- cell
- project
- ticket
- recipes
- git
- graph
- core
- cortex
- llm
- recipe-core
- recipe-worker
- recipe-input
- ops
- storage/astore/rstore

## Design Pattern

### Service Structure

Each service should follow this pattern (based on workflow service):

```go
// Config struct
type ServiceConfig struct {
    // ... existing fields ...
    Logger *slog.Logger  // New field, optional
}

// Service struct
type service struct {
    // ... existing fields ...
    logger *slog.Logger  // New field, private
}

// Constructor
func New(cfg ServiceConfig) (Service, error) {
    // ... existing validation ...

    // Logger setup with default fallback
    logger := cfg.Logger
    if logger == nil {
        logger = slog.Default()
    }

    return &service{
        // ... existing fields ...
        logger: logger,
    }, nil
}
```

### Logging Levels

Use three primary levels consistently:
- **Debug**: Detailed operational information for troubleshooting
- **Info**: Significant events (service startup, major operations)
- **Warn**: Warning conditions that don't prevent operation
- **Error**: Error conditions that caused operation failure

### Key Naming Conventions

Establish consistent key names across all services:

**Entity IDs:**
- `project_id`: Project identifier
- `cell_id`: Cell identifier
- `ticket_id`: Ticket identifier
- `recipe_name`: Recipe name
- `recipe_version`: Recipe version/ref
- `workflow_id`: Workflow/job identifier
- `tenant_id`: Tenant ID
- `job_id`: Job ID

**Operations:**
- `operation`: Name of the operation (e.g., "CreateProject", "UpdateCell")
- `error`: Error details
- `duration_ms`: Operation duration in milliseconds

**Contextual:**
- `count`: Number of items (e.g., "cell_count", "ticket_count")
- `status`: Status value
- `state`: State value

### Logging Pattern

Follow this pattern for all operations:

```go
// 1. Log intent before operation
s.logger.Debug("CreateProject: creating new project",
    "name", input.Name,
    "git_repo", input.GitRepoPath)

// 2. Perform operation
project, err := s.store.Create(ctx, input)

// 3a. Log error if failed
if err != nil {
    s.logger.Error("CreateProject: failed to create project",
        "name", input.Name,
        "error", err)
    return nil, err
}

// 3b. Log success with results
s.logger.Debug("CreateProject: project created successfully",
    "project_id", project.ID,
    "name", project.Name)
```

## Implementation Plan

### Phase 1: Core Services (High Priority)

These services are fundamental and used by many others:

#### 1.1 Project Service (`server/project`)
- **Files to modify:**
  - `internal/service/service.go`: Add logger field to ServiceConfig and service struct
  - All service methods: Add logging for Create, Update, Delete, Get, List operations
- **Key operations to log:**
  - CreateProject, UpdateProject, DeleteProject
  - GetProject, ListProjects
  - Database errors, validation failures
- **Estimated changes:** ~15-20 log statements

#### 1.2 Cell Service (`server/cell`)
- **Files to modify:**
  - `internal/service/service.go`: Add logger field
  - All CRUD methods
  - `SyncFromPopulator` method (complex operation deserves detailed logging)
- **Key operations to log:**
  - CreateCell, UpdateCell, MarkDeleted
  - GetCell, ListCells
  - ReplaceDependencies
  - SyncFromPopulator (start, progress, completion)
- **Estimated changes:** ~20-25 log statements

#### 1.3 Ticket Service (`server/ticket`)
- **Files to modify:**
  - `internal/service/service.go`: Add logger field
  - Ticket CRUD operations
  - Event appending operations
  - Workflow integration points
- **Key operations to log:**
  - CreateTicket, UpdateTicket, SearchTickets
  - AppendWorkflowEvent, AppendMarkdownEvent, AppendChangeSetEvent
  - Workflow autostart operations
  - ResetTicket operations
- **Estimated changes:** ~25-30 log statements

#### 1.4 Recipe Service (`server/recipes`)
- **Files to modify:**
  - `internal/service/service.go`: Add logger field
  - Recipe lifecycle methods
  - Git synchronization operations
- **Key operations to log:**
  - CreateRecipe, UpdateRecipe, DeleteRecipe
  - PublishRecipe, UnpublishRecipe
  - GetRecipe, ListRecipes
  - SyncFromRemote (git operations)
  - ValidateRecipe
- **Estimated changes:** ~25-30 log statements

### Phase 2: Recipe Execution & Workflow (Medium Priority)

#### 2.1 Recipe-Worker (`server/recipe-worker`)
- **Components:**
  - Workflow control (SWFWorkflowControl)
  - Executor implementation
  - Op execution
- **Key operations to log:**
  - Workflow start/stop
  - Op execution (start, success, failure)
  - Input/output handling
  - Error conditions
- **Estimated changes:** ~30-40 log statements

#### 2.2 Recipe-Core (`server/recipe-core`)
- **Components:**
  - Op registry
  - Service dependencies
  - Context management
- **Key operations to log:**
  - Op registration
  - Dependency injection
  - Context creation
- **Estimated changes:** ~15-20 log statements

#### 2.3 Recipe-Input (`server/recipe-input`)
- **Components:**
  - SSE manager
  - Input resolution
- **Key operations to log:**
  - SSE connections
  - Input requests/responses
  - Default value resolution
- **Estimated changes:** ~20-25 log statements

### Phase 3: Support Services (Lower Priority)

#### 3.1 Git Service (`server/git`)
- **Key operations to log:**
  - Clone, fetch, pull operations
  - Commit, branch operations
  - Error conditions (network, auth, conflicts)
- **Estimated changes:** ~20-25 log statements

#### 3.2 Graph Service (`server/graph`)
- **Key operations to log:**
  - Graph building
  - Dependency analysis
  - Cache operations
- **Estimated changes:** ~15-20 log statements

#### 3.3 Core (`server/core`)
- **Key operations to log:**
  - Core interface implementations
  - Graph operations
- **Estimated changes:** ~10-15 log statements

#### 3.4 LLM Service (`server/llm`)
- **Key operations to log:**
  - API calls (with sanitized prompts)
  - Response parsing
  - Rate limiting/retries
  - Errors and timeouts
- **Estimated changes:** ~20-25 log statements
- **Note:** Avoid logging full prompts/responses (may contain PII)

#### 3.5 Cortex (`server/cortex`)
- **Key operations to log:**
  - Agent operations
  - State management
- **Estimated changes:** ~15-20 log statements

### Phase 4: Storage & Infrastructure

#### 4.1 Storage Services (`server/storage`, `server/astore`, `server/rstore`)
- **Key operations to log:**
  - Storage operations
  - Cache hits/misses
  - Errors
- **Estimated changes:** ~10-15 log statements per service

#### 4.2 Ops (`server/ops`)
- **Key operations to log:**
  - Op execution
  - Registration
  - Errors
- **Estimated changes:** ~15-20 log statements

### Phase 5: Integration & Testing

#### 5.1 Update testserver
- **Modify:** `api/cmd/testserver/main.go`
- **Changes:**
  - Pass configured logger to each service during initialization
  - Option to configure different log levels per service via CLI flags
  - Example:
    ```go
    // Create loggers for each service (can be configured independently)
    projectLogger := createServiceLogger("project", slog.LevelInfo)
    cellLogger := createServiceLogger("cell", slog.LevelDebug)
    ticketLogger := createServiceLogger("ticket", slog.LevelDebug)

    // Pass to services
    projectSvc, err := project.NewService(project.ServiceConfig{
        Store: projectStore,
        Logger: projectLogger,
    })
    ```

#### 5.2 Environment-based Configuration
- **Create:** `server/pkg/logging` package (optional shared utilities)
- **Features:**
  - Environment variable parsing (LOG_LEVEL, LOG_FORMAT)
  - Standard logger factory functions
  - Common error handler (like testserver's handleErr)
  - Example:
    ```go
    package logging

    func NewLogger(component string, opts ...Option) *slog.Logger {
        // Default options
        cfg := &config{
            level:  slog.LevelInfo,
            format: "json",
        }

        // Apply options
        for _, opt := range opts {
            opt(cfg)
        }

        // Create handler with component name in all logs
        var handler slog.Handler
        handlerOpts := &slog.HandlerOptions{
            Level:       cfg.level,
            AddSource:   true,
            ReplaceAttr: handleErr,
        }

        if cfg.format == "json" {
            handler = slog.NewJSONHandler(os.Stdout, handlerOpts)
        } else {
            handler = slog.NewTextHandler(os.Stdout, handlerOpts)
        }

        // Wrap with component attribute
        logger := slog.New(handler).With("component", component)
        return logger
    }
    ```

#### 5.3 Documentation
- **Create/Update:**
  - `server/LOGGING.md`: Overall logging strategy and conventions
  - Update individual service READMEs with logging examples
  - Migration guide for existing services

## Advanced Features (Future)

These can be added after basic logging is in place:

1. **Contextual Loggers**: Create child loggers with pre-populated context
   ```go
   // Create a logger for a specific operation with pre-filled context
   opLogger := s.logger.With(
       "project_id", projectID,
       "operation", "cell_sync",
   )
   // All subsequent logs include these fields automatically
   opLogger.Debug("starting sync")
   ```

2. **Metrics Integration**: Add duration logging
   ```go
   start := time.Now()
   // ... operation ...
   s.logger.Info("operation completed",
       "operation", "create_project",
       "duration_ms", time.Since(start).Milliseconds())
   ```

3. **Trace ID Propagation**: Extract and log trace IDs from context
   ```go
   if traceID := ctx.Value("trace_id"); traceID != nil {
       logger = logger.With("trace_id", traceID)
   }
   ```

4. **Log Sampling**: Sample high-volume debug logs in production
   ```go
   // Only log 1% of debug messages
   if rand.Float64() < 0.01 {
       logger.Debug("high volume operation", ...)
   }
   ```

5. **Structured Error Types**: Define error types that carry context
   ```go
   type ServiceError struct {
       Operation string
       ProjectID string
       Cause     error
   }
   ```

## Migration Strategy

### Incremental Rollout
1. Implement one service at a time (no big-bang changes)
2. Each service can be merged independently
3. Services without logger field still work (they use slog.Default())
4. No breaking changes to public APIs

### Testing Strategy
1. Add logging to tests to verify behavior
2. Use test logger with custom handler to verify log output
3. Example:
   ```go
   func TestCreateProject_Logging(t *testing.T) {
       var logBuf bytes.Buffer
       testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

       svc := NewService(ServiceConfig{
           Store: mockStore,
           Logger: testLogger,
       })

       // Perform operation
       _, err := svc.CreateProject(ctx, input)

       // Verify logs
       logs := logBuf.String()
       assert.Contains(t, logs, "CreateProject")
       assert.Contains(t, logs, "project_id")
   }
   ```

### Backward Compatibility
- Logger field is always optional in Config structs
- Default to `slog.Default()` if not provided
- Existing code continues to work without changes
- Can gradually add logger configuration

## Configuration Examples

### Development: Verbose logging
```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
    AddSource: true,
}))
```

### Production: JSON with Info level
```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
```

### Per-Service Configuration
```go
// High-traffic service: only errors
projectLogger := logging.NewLogger("project", logging.WithLevel(slog.LevelError))

// Service under investigation: verbose
cellLogger := logging.NewLogger("cell", logging.WithLevel(slog.LevelDebug))

// Normal service: info
ticketLogger := logging.NewLogger("ticket", logging.WithLevel(slog.LevelInfo))
```

## Success Criteria

1. ✅ All services accept optional `Logger *slog.Logger` in their Config
2. ✅ All services default to `slog.Default()` if logger not provided
3. ✅ Key operations (CRUD, complex flows) are logged with appropriate levels
4. ✅ Consistent key naming across all services
5. ✅ No breaking changes to existing APIs
6. ✅ Documentation updated (at least one example per service)
7. ✅ Testserver demonstrates per-service logger configuration
8. ✅ At least one test per service verifies logging behavior

## Timeline Estimate

Based on the scope:
- **Phase 1** (Core Services): 4 services × ~1 day = ~4 days
- **Phase 2** (Recipe Execution): 3 services × ~1.5 days = ~4.5 days
- **Phase 3** (Support Services): 5 services × ~1 day = ~5 days
- **Phase 4** (Storage): 4 services × ~0.5 days = ~2 days
- **Phase 5** (Integration & Testing): ~2 days
- **Documentation & Review**: ~1-2 days

**Total estimate**: ~18-20 days for complete rollout

However, since this is incremental:
- **Minimum viable**: Phase 1 only (~4 days)
- **High value**: Phases 1-2 (~8-9 days)
- **Complete**: All phases (~18-20 days)

## Open Questions

1. **Should we create a shared `server/pkg/logging` package?**
   - Pro: Reduces duplication, provides common utilities
   - Con: Adds dependency, might be overkill for simple logger passing
   - Recommendation: Start without, add if pattern duplication becomes painful

2. **Should we configure per-service log levels via environment variables?**
   - Pro: Easy production debugging without redeployment
   - Con: More complex configuration management
   - Recommendation: Add in Phase 5 if valuable

3. **Should we log to different outputs per service?**
   - Pro: Service isolation, easier log routing
   - Con: Complexity, harder to correlate logs
   - Recommendation: Not initially, all services to stdout/stderr

4. **Should we add request IDs/trace IDs?**
   - Pro: Critical for distributed tracing
   - Con: Requires context propagation infrastructure
   - Recommendation: Add as "Advanced Feature" after basic logging works

## References

- Existing implementation: `server/workflow/LOGGING.md`
- Testserver example: `server/api/cmd/testserver/main.go` and `err.go`
- Go slog documentation: https://pkg.go.dev/log/slog
