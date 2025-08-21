# VIBETHIS

## Overview
Self-contained embedded Temporal server for Go applications using SQLite persistence. Eliminates need for separate Temporal service in development, testing, and simple deployment scenarios.

## Architecture

### Core Components
- **Server**: Main embedded Temporal server (`pkg/temporal/server.go`)
- **Client Factory**: Helper functions for creating configured clients (`pkg/temporal/client.go`)  
- **Utilities**: Port management and network utilities (`pkg/temporal/utils.go`)

### Service Structure
- **Frontend Service**: gRPC API endpoint (configurable port)
- **History Service**: Workflow execution state management (dynamic port)
- **Matching Service**: Task queue management (dynamic port)
- **Worker Service**: Internal system workflows (dynamic port)
- **SQLite Backend**: Single-file persistence with WAL mode

## Key Interfaces

### Server Management
```go
// Create embedded server
func NewServer(opts Options) (*Server, error)

// Start server (blocking)
func (s *Server) Start() error

// Stop server gracefully
func (s *Server) Stop() error

// Get frontend address for clients
func (s *Server) GetFrontendAddress() string
```

### Configuration Options
```go
type Options struct {
    FrontendIP    string            // Default: 127.0.0.1
    FrontendPort  int               // Frontend service port
    DatabaseFile  string            // SQLite database file path
    LogLevel      string            // debug, info, error
    Namespaces    []string          // Auto-create namespaces
    SQLitePragmas map[string]string // Custom SQLite settings
    EnableUI      bool              // Enable web UI
}
```

### Client Creation
```go
// Create workflow client
func NewClient(opts ClientOptions) (client.Client, error)

// Create namespace management client
func NewNamespaceClient(hostPort string) (client.NamespaceClient, error)
```

### Utility Functions
```go
// Check port availability
func IsPortAvailable(host string, port int) bool

// Find available port
func FindFreePort() int
```

## Usage Examples

### Basic Server Setup
```go
server, err := temporal.NewServer(temporal.Options{
    FrontendPort: 7233,
    DatabaseFile: "/tmp/temporal.db",
    LogLevel:     "info",
    Namespaces:   []string{"default", "testing"},
})
if err != nil {
    return err
}

// Start server
if err := server.Start(); err != nil {
    return err
}
defer server.Stop()
```

### Client Connection
```go
// Create client for workflow operations
client, err := temporal.NewClient(temporal.ClientOptions{
    HostPort:  server.GetFrontendAddress(),
    Namespace: "default",
})
if err != nil {
    return err
}
defer client.Close()
```

### Testing Pattern
```go
func TestWorkflow(t *testing.T) {
    server, err := temporal.NewServer(temporal.Options{
        FrontendPort: temporal.FindFreePort(),
        DatabaseFile: filepath.Join(t.TempDir(), "test.db"),
        LogLevel:     "error",
        Namespaces:   []string{"test"},
    })
    require.NoError(t, err)
    
    require.NoError(t, server.Start())
    defer server.Stop()
    
    // Run tests using server.GetFrontendAddress()
}
```

## Configuration

### SQLite Optimization
```go
opts := temporal.Options{
    DatabaseFile: "temporal.db",
    SQLitePragmas: map[string]string{
        "cache_size":      "-64000",    // 64MB cache
        "mmap_size":       "268435456", // 256MB mmap
        "busy_timeout":    "30000",     // 30s timeout
    },
}
```

### Default Pragmas
- `journal_mode=WAL`: Write-ahead logging for concurrency
- `synchronous=NORMAL`: Balanced durability/performance
- `foreign_keys=ON`: Referential integrity
- `busy_timeout=10000`: 10-second lock timeout