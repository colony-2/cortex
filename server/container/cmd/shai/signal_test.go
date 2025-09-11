package main

import (
    "os/signal"
    "syscall"
    "testing"
    "time"
)

// This test validates that in ephemeral mode we ignore SIGINT (Ctrl-C) at the CLI level
// so that it flows to the container shell instead of terminating the CLI process.
// We test by inspecting the signal disposition and verifying that SIGINT does not cancel the context.
func TestEphemeralIgnoresSIGINT(t *testing.T) {
    // Reset any prior signal handlers to avoid interference
    signal.Reset(syscall.SIGINT, syscall.SIGTERM)
    ctx, cancel := setupSignals()
    defer func() {
        cancel()
        signal.Reset(syscall.SIGINT, syscall.SIGTERM)
    }()

    // SIGINT should be ignored
    ignored := signal.Ignored(syscall.SIGINT)
    if !ignored {
        t.Fatalf("SIGINT not ignored in ephemeral mode")
    }

    // Send SIGINT to current process and ensure context is not cancelled
    _ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
    select {
    case <-ctx.Done():
        t.Fatal("context cancelled on SIGINT (unexpected)")
    case <-time.After(200 * time.Millisecond):
        // ok
    }
}
