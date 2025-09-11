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
    // Minimal devcontainer; workspaceFolder is ignored by shai mounts, but set for clarity
    dc := `{"image":"alpine:latest","workspaceFolder":"/src"}`
    require.NoError(t, os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(dc), 0o644))

    // Build CLI binary in a temp location to avoid races
    bin := filepath.Join(tmp, "shai_bin")
    build := exec.Command("go", "build", "-o", bin, "./cmd/shai")
    build.Dir = repoRoot(t)
    build.Env = append(os.Environ(), "CGO_ENABLED=0")
    out, err := build.CombinedOutput()
    require.NoError(t, err, "go build failed: %s", string(out))

    // Prepare command: echo HELLO then exit
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, bin, "-rw", ".")
    cmd.Dir = tmp
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    stdin, err := cmd.StdinPipe()
    require.NoError(t, err)

    // Start process
    require.NoError(t, cmd.Start())
    // Allow container to start and shell to appear
    time.Sleep(2 * time.Second)
    // Type command and exit
    _, _ = stdin.Write([]byte("echo HELLO\nexit\n"))
    _ = stdin.Close()

    err = cmd.Wait()
    // CLI may exit with 0 or non-zero depending on shell termination; do not fail on code
    _ = err

    got := stdout.String() + stderr.String()
    assert.Contains(t, got, "HELLO", "shell output should contain HELLO")
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

// repoRoot finds the repo root assuming this test file is under server/container/cmd/shai
func repoRoot(t *testing.T) string {
    wd, err := os.Getwd()
    require.NoError(t, err)
    // walk up until we find go.mod that belongs to server/container
    // simple approach: go up 3 directories to the monorepo root
    return filepath.Clean(filepath.Join(wd, "../../.."))
}

