package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/output"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func setupWorkflowApp(t *testing.T, handler http.Handler) (*App, *bytes.Buffer, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
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
	cleanup := func() { srv.Close() }
	return app, buf, cleanup
}

func TestWorkflowOutputGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/projects/proj/workflows/w1":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"workflow_id":"w1",
				"run_id":"w1",
				"recipe_name":"r",
				"chapters":[
					{"chapter_number":1,"status":"completed","output":{"foo":"bar"}}
				],
				"status":"completed",
				"created_at":"2025-01-01T00:00:00Z"
			}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	app, buf, cleanup := setupWorkflowApp(t, handler)
	defer cleanup()

	cmd := newWorkflowOutputCmd()
	cmd.SetArgs([]string{"w1"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); !bytes.Contains([]byte(got), []byte(`"foo": "bar"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestWorkflowArtifactList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/projects/proj/jobs/w1/outcome" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"job_id":"w1",
				"status":"completed",
				"artifacts":[{"artifact_id":"a1","artifact_type":"text","name":"log.txt","created_at":"2025-01-01T00:00:00Z"}]
			}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	app, buf, cleanup := setupWorkflowApp(t, handler)
	defer cleanup()

	cmd := newWorkflowArtifactListCmd()
	cmd.SetArgs([]string{"w1"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); !bytes.Contains([]byte(got), []byte(`"name": "log.txt"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestWorkflowArtifactGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/projects/proj/jobs/w1/tasks/1/artifacts/log.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	app, _, cleanup := setupWorkflowApp(t, handler)
	defer cleanup()

	outPath := filepath.Join(t.TempDir(), "artifact.txt")
	cmd := newWorkflowArtifactGetCmd()
	cmd.SetArgs([]string{"w1", "--chapter", "1", "--name", "log.txt", "--output-file", outPath})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected artifact content: %s", string(data))
	}
}
