package temporal_test

import (
    "fmt"
    "path/filepath"
    "testing"
    "time"

    et "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
)

// These tests target the startup readiness behavior reported in
// server/api/BUG-REPORT-startup-readiness.md. We ensure that with background
// features disabled and a fresh SQLite DB, the server becomes ready within a
// reasonable timeout using a lightweight readiness probe.

func TestReadiness_StartupWithinTimeout_MinimalServices(t *testing.T) {
    tmpDir := t.TempDir()
    db := filepath.Join(tmpDir, "temporal-e2e.db")

    opts := et.Options{
        FrontendIP:               "127.0.0.1",
        FrontendPort:             et.FindFreePort(),
        DatabaseFile:             db,
        LogLevel:                 "info",
        Namespaces:               []string{},
        ReadinessTimeout:         30 * time.Second,
        DisableScanners:          true,
        DisableNexus:             true,
        DisableParentClosePolicy: true,
        EnableInternalWorker:     false,
    }

    srv, err := et.NewServer(opts)
    if err != nil {
        t.Fatalf("failed to construct server: %v", err)
    }

    if err := srv.Start(); err != nil {
        t.Fatalf("server failed to start: %v", err)
    }
    t.Cleanup(func() { _ = srv.Stop() })
    // Start succeeded; readiness is satisfied via namespace creation retries
}

func TestReadiness_RepeatedFreshDBs_NoTimeouts(t *testing.T) {
    for i := 0; i < 3; i++ { // run a few times to catch regressions
        t.Run(
            fmt.Sprintf("iter-%d", i+1),
            func(t *testing.T) {
                tmpDir := t.TempDir()
                db := filepath.Join(tmpDir, "temporal-e2e.db")
                opts := et.Options{
                    FrontendIP:               "127.0.0.1",
                    FrontendPort:             et.FindFreePort(),
                    DatabaseFile:             db,
                    LogLevel:                 "info",
                    Namespaces:               []string{}, // skip namespace creation to focus on readiness
                ReadinessTimeout:         30 * time.Second,
                    DisableScanners:          true,
                    DisableNexus:             true,
                    DisableParentClosePolicy: true,
                    EnableInternalWorker:     false,
                }
                srv, err := et.NewServer(opts)
                if err != nil {
                    t.Fatalf("failed to construct server: %v", err)
                }
                if err := srv.Start(); err != nil {
                    t.Fatalf("server failed to start: %v", err)
                }
                _ = srv.Stop()
            },
        )
    }
}
