package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	"github.com/divisive-ai/vibethis/server/container/pkg/container"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	inputops "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	inputpkg "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func buildDeps(sse coreops.SSEManager, ctl workflowctl.WorkflowControl, namespace string) coreops.ServiceDependencies2 {
	b := coreops.NewServiceDepsBuilder()
	if sse != nil {
		b = b.WithSSEManager(sse)
	}
	if ctl != nil {
		b = b.WithWorkflowControl(ctl)
	}
	if namespace != "" {
		b = b.WithTemporalNamespace(namespace)
	}
	return b.Build()
}

// suiteWorkflowCtl adapts the Temporal WorkflowTestSuite environment to workflowctl.WorkflowControl
type suiteWorkflowCtl struct {
	env *testsuite.TestWorkflowEnvironment
}

func (c *suiteWorkflowCtl) Describe(ctx context.Context, ref workflowctl.ExecutionRef) (workflowctl.WorkflowSummary, error) {
	status := workflowctl.StatusRunning
	if c.env.IsWorkflowCompleted() {
		status = workflowctl.StatusCompleted
	}
	return workflowctl.WorkflowSummary{WorkflowID: ref.WorkflowID, Status: status}, nil
}

func (c *suiteWorkflowCtl) Signal(ctx context.Context, ref workflowctl.ExecutionRef, signalName string, payload any) error {
	c.env.SignalWorkflow(signalName, payload)
	return nil
}

func (c *suiteWorkflowCtl) Cancel(ctx context.Context, ref workflowctl.ExecutionRef, reason string) error {
	return nil
}

func (c *suiteWorkflowCtl) ResetWorkflow(ctx context.Context, req workflowctl.ResetRequest) (workflowctl.ResetResponse, error) {
	return workflowctl.ResetResponse{Execution: req.Execution, Completed: req.WaitForResult}, nil
}

func (c *suiteWorkflowCtl) StartWorkflow(ctx context.Context, req workflowctl.StartRequest) (workflowctl.StartResponse, error) {
	return workflowctl.StartResponse{Execution: workflowctl.ExecutionRef{WorkflowID: req.WorkflowID}}, nil
}

func (c *suiteWorkflowCtl) StartChildWorkflow(ctx context.Context, req workflowctl.StartChildRequest) (workflowctl.StartChildResponse, error) {
	return workflowctl.StartChildResponse{Execution: workflowctl.ExecutionRef{WorkflowID: req.WorkflowID}}, nil
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
// buildServerWithTemporal removed in favor of WorkflowTestSuite-based control

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
	// initialize with SSE only and no workflow control
	err := mgmt.Initialize(buildDeps(sseMgr, nil, ""))
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
	// Use WorkflowTestSuite (no external server)
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(inputpkg.InputCollectionWorkflow)

	// Start a workflow instance that waits for user-response signal
	wfID := fmt.Sprintf("test-input-%d", time.Now().UnixNano())
	id := "api-input-id"
	params := inputpkg.InputWorkflowParams{
		ID:         id,
		Form:       inputpkg.InputForm{Question: "Approve?", Type: inputpkg.FieldTypeMultipleChoice, Options: []inputpkg.Option{{Value: "yes"}, {Value: "no"}}, Timeout: 20 * time.Second},
		Timeout:    20 * time.Second,
		BoxID:      "test-cell",
		ActivityID: "approve-activity",
	}
	// Start workflow asynchronously in the test environment
	go env.ExecuteWorkflow(inputpkg.InputCollectionWorkflow, params)
	// Give the workflow time to start
	time.Sleep(20 * time.Millisecond)

	// Build API server with input management service initialized with SSE + suite-backed workflow control
	store := storage.NewMemoryStorage()
	gb := graph.NewBuilder(".")
	fb := files.NewBrowser(files.Config{})
	gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
	cm := container.NewManager(container.Config{})
	sseMgr := inputops.NewSimpleSSEManager()
	op := inputops.GetOp()
	mgmt := op.GetManagementService()
	ctl := &suiteWorkflowCtl{env: env}
	err := mgmt.Initialize(buildDeps(sseMgr, ctl, ""))
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

	// Submit response to trigger workflow signal via workflow control
	t.Logf("POST respond for workflowID=%s", wfID)
	body := strings.NewReader(`{"id":"` + id + `","user_id":"tester","fields":{"approval":"yes"},"metadata":{"via":"api-test"}}`)
	postPath := "/api/user-inputs/" + wfID + "/respond"
	t.Logf("TEST_WRAPPER: posting respond path=%s", postPath)
	rec := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, postPath, body)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("X-User-ID", "tester")
	api.ServeHTTP(rec, postReq)
	require.Equal(t, http.StatusOK, rec.Code)

	// Wait for workflow completion via env
	wfDone := make(chan struct{}, 1)
	go func() {
		for !env.IsWorkflowCompleted() {
			time.Sleep(10 * time.Millisecond)
		}
		wfDone <- struct{}{}
	}()

	// Ensure workflow finished successfully
	select {
	case <-wfDone:
		// ok
	case <-time.After(2 * time.Second):
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
