# Recipe Watcher CLI Implementation Specification

## Overview
Command-line tool that watches for recipe changes and triggers Temporal workflows using the recipe-worker code.

## Technology Stack
- **CLI Framework**: Cobra
- **Worker**: ../recipe-worker
- **Workflow Engine**: Temporal
- **Language**: Go

## Command Structure

```
recipe-watcher [flags]
```

### Flags
- `--name, -n` (string): Worker queue name prefix (required)
- `--temporal-server, -t` (string): Temporal server address (default: "docker.host.internal:7233")
- `--recipes-path, -r` (string): Path to recipes directory (required)
- `--namespace, -n` (string): Temporal namespace (default: "default")
- `--watch-interval, -i` (duration): Recipe directory watch interval (default: 5s)
- `--debug, -d` (bool): Enable debug logging (default: false)

## Implementation Steps

### 1. Project Setup

#### Structure
```
cmd/
  recipe-watcher/
    main.go
internal/
  watcher/
    watcher.go
    watcher_test.go
  worker/
    worker.go
    worker_test.go
  config/
    config.go
    config_test.go
```

#### Dependencies
```go
require (
    github.com/spf13/cobra v1.8.0
    go.temporal.io/sdk v1.26.0
    github.com/fsnotify/fsnotify v1.7.0
)
```

#### Testing Requirements
- Unit test for command initialization
- Verify all flags are registered correctly
- Test default values assignment
- Mock file system for recipe path validation

### 2. Configuration Module

#### `internal/config/config.go`
```go
type Config struct {
    QueuePrefix    string
    TemporalServer string
    RecipesPath    string
    Namespace      string
    WatchInterval  time.Duration
    Debug          bool
}

func (c *Config) Validate() error {
    // Validate recipes path exists
    // Validate temporal server format
    // Validate queue prefix format
}
```

#### Testing Requirements
- Test validation with valid/invalid recipe paths
- Test temporal server URL parsing
- Test queue prefix constraints
- Mock file system operations

### 3. Cobra Command Setup

#### `cmd/recipe-watcher/main.go`
```go
var rootCmd = &cobra.Command{
    Use:   "recipe-watcher",
    Short: "Watch recipes and trigger ono workflows",
    RunE:  run,
}

func init() {
    rootCmd.PersistentFlags().StringP("queue-prefix", "q", "recipe", "Worker queue prefix")
    // Add other flags...
    rootCmd.MarkFlagRequired("recipes-path")
}
```

#### Testing Requirements
- Test command execution with all flag combinations
- Test required flag validation
- Test help text generation
- Mock Temporal client for integration tests

### 4. Recipe Watcher Implementation

#### `internal/watcher/watcher.go`
```go
type Watcher struct {
    recipesPath string
    onChange    func(path string) error
    fsWatcher   *fsnotify.Watcher
}

func (w *Watcher) Start(ctx context.Context) error {
    // Setup fsnotify watcher
    // Watch recipes directory recursively
    // Handle file change events
}
```

#### Testing Requirements
- Test file change detection
- Test recursive directory watching
- Test error handling for inaccessible files
- Mock fsnotify for unit tests
- Test context cancellation

### 5. Worker Integration

#### `internal/worker/worker.go`
```go
type RecipeWorker struct {
    client     client.Client
    queueName  string
    activities []interface{}
    workflows  []interface{}
}

func (rw *RecipeWorker) Start(ctx context.Context) error {
    // Initialize Temporal worker
    // Register activities from ../recipe-worker
    // Register workflows
    // Start worker
}
```

#### Testing Requirements
- Test worker initialization
- Test activity/workflow registration
- Mock Temporal client
- Test graceful shutdown
- Test queue name generation with prefix

### 6. Main Execution Flow

#### `cmd/recipe-watcher/main.go` - run function
```go
func run(cmd *cobra.Command, args []string) error {
    // 1. Parse and validate config
    // 2. Initialize Temporal client
    // 3. Create and start worker
    // 4. Create and start watcher
    // 5. Handle shutdown signals
}
```

#### Testing Requirements
- Integration test with mock Temporal server
- Test graceful shutdown on signals
- Test error propagation from components
- Test concurrent operation of watcher and worker

### 7. Workflow Triggering

#### Workflow Interface
```go
type RecipeChangeWorkflow interface {
    ExecuteRecipeChange(ctx workflow.Context, recipePath string) error
}
```

#### Testing Requirements
- Test workflow execution on file changes
- Test workflow parameter passing
- Test error handling in workflows
- Mock workflow execution for unit tests

## Error Handling

### Error Types
```go
var (
    ErrInvalidConfig = errors.New("invalid configuration")
    ErrWorkerStart   = errors.New("failed to start worker")
    ErrWatcherStart  = errors.New("failed to start watcher")
)
```

### Testing Requirements
- Test all error paths
- Verify error messages are helpful
- Test recovery mechanisms
- Test logging of errors

## Logging

### Implementation
```go
// Use structured logging
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
```

### Testing Requirements
- Test log output in debug mode
- Test log levels
- Verify sensitive data isn't logged
- Mock logger for tests

## Testing Strategy

### Unit Tests
- Mock all external dependencies
- Test each component in isolation
- Achieve >80% code coverage
- Use table-driven tests

### Integration Tests
- Use testcontainers for Temporal
- Create temporary recipe directories
- Test end-to-end workflow execution
- Test concurrent operations

### Example Test
```go
func TestWatcherDetectsChanges(t *testing.T) {
    tmpDir := t.TempDir()
    // Setup watcher
    // Create/modify recipe file
    // Assert workflow triggered
}
```

## Build and Run

### Makefile targets
```makefile
build:
    go build -o bin/recipe-watcher ./cmd/recipe-watcher

test:
    go test -v -race ./...

test-integration:
    go test -v -tags=integration ./...

run:
    ./bin/recipe-watcher --recipes-path ./recipes
```

## Usage Examples

```bash
# Basic usage
recipe-watcher --recipes-path /path/to/recipes

# Custom configuration
recipe-watcher \
  --recipes-path /path/to/recipes \
  --queue-prefix production \
  --temporal-server temporal.example.com:7233 \
  --namespace production-ns \
  --debug

# With environment variables
export TEMPORAL_SERVER=temporal.example.com:7233
recipe-watcher --recipes-path ./recipes
```

## Security Considerations

- Validate all file paths to prevent directory traversal
- Sanitize recipe names before using in workflows
- Use TLS for Temporal connections in production
- Don't log sensitive configuration values

## Performance Considerations

- Debounce rapid file changes
- Batch multiple changes into single workflow
- Use worker pool for concurrent processing
- Monitor memory usage for large recipe directories
