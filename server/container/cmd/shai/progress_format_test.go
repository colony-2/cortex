package main

import (
    "bytes"
    "strings"
    "testing"

    "github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

func TestFormatEphemeralProgress_CleanLines(t *testing.T) {
    var out bytes.Buffer
    verbose := false

    // Simulate a full sequence
    seq := []shai.ProgressUpdate{
        {Phase: "INIT", Status: "START", Message: "Initializing devcontainer setup"},
        {Phase: "FEATURES", Status: "START", Message: "Installing devcontainer features"},
        {Phase: "FEATURES", Status: "PROGRESS", Message: "Installing feature ghcr.io/devcontainers/features/common-utils:2"},
        {Phase: "FEATURES", Status: "PROGRESS", Message: "Installing feature ghcr.io/devcontainers/features/git:1"},
        {Phase: "FEATURES", Status: "COMPLETE", Message: "All features installed"},
        {Phase: "POSTCREATE", Status: "START", Message: "Executing postcreate command"},
        {Phase: "POSTCREATE", Status: "COMPLETE", Message: "Completed postcreate command"},
        {Phase: "USERSWITCH", Status: "START", Message: "Switching to user vscode"},
    }

    for _, u := range seq {
        formatEphemeralProgress(&out, u, verbose)
    }

    got := out.String()
    // No carriage returns
    if strings.Contains(got, "\r") {
        t.Fatalf("unexpected carriage returns in output: %q", got)
    }
    // Expected lines
    wantLines := []string{
        "⚙️  Initializing devcontainer setup",
        "🔧 Installing devcontainer features",
        "  - Installing feature ghcr.io/devcontainers/features/common-utils:2",
        "  - Installing feature ghcr.io/devcontainers/features/git:1",
        "✅ All features installed",
        "🔨 Executing postcreate command",
        "✅ Completed postcreate command",
        "👤 Switching to user vscode",
    }
    for _, w := range wantLines {
        if !strings.Contains(got, w+"\n") {
            t.Fatalf("missing expected line: %q in %q", w, got)
        }
    }
}

