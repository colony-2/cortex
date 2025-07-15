# rwshimgo - Go Library for Read/Write Interception

A Go library that provides a monitoring interface for the rwshim read/write interception system. This library allows you to:

- Host a Unix domain socket server that communicates with the LD_PRELOAD shim
- Launch processes with automatic shim injection
- Define custom policies for allowing/denying I/O operations
- Monitor and control file access in real-time

## Installation

```bash
go get github.com/divisive-ai/vibethis/rwshim/rwshimgo
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "github.com/divisive-ai/vibethis/rwshim/rwshimgo"
)

func main() {
    // Create a monitor with a simple policy
    monitor := rwshimgo.NewMonitor(rwshimgo.AllowAll)
    
    // Start the monitor
    if err := monitor.Start(); err != nil {
        log.Fatal(err)
    }
    defer monitor.Stop()
    
    // Launch a process with monitoring
    ctx := context.Background()
    proc, err := monitor.StartProcess(ctx, "cat", []string{"/etc/passwd"})
    if err != nil {
        log.Fatal(err)
    }
    
    // Run the process
    if err := proc.Start(); err != nil {
        log.Fatal(err)
    }
    
    if err := proc.Wait(); err != nil {
        log.Fatal(err)
    }
}
```

## Custom Policies

Define custom policies to control I/O operations:

```go
// Custom policy function
policy := func(req rwshimgo.Request) rwshimgo.Response {
    // Log the operation
    log.Printf("%s %s (fd=%d, size=%d)\n", 
        req.Operation, req.Filename, req.FD, req.Size)
    
    // Deny writes to sensitive files
    if req.Operation == rwshimgo.OpWrite && 
       strings.HasPrefix(req.Filename, "/etc/") {
        return rwshimgo.Response{Allow: false}
    }
    
    return rwshimgo.Response{Allow: true}
}

monitor := rwshimgo.NewMonitor(policy)
```

## Policy Builder

Use the PolicyBuilder for complex policies:

```go
policy := rwshimgo.NewPolicyBuilder().
    // Allow reads from /home
    AllowRead(rwshimgo.MatchFilenamePrefix("/home/")).
    // Deny writes to log files
    DenyWrite(rwshimgo.MatchFilenameSuffix(".log")).
    // Allow all standard I/O
    AllowRead(rwshimgo.MatchStdStreams()).
    AllowWrite(rwshimgo.MatchStdStreams()).
    // Default deny
    Default(false).
    Build()
```

## Process Options

Configure process execution with options:

```go
proc, err := monitor.StartProcess(ctx, "myapp", []string{"arg1"},
    rwshimgo.WithDir("/tmp"),
    rwshimgo.WithEnv([]string{"DEBUG=1"}),
    rwshimgo.WithShimPath("/custom/path/intercept.so"),
)
```

## Pre-built Policies

- `AllowAll` - Allows all I/O operations
- `DenyAll` - Denies all I/O operations  
- `DenyWrites` - Allows reads, denies writes
- `DenyReads` - Allows writes, denies reads

## Testing

Run tests using standard Go tooling:

```bash
go test ./...
```

Or run tests in Docker (no additional packages required):

```bash
./test.sh
```

## Requirements

- The C shim library (`intercept.so`) must be available
- Unix domain sockets support (Linux/macOS)
- Go 1.21 or later

## Architecture

The library consists of:

1. **Monitor** - Manages the Unix domain socket server
2. **Process** - Handles process creation with LD_PRELOAD
3. **Policies** - Defines rules for I/O operations
4. **PolicyBuilder** - Fluent API for complex policies

## Thread Safety

The Monitor is thread-safe and can handle multiple concurrent connections. Each I/O operation creates a new connection to the monitoring socket.