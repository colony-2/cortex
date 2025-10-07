package handlers_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

// TestInputRoutes_Integration_WorkflowSuite_HTTP verifies the full integration path:
// Temporal WorkflowTestSuite + REST HTTP call via management service.
func TestInputRoutes_Integration_WorkflowSuite_HTTP(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(inputpkg.InputCollectionWorkflow)

	wfID := fmt.Sprintf("test-input-%d", time.Now().UnixNano())
	id := "api-int-id"
	params := inputpkg.InputWorkflowParams{
		ID:         id,
		Form:       inputpkg.InputForm{Question: "Approve?", Type: inputpkg.FieldTypeMultipleChoice, Options: []inputpkg.Option{{Value: "yes"}, {Value: "no"}}, Timeout: 5 * time.Second},
		Timeout:    5 * time.Second,
		BoxID:      "test-cell",
		ActivityID: "approve-activity",
	}
	go env.ExecuteWorkflow(inputpkg.InputCollectionWorkflow, params)
	time.Sleep(10 * time.Millisecond)

	// Build API server with management service that uses typed workflow control
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

	// Mount the management routes
	var routes []web.ExtensionRoute
	for _, r := range mgmt.GetRoutes() {
		p := r.Path
		if strings.HasPrefix(p, "/api") {
			p = strings.TrimPrefix(p, "/api")
		}
		routes = append(routes, web.ExtensionRoute{Method: r.Method, Path: p, Handler: r.Handler})
	}
	deps := web.Dependencies{Storage: store, Graph: gb, Files: fb, Git: gr, Container: cm, ExtensionRoutes: routes}
	api := web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps)

	// POST response via REST (ServeHTTP) to complete the workflow
	body := strings.NewReader(`{"id":"` + id + `","user_id":"tester","fields":{"approval":"yes"},"metadata":{"via":"api-test"}}`)
	postPath := "/api/user-inputs/" + wfID + "/respond"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, postPath, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "tester")
	api.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Wait for completion
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("workflow did not complete in time")
	default:
		// spin until completed or timeout
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if env.IsWorkflowCompleted() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("workflow did not complete in time")
	}
}
