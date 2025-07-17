# Recipe Watcher CLI Implementation Specification

## Overview
Command-line tool that provides a managed way to run the existing recipe-worker package with configurable options. The recipe-worker already handles recipe discovery, monitoring, and dynamic workflow execution - this CLI wraps it with proper configuration and lifecycle management.

## Technology Stack
- **CLI Framework**: Cobra
- **Worker Package**: ../recipe-worker (existing implementation)
- **Workflow Engine**: Temporal
- **Language**: Go

## Command Structure

```
recipe-watcher [flags]
```

### Flags
- `--name` (string): Worker name/identifier for queue naming (required)
- `--temporal-server, -t` (string): Temporal server address (default: "localhost:7233")
- `--recipes-path, -r` (string): Path to recipes directory (required)
- `--namespace` (string): Temporal namespace (default: "default")
- `--debug, -d` (bool): Enable debug logging (default: false)

## Implementation Steps

### 1. Project Setup

#### Structure
```
cmd/
  recipe-watcher/
    main.go
internal/
  config/
    config.go
    config_test.go
  client/
    client.go
    client_test.go
```

#### Dependencies
```go
require (
    github.com/spf13/cobra v1.8.0
    go.temporal.io/sdk v1.26.0
    go.uber.org/zap v1.26.0
    // Import the recipe-worker package
    github.com/vibethis/recipe-worker v0.0.0
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
    Name           string
    TemporalServer string
    RecipesPath    string
    Namespace      string
    Debug          bool
}

func (c *Config) Validate() error {
    // Validate recipes path exists
    // Validate temporal server format
    // Validate name format (no special chars)
}
```

#### Testing Requirements
- Test validation with valid/invalid recipe paths
- Test temporal server URL parsing
- Test name constraints (alphanumeric, dashes)
- Mock file system operations

### 3. Cobra Command Setup

#### `cmd/recipe-watcher/main.go`
```go
var rootCmd = &cobra.Command{
    Use:   "recipe-watcher",
    Short: "Run recipe-worker with Temporal integration",
    RunE:  run,
}

func init() {
    rootCmd.PersistentFlags().String("name", "", "Worker name/identifier")
    rootCmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "Temporal server")
    rootCmd.PersistentFlags().StringP("recipes-path", "r", "", "Path to recipes")
    rootCmd.PersistentFlags().String("namespace", "default", "Temporal namespace")
    rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging")
    
    rootCmd.MarkFlagRequired("name")
    rootCmd.MarkFlagRequired("recipes-path")
}
```

#### Testing Requirements
- Test command execution with all flag combinations
- Test required flag validation
- Test help text generation
- Mock logger setup based on debug flag

### 4. Temporal Client Setup

#### `internal/client/client.go`
```go
func NewTemporalClient(config *Config) (client.Client, error) {
    // Create Temporal client options
    options := client.Options{
        HostPort:  config.TemporalServer,
        Namespace: config.Namespace,
    }
    
    // Create and return client
    return client.Dial(options)
}
```

#### Testing Requirements
- Test client creation with various configs
- Test connection error handling
- Mock client for unit tests
- Test namespace configuration

### 5. Integration with recipe-worker

#### `cmd/recipe-watcher/main.go` - run function
```go
import (
    recipeworker "github.com/vibethis/recipe-worker/pkg/worker"
)

func run(cmd *cobra.Command, args []string) error {
    // 1. Parse and validate config
    config, err := parseConfig(cmd)
    if err != nil {
        return err
    }
    
    // 2. Setup logger
    logger := setupLogger(config.Debug)
    
    // 3. Initialize Temporal client
    temporalClient, err := client.NewTemporalClient(config)
    if err != nil {
        return fmt.Errorf("failed to create Temporal client: %w", err)
    }
    defer temporalClient.Close()
    
    // 4. Create recipe worker using existing implementation
    worker, err := recipeworker.NewWorker(
        logger,
        config.RecipesPath,
        temporalClient,
    )
    if err != nil {
        return fmt.Errorf("failed to create worker: %w", err)
    }
    
    // 5. Start the worker (handles its own file watching)
    if err := worker.Start(); err != nil {
        return fmt.Errorf("failed to start worker: %w", err)
    }
    
    // 6. Handle shutdown signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    <-sigChan
    logger.Info("Shutdown signal received")
    
    // 7. Graceful shutdown
    return worker.Stop()
}
```

#### Testing Requirements
- Integration test with mock recipe-worker
- Test signal handling and graceful shutdown
- Test error propagation from worker
- Verify worker lifecycle management

### 6. Logger Setup

#### Logger Configuration
```go
func setupLogger(debug bool) *zap.Logger {
    var logger *zap.Logger
    var err error
    
    if debug {
        logger, err = zap.NewDevelopment()
    } else {
        logger, err = zap.NewProduction()
    }
    
    if err != nil {
        panic(fmt.Sprintf("failed to initialize logger: %v", err))
    }
    
    return logger
}
```

#### Testing Requirements
- Test debug vs production logger setup
- Verify log output format
- Test logger panic on initialization failure

## How recipe-worker Integration Works

The recipe-worker package provides:
1. **Registry**: Automatically discovers and monitors recipe YAML files
2. **WorkerManager**: Creates and manages Temporal workers for each recipe
3. **DynamicWorkflow**: Executes workflows based on YAML definitions
4. **Executor**: Handles activity execution (HTTP, gRPC, scripts)
5. **Live Reloading**: Automatically updates workers when recipes change

Our CLI provides:
1. Configuration management via command-line flags
2. Temporal client setup and lifecycle
3. Graceful shutdown handling
4. Logging configuration

Task queues created by recipe-worker follow the pattern: `ono-recipes-{recipe-name}`

## Error Handling

### Error Types
```go
var (
    ErrInvalidConfig = errors.New("invalid configuration")
    ErrClientCreate  = errors.New("failed to create Temporal client")
    ErrWorkerCreate  = errors.New("failed to create worker")
    ErrWorkerStart   = errors.New("failed to start worker")
)
```

### Testing Requirements
- Test all error paths
- Verify error messages are helpful
- Test graceful degradation
- Ensure proper cleanup on errors

## Testing Strategy

### Unit Tests
- Mock recipe-worker package
- Mock Temporal client
- Test configuration validation
- Test signal handling

### Integration Tests
```go
func TestEndToEnd(t *testing.T) {
    // Use testcontainers for Temporal
    // Create temporary recipe directory
    // Start CLI with test config
    // Verify worker starts and processes recipes
    // Test graceful shutdown
}
```

### Example Unit Test
```go
func TestConfigValidation(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name: "valid config",
            config: Config{
                Name:           "test-worker",
                RecipesPath:    "/tmp/recipes",
                TemporalServer: "localhost:7233",
            },
            wantErr: false,
        },
        {
            name: "missing name",
            config: Config{
                RecipesPath: "/tmp/recipes",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("got error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Build and Run

### Makefile targets
```makefile
.PHONY: build test test-integration run

build:
	go build -o bin/recipe-watcher ./cmd/recipe-watcher

test:
	go test -v -race ./...

test-integration:
	go test -v -tags=integration ./...

run:
	./bin/recipe-watcher --name local --recipes-path ./recipes

clean:
	rm -rf bin/
```

## Usage Examples

```bash
# Basic usage
recipe-watcher --name my-worker --recipes-path /path/to/recipes

# Custom configuration
recipe-watcher \
  --name production-worker \
  --recipes-path /path/to/recipes \
  --temporal-server temporal.example.com:7233 \
  --namespace production-ns \
  --debug

# With environment variables
export TEMPORAL_SERVER=temporal.example.com:7233
export TEMPORAL_NAMESPACE=production
recipe-watcher --name worker-1 --recipes-path ./recipes
```

## Security Considerations

- Validate all file paths to prevent directory traversal
- Sanitize worker name to prevent injection
- Use TLS for Temporal connections in production
- Don't log sensitive configuration values
- Ensure recipe files are from trusted sources

## Performance Considerations

- The recipe-worker handles file watching efficiently with fsnotify
- Each recipe runs in its own task queue for isolation
- Worker lifecycle is managed per-recipe by recipe-worker
- Memory usage scales with number of active recipes

## Deployment Notes

1. Ensure recipe directory is accessible and has proper permissions
2. Configure Temporal connection with appropriate credentials
3. Use systemd or similar for production deployments
4. Monitor worker health via Temporal UI
5. Set up log rotation for production environments