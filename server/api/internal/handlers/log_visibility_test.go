package handlers_test

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/shai/pkg/shai/runtime"
	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
)

// Test that log.Printf from HTTP handlers shows up in go test output
func TestLogVisibilityInHandlers(t *testing.T) {
	store := storage.NewMemoryStorage()
	gb := graph.NewBuilder(".")
	fb := files.NewBrowser(files.Config{})
	gr := git.NewRepository(git.Config{DefaultAuthor: "Test", DefaultEmail: "test@example.com"})
	cm := container.NewManager(container.Config{})

	// Add a simple route that logs to the default logger
	routes := []web.ExtensionRoute{
		{
			Method: "POST",
			Path:   "/log-echo",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				log.Printf("TEST_LOG: handler called path=%s", r.URL.Path)
				w.WriteHeader(http.StatusOK)
			},
		},
	}

	deps := web.Dependencies{Storage: store, Graph: gb, Files: fb, Git: gr, Container: cm, ExtensionRoutes: routes}
	api := web.NewServer(web.Config{Port: 0, CORSOrigins: []string{}}, deps)

	req := httptest.NewRequest(http.MethodPost, "/api/log-echo", nil)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	// No assertion on log capture here; rely on go test output. The presence of
	// 'TEST_LOG: handler called' in the test output confirms visibility.
}
