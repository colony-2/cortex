//go:build integration
// +build integration

package main

import (
    "bytes"
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "testing"
    "time"

    "github.com/docker/docker/client"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestCLI_EphemeralShell_StartsAndEchoes runs the shai CLI, starts a shell, echoes output, and exits
func TestCLI_EphemeralShell_StartsAndEchoes(t *testing.T) {
    if !dockerAvailable(t) {
        t.Skip("Docker not available")
    }

    tmp := t.TempDir()
    dcDir := filepath.Join(tmp, ".devcontainer")
    require.NoError(t, os.MkdirAll(dcDir, 0o755))

    // Use postCreateCommand to write test output, which runs before shell starts
    dc := `{
        "image":"alpine:latest",
        "workspaceFolder":"/src",
        "postCreateCommand": "echo HELLO"
    }`
    require.NoError(t, os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(dc), 0o644))

    // Build CLI binary in a temp location to avoid races
    bin := filepath.Join(tmp, "shai_bin")
    build := exec.Command("go", "build", "-o", bin, ".")
    wd, err := os.Getwd()
    require.NoError(t, err)
    build.Dir = wd
    build.Env = append(os.Environ(), "CGO_ENABLED=0")
    out, err := build.CombinedOutput()
    require.NoError(t, err, "go build failed: %s", string(out))

    // Run the CLI with stdin that immediately exits
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, bin, "-rw", ".")
    cmd.Dir = tmp
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // Provide "exit" command on stdin to terminate shell immediately
    cmd.Stdin = bytes.NewReader([]byte("exit\n"))

    err = cmd.Run()
    // Expect exit due to shell termination

    got := stdout.String() + stderr.String()

    // Check that HELLO appears in output from postCreateCommand
    assert.Contains(t, got, "HELLO", "CLI output should contain HELLO from postCreateCommand")
}

// dockerAvailable tries to ping Docker; returns true if reachable
func dockerAvailable(t *testing.T) bool {
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err == nil {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()
        if _, err := cli.Ping(ctx); err == nil {
            _ = cli.Close()
            return true
        }
        _ = cli.Close()
    }
    // Try common sockets
    sockets := []string{
        "unix:///var/run/docker.sock",
        "unix://" + os.Getenv("HOME") + "/.docker/run/docker.sock",
    }
    for _, s := range sockets {
        cli, err := client.NewClientWithOpts(client.WithHost(s), client.WithAPIVersionNegotiation())
        if err != nil { continue }
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        _, err = cli.Ping(ctx)
        cancel()
        _ = cli.Close()
        if err == nil { return true }
    }
    return false
}


