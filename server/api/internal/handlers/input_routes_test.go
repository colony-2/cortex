package handlers_test

import (
    "context"
    "encoding/json"
    "fmt"
    "bufio"
    "log"
    "net/http"
    "net/http/httptest"
    "net/url"
    "net"
    commonpb "go.temporal.io/api/common/v1"
    wfsvc "go.temporal.io/api/workflowservice/v1"
    embeddedtemporal "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
    "strings"
    "testing"
    "time"

    "github.com/divisive-ai/vibethis/server/api/pkg/web"
    "github.com/divisive-ai/vibethis/server/container/pkg/container"
    "github.com/divisive-ai/vibethis/server/files/pkg/files"
    "github.com/divisive-ai/vibethis/server/git/pkg/git"
    "github.com/divisive-ai/vibethis/server/graph/pkg/graph"
    coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
    inputops "github.com/divisive-ai/vibethis/server/ops/pkg/input"
    inputpkg "github.com/divisive-ai/vibethis/server/ops/pkg/input"
    "github.com/divisive-ai/vibethis/server/storage/pkg/storage"
    "github.com/stretchr/testify/require"
    sdkclient "go.temporal.io/sdk/client"
    "go.temporal.io/sdk/worker"
)

// depsImpl implements ops.ServiceDependencies for tests
type depsImpl struct {
    sse    coreops.SSEManager
    client sdkclient.Client
}

func (d depsImpl) Get(name string) (interface{}, error) {
    switch name {
    case "sse":
        return d.sse, nil
    case "temporal_client":
        // return typed-nil client when not set
        var c sdkclient.Client = d.client
        return c, nil
    default:
        return nil, fmt.Errorf("service not found: %s", name)
    }
}

func buildTestServer(t *testing.T) (*web.Server, coreops.SSEManager) {
    t.Helper()

    // Prepare base dependencies
    store := storage.NewMemoryStorage()
    gb := graph.NewBuilder(".")
    fb := files.NewBrowser(files.Config{})
    gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
    cm := container.NewManager(container.Config{})

    // Build routes from the management service without initialization (no Temporal or SSE needed)
    op := inputops.GetOp()
    mgmt := op.GetManagementService()
    var routes []web.ExtensionRoute
    for _, r := range mgmt.GetRoutes() {
        path := r.Path
        if strings.HasPrefix(path, "/api") {
            path = strings.TrimPrefix(path, "/api")
        }
        routes = append(routes, web.ExtensionRoute{Method: r.Method, Path: path, Handler: r.Handler})
    }
    // Wrap routes for registration/invocation logging
    routes = wrapRoutes(t, routes)
    for _, rt := range routes {
        t.Logf("TEST_WRAPPER: registered method=%s path=%s", rt.Method, rt.Path)
    }

    // Wrap for logging
    routes = wrapRoutes(t, routes)

    deps := web.Dependencies{
        Storage:         store,
        Graph:           gb,
        Files:           fb,
        Git:             gr,
        Container:       cm,
        ExtensionRoutes: routes,
    }
    return web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps), nil
}

// helper to build a server with initialized input management using an embedded Temporal server
func buildServerWithTemporal(t *testing.T, port int) (*web.Server, coreops.SSEManager, func()) {
    t.Helper()
    // Prepare base dependencies
    store := storage.NewMemoryStorage()
    gb := graph.NewBuilder(".")
    fb := files.NewBrowser(files.Config{})
    gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
    cm := container.NewManager(container.Config{})

    // Start embedded Temporal server on a unique free port (avoid collisions)
    // We use a simple incremental port offset to reduce collision risk in CI
    // Prefer stable port to satisfy static host mapping
    tmpDB := t.TempDir() + "/temporal-e2e.db"
    srv, err := embeddedtemporal.NewServer(embeddedtemporal.Options{
        FrontendIP:               "127.0.0.1",
        FrontendPort:             port,
        DatabaseFile:            tmpDB,
        LogLevel:                "error",
        ReadinessTimeout:        60 * time.Second,
        DisableScanners:         true,
        DisableNexus:            true,
        DisableParentClosePolicy: true,
        Namespaces:              []string{"client-test"},
    })
    require.NoError(t, err)
    require.NoError(t, srv.Start())
    cleanup := func() { _ = srv.Stop() }

    cli, err := embeddedtemporal.NewClient(embeddedtemporal.ClientOptions{HostPort: srv.GetFrontendAddress(), Namespace: "client-test"})
    require.NoError(t, err)
    oldCleanup := cleanup
    cleanup = func() { cli.Close(); oldCleanup() }

    // Initialize management service with SSE + client
    sseMgr := inputops.NewSimpleSSEManager()
    op := inputops.GetOp()
    mgmt := op.GetManagementService()
    err = mgmt.Initialize(depsImpl{ sse: sseMgr, client: cli })
    require.NoError(t, err)

    // Routes
    var routes []web.ExtensionRoute
    for _, r := range mgmt.GetRoutes() {
        path := r.Path
        if strings.HasPrefix(path, "/api") {
            path = strings.TrimPrefix(path, "/api")
        }
        routes = append(routes, web.ExtensionRoute{Method: r.Method, Path: path, Handler: r.Handler})
    }

    // Wrap for logging
    routes = wrapRoutes(t, routes)

    deps := web.Dependencies{Storage: store, Graph: gb, Files: fb, Git: gr, Container: cm, ExtensionRoutes: routes}
    api := web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps)
    return api, sseMgr, cleanup
}

// helper to build a server with SSE only (no Temporal); useful for fast SSE tests
func buildServerWithSSEOnly(t *testing.T) (*web.Server, coreops.SSEManager) {
    t.Helper()
    store := storage.NewMemoryStorage()
    gb := graph.NewBuilder(".")
    fb := files.NewBrowser(files.Config{})
    gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
    cm := container.NewManager(container.Config{})

    sseMgr := inputops.NewSimpleSSEManager()
    op := inputops.GetOp()
    mgmt := op.GetManagementService()
    // initialize with SSE and typed-nil Temporal client
    err := mgmt.Initialize(depsImpl{ sse: sseMgr, client: nil })
    require.NoError(t, err)

    var routes []web.ExtensionRoute
    for _, r := range mgmt.GetRoutes() {
        path := r.Path
        if strings.HasPrefix(path, "/api") {
            path = strings.TrimPrefix(path, "/api")
        }
        routes = append(routes, web.ExtensionRoute{Method: r.Method, Path: path, Handler: r.Handler})
    }

    // Wrap for logging
    routes = wrapRoutes(t, routes)
    deps := web.Dependencies{Storage: store, Graph: gb, Files: fb, Git: gr, Container: cm, ExtensionRoutes: routes}
    return web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps), sseMgr
}

// wrapRoutes logs registration and invocation of extension routes to aid debugging
func wrapRoutes(t *testing.T, routes []web.ExtensionRoute) []web.ExtensionRoute {
    out := make([]web.ExtensionRoute, 0, len(routes))
    for _, r := range routes {
        method := r.Method
        path := r.Path
        t.Logf("TEST_WRAPPER: register method=%s path=%s", method, path)
        base := r.Handler
        handler := func(w http.ResponseWriter, req *http.Request) {
            u := &url.URL{Path: req.URL.Path}
            log.Printf("TEST_WRAPPER: invoke method=%s path=%s raw=%s", req.Method, path, u.String())
            base(w, req)
        }
        out = append(out, web.ExtensionRoute{Method: method, Path: path, Handler: handler})
    }
    return out
}

func TestInputRoutes_Pending(t *testing.T) {
    server, _ := buildTestServer(t)

    req := httptest.NewRequest("GET", "/api/user-inputs/pending", nil)
    rec := httptest.NewRecorder()
    server.ServeHTTP(rec, req)

    require.Equal(t, http.StatusOK, rec.Code)
    var pending []map[string]interface{}
    require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pending))
}

func TestInputRoutes_SSEConnect(t *testing.T) {
    server, _ := buildTestServer(t)

    // Short timeout to end stream
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    req := httptest.NewRequest("GET", "/api/user-inputs/stream", nil).WithContext(ctx)
    req.Header.Set("X-Client-ID", "test-client")
    rec := httptest.NewRecorder()
    done := make(chan struct{})
    go func() {
        server.ServeHTTP(rec, req)
        close(done)
    }()

    // Give the stream time to connect and send initial event
    time.Sleep(50 * time.Millisecond)
    // No SSE configured in this setup; expect handler to report error and close

    // Wait for context timeout to close stream
    <-done

    body := rec.Body.String()
    require.Contains(t, body, "event: error")
}

func TestInputRoutes_SimpleInputCycle(t *testing.T) {
    // Start embedded Temporal server on an available port to avoid collisions
    tmpDB := t.TempDir() + "/temporal-e2e.db"
    port := findFreePort(t)
    srv, err := embeddedtemporal.NewServer(embeddedtemporal.Options{
        FrontendIP:               "127.0.0.1",
        FrontendPort:             port,
        DatabaseFile:            tmpDB,
        LogLevel:                "error",
        ReadinessTimeout:        60 * time.Second,
        DisableScanners:         true,
        DisableNexus:            true,
        DisableParentClosePolicy: true,
        Namespaces:              []string{"client-test"},
    })
    require.NoError(t, err)
    require.NoError(t, srv.Start())
    t.Cleanup(func() { _ = srv.Stop() })

    // Create Temporal client and worker
    cli, err := embeddedtemporal.NewClient(embeddedtemporal.ClientOptions{HostPort: srv.GetFrontendAddress(), Namespace: "client-test"})
    require.NoError(t, err)
    t.Cleanup(func() { cli.Close() })

    taskQueue := "input-e2e-queue"
    w := worker.New(cli, taskQueue, worker.Options{})
    w.RegisterWorkflow(inputpkg.InputCollectionWorkflow)
    // Start worker and wait until pollers are running
    require.NoError(t, w.Start())
    t.Cleanup(func() { w.Stop() })

    // Start a workflow instance that waits for user-response signal
    wfID := fmt.Sprintf("test-input-%d", time.Now().UnixNano())
    params := inputpkg.InputWorkflowParams{
        Form: inputpkg.InputForm{Question: "Approve?", Type: inputpkg.FieldTypeMultipleChoice, Options: []inputpkg.Option{{Value: "yes"}, {Value: "no"}}, Timeout: 20 * time.Second},
        Timeout:    20 * time.Second,
        BoxID:      "test-cell",
        ActivityID: "approve-activity",
    }
    // Start workflow asynchronously
    we, err := cli.ExecuteWorkflow(context.Background(), sdkclient.StartWorkflowOptions{ID: wfID, TaskQueue: taskQueue}, inputpkg.InputCollectionWorkflow, params)
    require.NoError(t, err)

    // Build API server with input management service initialized with SSE + client
    store := storage.NewMemoryStorage()
    gb := graph.NewBuilder(".")
    fb := files.NewBrowser(files.Config{})
    gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
    cm := container.NewManager(container.Config{})
    sseMgr := inputops.NewSimpleSSEManager()
    op := inputops.GetOp()
    mgmt := op.GetManagementService()
    err = mgmt.Initialize(depsImpl{sse: sseMgr, client: cli})
    require.NoError(t, err)
    var routes []web.ExtensionRoute
    for _, r := range mgmt.GetRoutes() {
        path := r.Path
        if strings.HasPrefix(path, "/api") {
            path = strings.TrimPrefix(path, "/api")
        }
        routes = append(routes, web.ExtensionRoute{Method: r.Method, Path: path, Handler: r.Handler})
    }
    deps := web.Dependencies{Storage: store, Graph: gb, Files: fb, Git: gr, Container: cm, ExtensionRoutes: routes}
    api := web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps)

    // Use a real HTTP server to ensure flushing works with SSE
    ts := httptest.NewServer(api)
    defer ts.Close()

    // Start SSE stream
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/user-inputs/stream", nil)
    req.Header.Set("X-Client-ID", "client-1")
    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()

    // Submit response to trigger SSE broadcast
    t.Logf("POST respond for workflowID=%s", wfID)
    body := strings.NewReader(`{"fields":{"approval":"yes"},"metadata":{"via":"api-test"}}`)
    postURL := ts.URL+"/api/user-inputs/"+wfID+"/respond"
    t.Logf("TEST_WRAPPER: posting respond url=%s", postURL)
    postReq, _ := http.NewRequest(http.MethodPost, postURL, body)
    postReq.Header.Set("Content-Type", "application/json")
    postReq.Header.Set("X-User-ID", "tester")
    postResp, err := http.DefaultClient.Do(postReq)
    require.NoError(t, err)
    require.Equal(t, http.StatusOK, postResp.StatusCode)
    _ = postResp.Body.Close()

    // Describe workflow after posting response to verify state
    dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer dcancel()
    desc, derr := cli.DescribeWorkflowExecution(dctx, wfID, "")
    if derr != nil {
        t.Logf("DescribeWorkflowExecution error: %v", derr)
    } else if desc != nil && desc.WorkflowExecutionInfo != nil {
        t.Logf("WF Status after respond: %s", desc.WorkflowExecutionInfo.Status.String())
    }

    // Control: also send direct signal via client to ensure delivery path works
    ctrlSig := inputpkg.UserResponseSignal{
        Fields:      map[string]interface{}{"approval": "yes"},
        UserID:      "tester",
        RespondedAt: time.Now(),
        Metadata:    map[string]interface{}{"via": "control-signal"},
    }
    sigErr := cli.SignalWorkflow(context.Background(), wfID, "", "user-response", ctrlSig)
    if sigErr != nil {
        t.Logf("Control SignalWorkflow error: %v", sigErr)
    } else {
        t.Logf("Control signal sent to %s", wfID)
    }

    // Read SSE events and verify connected + input_completed while also waiting for workflow completion
    scanner := bufio.NewScanner(resp.Body)
    gotConnected := false
    gotCompleted := false

    // Wait for workflow result concurrently
    wfDone := make(chan error, 1)
    go func() {
        var result inputpkg.InputWorkflowResult
        wfCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        wfDone <- we.Get(wfCtx, &result)
    }()

    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) && (!gotConnected || !gotCompleted) {
        if !scanner.Scan() {
            // Small wait to allow more data
            time.Sleep(20 * time.Millisecond)
            continue
        }
        line := scanner.Text()
        t.Logf("SSE: %s", line)
        if strings.HasPrefix(line, "event: ") {
            if strings.Contains(line, "connected") {
                gotConnected = true
            }
            if strings.Contains(line, "input_completed") {
                gotCompleted = true
            }
        }
        select {
        case err := <-wfDone:
            require.NoError(t, err)
        default:
        }
    }
    require.True(t, gotConnected, "expected connected event")
    require.True(t, gotCompleted, "expected input_completed event")
    // Ensure workflow finished successfully
    select {
    case err := <-wfDone:
        require.NoError(t, err)
    default:
        // Fetch history to help diagnose signal delivery
        hctx, hcancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer hcancel()
        hist, herr := cli.WorkflowService().GetWorkflowExecutionHistory(hctx, &wfsvc.GetWorkflowExecutionHistoryRequest{
            Namespace: "client-test",
            Execution: &commonpb.WorkflowExecution{WorkflowId: wfID},
        })
        if herr != nil {
            t.Logf("GetWorkflowExecutionHistory error: %v", herr)
        } else if hist != nil && hist.History != nil {
            evts := hist.History.Events
            // Log last few events concisely
            max := 10
            if len(evts) < max {
                max = len(evts)
            }
            for i := len(evts) - max; i < len(evts); i++ {
                if i < 0 { continue }
                t.Logf("History[%d]: type=%v", i, evts[i].GetEventType())
            }
        }
        t.Fatal("workflow did not complete in time")
    }
}

// findFreePort finds an available TCP port on localhost for test use
func findFreePort(t *testing.T) int {
    t.Helper()
    l, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("failed to get free port: %v", err)
    }
    defer l.Close()
    return l.Addr().(*net.TCPAddr).Port
}
