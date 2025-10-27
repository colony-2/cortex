package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	temporalharness "github.com/divisive-ai/vibethis/server/api/internal/testsupport/temporalharness"
	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	"github.com/divisive-ai/vibethis/server/container/pkg/container"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	inputops "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
	"github.com/stretchr/testify/require"
)

// TestInputRoutes_Integration_WorkflowSuite_HTTP verifies the full integration path using
// the embedded Temporal harness and real recipe worker.
func TestInputRoutes_Integration_WorkflowSuite_HTTP(t *testing.T) {
	harness := temporalharness.Start(t)

	store := storage.NewMemoryStorage()
	gb := graph.NewBuilder(".")
	fb := files.NewBrowser(files.Config{})
	gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
	cm := container.NewManager(container.Config{})
	sseMgr := inputops.NewSimpleSSEManager()
	mgmt := inputops.GetOp().GetManagementService()
	require.NoError(t, mgmt.Initialize(harness.ServiceDependencies(sseMgr)))

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
	api := web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps)

	wfID := harness.StartWorkflow(t, map[string]interface{}{"box_id": "integration-cell"})
	harness.WaitForStatus(t, wfID, workflowctl.StatusRunning)
	pending := waitForPendingEntry(t, api, harness, wfID)
	inputID := pending.ID

	// POST response via REST (ServeHTTP) to complete the workflow.
	body := strings.NewReader(`{"id":"` + inputID + `","user_id":"tester","fields":{"approval":"yes"},"metadata":{"via":"api-test"}}`)
	postPath := "/api/user-inputs/" + wfID + "/respond"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, postPath, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "tester")
	api.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	harness.WaitForStatus(t, wfID, workflowctl.StatusCompleted)
}
