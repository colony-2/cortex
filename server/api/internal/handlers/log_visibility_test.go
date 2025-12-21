package handlers_test

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/colony2/server/api/pkg/web"
	"github.com/colony-2/colony2/server/graph/pkg/graph"
)

// Test that log.Printf from HTTP handlers shows up in go test output
func TestLogVisibilityInHandlers(t *testing.T) {
	gb := graph.NewBuilder(".")

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

	deps := web.Dependencies{Graph: gb, ExtensionRoutes: routes}
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
