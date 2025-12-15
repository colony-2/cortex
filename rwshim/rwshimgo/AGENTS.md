# rwshimgo

Go library for rwshim read/write interception. Hosts Unix socket server, launches processes with LD_PRELOAD shim, controls I/O operations via configurable policies.

## Overview

rwshimgo is a Go library that intercepts file I/O operations from child processes using a Unix domain socket server and LD_PRELOAD mechanism. It works in conjunction with a C shared library (intercept.so) to monitor and control read/write operations of spawned processes.

## Architecture

### Core Components

- **Monitor**: Unix domain socket server that receives I/O operation requests from intercepted processes
- **Process**: Wrapper for launched processes with shim library preloaded via LD_PRELOAD
- **PolicyFunc**: Callback functions that decide whether to allow or deny specific I/O operations
- **Request/Response**: Protocol types for communication between shim library and monitor

### Component Relationships

```
┌─────────────────┐    LD_PRELOAD    ┌─────────────────┐
│   Child Process │ ─────────────────► │  intercept.so   │
│                 │                   │  (C library)    │
└─────────────────┘                   └─────────────────┘
                                              │
                                              │ Unix Socket
                                              ▼
┌─────────────────┐    PolicyFunc     ┌─────────────────┐
│   Go Monitor    │ ◄─────────────────│  Socket Server  │
│                 │                   │                 │
└─────────────────┘                   └─────────────────┘
```

## Key Interfaces

### Monitor Interface

```go
type Monitor struct {
    impl *internal.MonitorImpl
}

// Core lifecycle methods
func NewMonitor(policy PolicyFunc) *Monitor
func NewMonitorWithPath(socketPath string, policy PolicyFunc) *Monitor
func (m *Monitor) Start() error
func (m *Monitor) Stop() error
func (m *Monitor) IsRunning() bool
func (m *Monitor) SocketPath() string

// Process management
func (m *Monitor) StartProcess(ctx context.Context, command string, args []string, opts ...ProcessOption) (*Process, error)
```

### Process Interface

```go
type Process struct {
    impl *internal.ProcessImpl
}

// Process control methods
func (p *Process) Start() error
func (p *Process) Wait() error
func (p *Process) WaitWithTimeout(timeout time.Duration) error
func (p *Process) Kill() error
func (p *Process) Signal(sig syscall.Signal) error
func (p *Process) Pid() int
func (p *Process) ExitCode() int
```

### Policy Types

```go
type Request struct {
    Operation Operation  // "READ" or "WRITE"
    FD        int        // File descriptor
    Size      int64      // Operation size in bytes
    Filename  string     // File path
}

type Response struct {
    Allow bool           // Whether to allow the operation
}

type PolicyFunc func(req Request) Response

// Policy builder for complex rules
type PolicyBuilder struct {
    rules []PolicyRule
}
```

## Usage Examples

### Basic Monitor Setup

```go
import "github.com/colony-2/colony2/rwshim/rwshimgo/pkg/rwshim"

// Create and start monitor
monitor := rwshim.NewMonitor(rwshim.AllowAll)
if err := monitor.Start(); err != nil {
    log.Fatal("Failed to start monitor:", err)
}
defer monitor.Stop()

// Launch monitored process
ctx := context.Background()
proc, err := monitor.StartProcess(ctx, "echo", []string{"Hello, World!"})
if err != nil {
    log.Fatal("Failed to create process:", err)
}

proc.Start()
proc.Wait()
```

### Custom Policy with Logging

```go
customPolicy := func(req rwshim.Request) rwshim.Response {
    log.Printf("Intercepted: %s fd=%d size=%d file=%s", 
        req.Operation, req.FD, req.Size, req.Filename)
    
    // Deny writes to /etc
    if req.Operation == rwshim.OpWrite && strings.HasPrefix(req.Filename, "/etc/") {
        return rwshim.Response{Allow: false}
    }
    
    return rwshim.Response{Allow: true}
}

monitor := rwshim.NewMonitor(customPolicy)
```

### Policy Builder Pattern

```go
policy := rwshim.NewPolicyBuilder().
    AllowRead(rwshim.MatchStdStreams()).                    // Allow stdin/stdout/stderr reads
    AllowWrite(rwshim.MatchStdStreams()).                   // Allow stdin/stdout/stderr writes
    DenyWrite(rwshim.MatchFilenameSuffix(".log")).          // Deny writes to .log files
    AllowRead(rwshim.MatchFilenamePrefix("/tmp/")).         // Allow reads from /tmp
    DenyWrite(rwshim.MatchLargeOperations(1024 * 1024)).   // Deny writes >1MB
    Default(true).                                          // Allow everything else
    Build()

monitor := rwshim.NewMonitor(policy)
```

### Process Configuration

```go
proc, err := monitor.StartProcess(ctx, "myapp", []string{"arg1", "arg2"},
    rwshim.WithDir("/tmp"),                                 // Set working directory
    rwshim.WithEnv([]string{"VAR=value", "PATH=/bin"}),     // Set environment
    rwshim.WithShimPath("/custom/path/intercept.so"),       // Custom shim path
    rwshim.WithStdout(os.Stdout),                          // Redirect stdout
    rwshim.WithStderr(os.Stderr),                          // Redirect stderr
)
```

### Multiple Process Management

```go
// Process with timeout
proc1, _ := monitor.StartProcess(ctx, "long-running-cmd", nil)
proc1.Start()
if err := proc1.WaitWithTimeout(30 * time.Second); err != nil {
    proc1.Kill()  // Force kill if timeout
}

// Process with signal handling
proc2, _ := monitor.StartProcess(ctx, "daemon", nil) 
proc2.Start()
proc2.Signal(syscall.SIGUSR1)  // Send custom signal
```

## Configuration

### Built-in Policies

```go
rwshim.AllowAll     // Allow all operations
rwshim.DenyAll      // Deny all operations  
rwshim.DenyWrites   // Allow reads, deny writes
rwshim.DenyReads    // Allow writes, deny reads
```

### Matcher Functions

```go
rwshim.MatchFilename(filename)              // Exact filename match
rwshim.MatchFilenamePrefix(prefix)          // Filename prefix match
rwshim.MatchFilenameSuffix(suffix)          // Filename suffix match
rwshim.MatchFD(fd)                          // File descriptor match
rwshim.MatchStdStreams()                    // stdin/stdout/stderr match
rwshim.MatchLargeOperations(threshold)      // Operations over size threshold
```

### Socket Configuration

```go
// Default socket path: /tmp/colony2-rwshim.sock
monitor := rwshim.NewMonitor(policy)

// Custom socket path
monitor := rwshim.NewMonitorWithPath("/custom/path/socket", policy)
```

## Project Structure

```
rwshimgo/
├── pkg/rwshim/         # Public API
│   ├── types.go        # Public types (Request, Response, PolicyFunc)
│   ├── monitor.go      # Monitor API implementation
│   ├── process.go      # Process API implementation  
│   ├── policies.go     # Built-in policies and PolicyBuilder
│   └── matchers.go     # Matcher functions for policies
├── internal/           # Internal implementation
│   ├── types.go        # Internal type definitions
│   ├── monitor.go      # MonitorImpl with socket server
│   ├── process.go      # ProcessImpl with shim integration
│   └── export.go       # Internal exports
├── example/            # Usage examples
│   └── main.go         # Comprehensive example scenarios
├── integration_test.sh # Docker-based integration tests
├── moon.yml           # Build configuration
├── go.mod             # Go module definition
└── rwshim.go          # Backwards compatibility wrapper
```

## Testing

```bash
# Unit tests
go test ./...
moon run test

# Integration tests with C shim (requires Docker)
./integration_test.sh
moon run integration
```

**Requirements**: intercept.so (C shim library), Unix domain sockets, Go 1.24+
