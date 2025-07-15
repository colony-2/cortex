# rwshimgo

Go library for rwshim read/write interception. Hosts Unix socket server, launches processes with LD_PRELOAD shim, controls I/O via policies.

## Core API

The public API is available through two import paths:
- `github.com/divisive-ai/vibethis/rwshim/rwshimgo` - Backwards compatibility
- `github.com/divisive-ai/vibethis/rwshim/rwshimgo/pkg/rwshim` - Recommended

```go
import "github.com/divisive-ai/vibethis/rwshim/rwshimgo/pkg/rwshim"

// Start monitor
monitor := rwshim.NewMonitor(rwshim.AllowAll)
monitor.Start()
defer monitor.Stop()

// Launch process
ctx := context.Background()
proc, _ := monitor.StartProcess(ctx, "myapp", []string{"arg1"})
proc.Start()
proc.Wait()
```

## Policies

```go
// Custom
policy := func(req Request) Response {
    if req.Operation == OpWrite && strings.HasPrefix(req.Filename, "/etc/") {
        return Response{Allow: false}
    }
    return Response{Allow: true}
}

// Builder
policy := NewPolicyBuilder().
    AllowRead(MatchFilenamePrefix("/home/")).
    DenyWrite(MatchFilenameSuffix(".log")).
    Default(false).
    Build()

// Built-in: AllowAll, DenyAll, DenyWrites, DenyReads
```

## Process Options

```go
StartProcess(ctx, cmd, args,
    WithDir("/tmp"),
    WithEnv([]string{"VAR=val"}),
    WithShimPath("/path/intercept.so"))
```

## Types

- `Monitor`: Socket server manager
- `Process`: Launched process handle  
- `Request`: {Operation, FD, Size, Filename}
- `Response`: {Allow bool}
- `PolicyFunc`: func(Request) Response

## Project Structure

```
rwshimgo/
├── pkg/rwshim/         # Public API
│   ├── types.go        # Public types
│   ├── monitor.go      # Monitor API
│   ├── process.go      # Process API
│   ├── policies.go     # Policy helpers
│   └── matchers.go     # Matcher functions
├── internal/           # Internal implementation
│   ├── types.go        # Internal types
│   ├── monitor.go      # Monitor implementation
│   └── process.go      # Process implementation
├── example/            # Example usage
└── rwshim.go          # Backwards compatibility wrapper

```

## Testing

```bash
moon run test           # Unit tests
moon run integration    # Integration with C shim
```

Requires: intercept.so, Unix sockets, Go 1.21+
