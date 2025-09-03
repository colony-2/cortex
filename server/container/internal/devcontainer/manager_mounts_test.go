package devcontainer

import (
    "testing"
    "github.com/stretchr/testify/require"
    pkgcontainer "github.com/divisive-ai/vibethis/server/container/pkg/container"
)

func TestApplyCustomMountsMergesExisting(t *testing.T) {
    mgr := &Manager{}
    dc := &DevContainer{}

    // Existing devcontainer mounts (object + string)
    dc.Mounts = []interface{}{
        map[string]interface{}{"type": "bind", "source": "/host/a", "target": "/container/a", "readonly": true},
        "type=volume,source=vol1,target=/container/vol1",
    }

    // Custom mounts include one overriding /container/a and one new
    mgr.customMounts = []pkgcontainer.Mount{
        {Type: "bind", Source: "/host/a-new", Target: "/container/a", ReadOnly: false},
        {Type: "bind", Source: "/host/b", Target: "/container/b", ReadOnly: false},
    }

    require.NoError(t, mgr.applyCustomMounts(dc))

    // Verify existing string mount is preserved
    foundString := false
    foundA := false
    foundB := false
    for _, m := range dc.Mounts {
        switch v := m.(type) {
        case string:
            if v == "type=volume,source=vol1,target=/container/vol1" {
                foundString = true
            }
        case map[string]interface{}:
            if tgt, _ := v["target"].(string); tgt == "/container/a" {
                // Should be overridden by custom
                if src, _ := v["source"].(string); src == "/host/a-new" {
                    foundA = true
                }
            }
            if tgt, _ := v["target"].(string); tgt == "/container/b" {
                foundB = true
            }
        }
    }

    require.True(t, foundString, "existing string mount should be preserved")
    require.True(t, foundA, "object mount should be overridden by custom")
    require.True(t, foundB, "custom mount should be added")
}
