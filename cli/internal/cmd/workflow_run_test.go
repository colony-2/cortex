package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/output"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func TestWorkflowRun(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/projects/proj/workflows" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run_id":"run-123","recipe_name":"r1","status":"running","created_at":"2025-01-01T00:00:00Z"}`))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	cl, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	buf := &bytes.Buffer{}
	app := &App{
		Config: config.Config{
			APIURL:  srv.URL,
			Output:  "json",
			Timeout: 5 * time.Second,
			Project: "proj",
		},
		Client:  cl,
		Printer: output.New("json", buf),
	}

	cmd := newWorkflowRunCmd()
	cmd.SetArgs([]string{"--recipe", "r1", "--cell-id", "cell-1"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); !bytes.Contains([]byte(got), []byte(`"run_id": "run-123"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}
