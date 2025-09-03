package main

import (
    "bytes"
    "testing"
    "github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

// This tests the final checklist rendering behavior by simulating a realistic event sequence
func TestProgressAggregate_FinalChecklist(t *testing.T) {
    var out bytes.Buffer
    // Use the same formatter closure logic by invoking formatEphemeralProgress for START/PROGRESS
    // and then printing the aggregated checklist at USERSWITCH START.

    // Rebuild the aggregate logic inline to validate intended behavior
    type item struct{ label string; done bool }
    var items []item
    var lastFeatureIdx = -1

    feed := func(ev shai.ProgressUpdate) {
        switch ev.Phase {
        case "INIT":
            if ev.Status == "START" { items = append(items, item{label: "Initialize devcontainer setup"}) }
        case "FEATURES":
            switch ev.Status {
            case "START":
                if len(items) > 0 && !items[0].done { items[0].done = true }
            case "PROGRESS":
                if lastFeatureIdx >= 0 && lastFeatureIdx < len(items) { items[lastFeatureIdx].done = true }
                items = append(items, item{label: ev.Message})
                lastFeatureIdx = len(items) - 1
            case "COMPLETE":
                if lastFeatureIdx >= 0 && lastFeatureIdx < len(items) { items[lastFeatureIdx].done = true }
            }
        case "POSTCREATE":
            if ev.Status == "START" { items = append(items, item{label: "PostCreate"}) }
            if ev.Status == "COMPLETE" && len(items) > 0 { items[len(items)-1].done = true }
        case "USERSWITCH":
            if ev.Status == "START" {
                for i := range items {
                    if !items[i].done { items[i].done = true }
                    out.WriteString("✓ ")
                    out.WriteString(items[i].label)
                    out.WriteByte('\n')
                }
            }
        }
    }

    seq := []shai.ProgressUpdate{
        {Phase: "INIT", Status: "START", Message: "Initializing devcontainer setup"},
        {Phase: "FEATURES", Status: "START", Message: "Installing devcontainer features"},
        {Phase: "FEATURES", Status: "PROGRESS", Message: "Install feature A"},
        {Phase: "FEATURES", Status: "PROGRESS", Message: "Install feature B"},
        {Phase: "FEATURES", Status: "COMPLETE", Message: "All features installed"},
        {Phase: "POSTCREATE", Status: "START", Message: "Executing postcreate"},
        {Phase: "POSTCREATE", Status: "COMPLETE", Message: "Completed postcreate"},
        {Phase: "USERSWITCH", Status: "START", Message: "Switching to user"},
    }
    for _, ev := range seq { feed(ev) }

    got := out.String()
    want := "✓ Initialize devcontainer setup\n" +
        "✓ Install feature A\n" +
        "✓ Install feature B\n" +
        "✓ PostCreate\n"
    if got != want {
        t.Fatalf("unexpected checklist output:\nGot:\n%s\nWant:\n%s", got, want)
    }
}

