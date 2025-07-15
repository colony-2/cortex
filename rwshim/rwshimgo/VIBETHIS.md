# rwshimgo

Go library for rwshim read/write interception. Hosts Unix socket server, launches processes with LD_PRELOAD shim, controls I/O via policies.

## Core API

```go
// Start monitor
monitor := rwshimgo.NewMonitor(rwshimgo.AllowAll)
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

## Testing

```bash
moon run test           # Unit tests
moon run integration    # Integration with C shim
```

Requires: intercept.so, Unix sockets, Go 1.21+
