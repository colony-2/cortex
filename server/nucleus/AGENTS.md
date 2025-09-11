# Nucleus

Nucleus is a CLI tool that provides a managed way to run recipe-worker with configurable Temporal integration. It monitors recipe files and dynamically executes workflows through Temporal.

## Architecture

Nucleus consists of three core components:

- **Main CLI Application** (`cmd/nucleus/main.go`): Cobra-based command-line interface that orchestrates worker lifecycle management
- **Configuration Management** (`internal/config`): Validates and manages worker configuration including Temporal server settings, recipe paths, and worker naming
- **Temporal Client** (`internal/client`): Provides Temporal client creation and connection management for workflow execution
- **Test Utilities** (`internal/testutil`): Embedded Temporal server utilities for integration testing

The application integrates with the recipe-worker package to provide automatic recipe discovery, live reloading, and dynamic workflow execution.

## Key Interfaces

### Main Application Functions

```go
func main()
func run(cmd *cobra.Command, args []string) error
func parseConfig(cmd *cobra.Command) (*config.Config, error)
func setupLogger(debug bool) *zap.Logger
```

### Configuration Interface

```go
type Config struct {
    Name           string
    TemporalServer string
    RecipesPath    string
    Namespace      string
    Debug          bool
}

func (c *Config) Validate() error
```

### Client Interface

```go
func NewTemporalClient(cfg *config.Config) (client.Client, error)
```

### Test Utilities Interface

```go
type TestServer struct {
    server   *embeddedtemporal.Server
    client   client.Client
    dbPath   string
    hostPort string
}

func StartTestServer(t *testing.T) *TestServer
func (ts *TestServer) Client() client.Client
func (ts *TestServer) HostPort() string
func (ts *TestServer) Cleanup()
func (ts *TestServer) WaitForWorkerReady(taskQueue string, timeout time.Duration) error
```

## Usage Examples

### Basic CLI Usage

```bash
# Start worker with minimal configuration
nucleus --name my-worker --recipes-path /path/to/recipes

# Full configuration example
nucleus \
  --name production-worker \
  --recipes-path /path/to/recipes \
  --temporal-server temporal.example.com:7233 \
  --namespace production-ns \
  --debug
```

### Programmatic Configuration

```go
cfg := &config.Config{
    Name:           "test-worker",
    RecipesPath:    "/path/to/recipes",
    TemporalServer: "localhost:7233",
    Namespace:      "default",
    Debug:          false,
}

if err := cfg.Validate(); err != nil {
    return fmt.Errorf("invalid config: %w", err)
}
```

### Integration Testing

```go
func TestWithEmbeddedTemporal(t *testing.T) {
    ts := testutil.StartTestServer(t)
    client := ts.Client()
    
    // Test workflow execution
    workflowRun, err := client.ExecuteWorkflow(
        context.Background(),
        client.StartWorkflowOptions{
            TaskQueue: "test-queue",
        },
        "TestWorkflow",
    )
}
```

## Configuration

### Required Flags
- `--name`: Worker name/identifier for queue naming (alphanumeric and hyphens only)
- `--recipes-path`: Path to recipes directory (must exist and be a directory)

### Optional Flags
- `--temporal-server`: Temporal server address (default: "localhost:7233")
- `--namespace`: Temporal namespace (default: "default")
- `--debug`: Enable debug logging (default: false)

### Environment Requirements
- Go 1.24.1 or later
- Running Temporal server (or embedded for testing)
- Access to recipe YAML files

Task queues follow the pattern: `ono-recipes-{recipe-name}`