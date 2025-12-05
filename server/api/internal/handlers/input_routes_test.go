package handlers_test

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
	"github.com/stretchr/testify/require"
)

func buildServerWithHarness(t *testing.T, sse coreops.SSEManager) (*web.Server, coreops.SSEManager) {
	t.Helper()
	store := storage.NewMemoryStorage()
	gb := graph.NewBuilder(".")
	fb := files.NewBrowser(files.Config{})
	gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
	cm := container.NewManager(container.Config{})

	if sse == nil {
		sse = inputops.NewSimpleSSEManager()
	}
	op := inputops.GetOp()
	mgmt := op.GetManagementService()
	require.NoError(t, mgmt.Initialize(h.ServiceDependencies(sse)))

	var routes []web.ExtensionRoute
	for _, r := range mgmt.GetRoutes() {
		path := r.Path
		if strings.HasPrefix(path, "/api") {
			path = strings.TrimPrefix(path, "/api")
		}
		routes = append(routes, web.ExtensionRoute{Method: r.Method, Path: path, Handler: r.Handler})
	}
	routes = wrapRoutes(t, routes)

	deps := web.Dependencies{Storage: store, Graph: gb, Files: fb, Git: gr, Container: cm, ExtensionRoutes: routes}
	return web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps), sse
}

func TestInputRoutes_Pending(t *testing.T) {
	harness := temporalharness.Start(t)
	server, _ := buildServerWithHarness(t, harness, nil)

	wfID := harness.StartWorkflow(t, map[string]interface{}{"box_id": "cell-x"})
	harness.WaitForStatus(t, wfID, workflowctl.StatusRunning)
	harness.DumpHistory(t, wfID)
	harness.WaitForSearchAttribute(t, wfID, "InputStatus", "pending")

	pending := waitForPendingEntry(t, server, harness, wfID)
	require.Equal(t, wfID, pending.WorkflowID)
	require.NotEmpty(t, pending.ID)
}

func TestInputRoutes_SSEConnect(t *testing.T) {
	harness := temporalharness.Start(t)
	server, _ := buildServerWithHarness(t, harness, nil)

	wfID := harness.StartWorkflow(t, map[string]interface{}{"box_id": "cell-sse"})
	harness.WaitForStatus(t, wfID, workflowctl.StatusRunning)
	harness.WaitForSearchAttribute(t, wfID, "InputStatus", "pending")
	_ = waitForPendingEntry(t, server, harness, wfID)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/user-inputs/stream", nil).WithContext(ctx)
	req.Header.Set("X-Client-ID", "test-client")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.ServeHTTP(rec, req)
		close(done)
	}()

	<-done

	body := rec.Body.String()
	t.Logf("SSE body: %s", body)
	require.Contains(t, body, "event: connected")
	require.Contains(t, body, "event: input_pending")
	require.Contains(t, body, wfID)
}

func TestInputRoutes_SimpleInputCycle(t *testing.T) {
	harness := temporalharness.Start(t)
	server, _ := buildServerWithHarness(t, harness, nil)

	wfID := harness.StartWorkflow(t, map[string]interface{}{"box_id": "test-cell"})
	harness.WaitForStatus(t, wfID, workflowctl.StatusRunning)
	pending := waitForPendingEntry(t, server, harness, wfID)
	responseID := pending.ID

	// Submit response via API
	payload := `{"id":"` + responseID + `","user_id":"tester","fields":{"approval":"yes"}}`
	respondReq := httptest.NewRequest(http.MethodPost, "/api/user-inputs/"+wfID+"/respond", strings.NewReader(payload))
	respondReq.Header.Set("Content-Type", "application/json")
	respondReq.Header.Set("X-User-ID", "tester")
	respondRec := httptest.NewRecorder()
	server.ServeHTTP(respondRec, respondReq)
	require.Equal(t, http.StatusOK, respondRec.Code)

	harness.WaitForStatus(t, wfID, workflowctl.StatusCompleted)
}

func waitForPendingEntry(t *testing.T, server *web.Server, harness *temporalharness.Harness, workflowID string) inputpkg.PendingInput {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/user-inputs/pending", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var pending []inputpkg.PendingInput
		if err := json.Unmarshal(rec.Body.Bytes(), &pending); err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		for _, entry := range pending {
			if entry.WorkflowID == workflowID {
				return entry
			}
		}
		if harness != nil {
			summary, err := harness.WorkflowControl().Describe(context.Background(), workflowctl.ExecutionRef{WorkflowID: workflowID})
			if err == nil {
				t.Logf("workflow %s status=%s attrs=%v", workflowID, summary.Status, summary.SearchAttributes)
			} else {
				t.Logf("describe workflow %s failed: %v", workflowID, err)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pending input for workflow %s not found", workflowID)
	return inputpkg.PendingInput{}
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

func waitForState(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
		return
	case <-time.After(2 * time.Second):
		t.Fatalf("%s", msg)
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
