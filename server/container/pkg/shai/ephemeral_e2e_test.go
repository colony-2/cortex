package shai

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

// This E2E test exercises the same code path as production: EphemeralRunner.Run.
// It requires network (GHCR) and Docker. It always runs and fails if unavailable.
func TestEphemeralRunner_CommonUtilsE2E(t *testing.T) {
    t.Parallel()

    // Create a temp workspace with a devcontainer.json
    dir := t.TempDir()
    dcDir := filepath.Join(dir, ".devcontainer")
    if err := os.MkdirAll(dcDir, 0755); err != nil { t.Fatal(err) }

    // Minimal devcontainer using golang bookworm + common-utils and remoteUser vscode
    devcontainer := map[string]interface{}{
        "image": "golang:1.24-bookworm",
        "remoteUser": "vscode",
        "features": map[string]interface{}{
            "ghcr.io/devcontainers/features/common-utils:2": map[string]interface{}{},
        },
        // Keep workspace simple; mount built by runner
    }
    b, _ := json.Marshal(devcontainer)
    if err := os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), b, 0644); err != nil { t.Fatal(err) }

    // Create runner
    runner, err := NewEphemeralRunner(EphemeralConfig{WorkingDir: dir})
    if err != nil {
        t.Fatalf("runner create: %v", err)
    }
    defer runner.Close()

    // Run with timeout (network + install may take some time)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()

    if err := runner.Run(ctx); err != nil {
        t.Fatalf("ephemeral run failed: %v", err)
    }
}

