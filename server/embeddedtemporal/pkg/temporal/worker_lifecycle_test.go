package temporal_test

import (
    "context"
    "testing"
    "time"

    et "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
    "github.com/stretchr/testify/require"
    sdkclient "go.temporal.io/sdk/client"
    "go.temporal.io/sdk/worker"
    "go.temporal.io/sdk/workflow"
)

// simpleWorkflow waits for a signal named "respond" carrying a bool, then returns.
func simpleWorkflow(ctx workflow.Context) error {
    ch := workflow.GetSignalChannel(ctx, "respond")
    var ok bool
    // Block until we get the signal
    ch.Receive(ctx, &ok)
    return nil
}

func TestWorkerLifecycle_SimpleWorkflow_FastTeardown(t *testing.T) {
    // Start embedded server with fast-teardown toggles to reproduce API test pattern
    port := et.FindFreePort()
    tmpDB := t.TempDir() + "/temporal-fast-teardown.db"

    srv, err := et.NewServer(et.Options{
        FrontendIP:              "127.0.0.1",
        FrontendPort:            port,
        DatabaseFile:            tmpDB,
        LogLevel:                "error",
        Namespaces:              []string{"client-test"},
        DisableScanners:         true,
        DisableNexus:            true,
        DisableParentClosePolicy: true,
    })
    require.NoError(t, err)
    t.Logf("starting embedded temporal on %s", srv.GetFrontendAddress())
    require.NoError(t, srv.Start())
    t.Logf("server started")
    t.Cleanup(func() { _ = srv.Stop() })

    // SDK client
    t.Logf("dialing sdk client")
    cli, err := et.NewClient(et.ClientOptions{HostPort: srv.GetFrontendAddress(), Namespace: "client-test"})
    require.NoError(t, err)
    t.Logf("sdk client connected")
    t.Cleanup(func() { cli.Close() })

    // Worker
    tq := "input-e2e-queue"
    t.Logf("starting sdk worker on %s", tq)
    w := worker.New(cli, tq, worker.Options{})
    w.RegisterWorkflow(simpleWorkflow)
    require.NoError(t, w.Start())
    t.Cleanup(func() { w.Stop() })

    // Start workflow
    wfID := "wf-" + time.Now().Format("150405.000000000")
    t.Logf("starting workflow")
    we, err := cli.ExecuteWorkflow(context.Background(), sdkclient.StartWorkflowOptions{
        ID:        wfID,
        TaskQueue: tq,
    }, simpleWorkflow)
    require.NoError(t, err)
    t.Logf("workflow started: %s %s", we.GetID(), we.GetRunID())

    // Signal to complete
    t.Logf("signaling workflow")
    err = cli.SignalWorkflow(context.Background(), we.GetID(), we.GetRunID(), "respond", true)
    require.NoError(t, err)

    // Wait for completion with a short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    t.Logf("waiting workflow result")
    require.NoError(t, we.Get(ctx, nil))
    t.Logf("workflow completed")

    // Stop server should be quick under fast-teardown toggles
    start := time.Now()
    require.NoError(t, srv.Stop())
    if d := time.Since(start); d > 5*time.Second {
        t.Fatalf("Server.Stop took too long: %s", d)
    }
}
