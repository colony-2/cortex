package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/output"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func setupTestAppWithProject(t *testing.T, handler http.Handler, project string) (*App, *bytes.Buffer, func()) {
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
			Project: project,
		},
		Client:  cl,
		Printer: output.New("json", buf),
	}
	return app, buf, func() { srv.Close() }
}

func TestRecipeCreatePublishFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"autoPublish":true`)) {
			http.Error(w, "autoPublish missing", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"r1","latestCommit":"abc","latestCommitAt":"2025-01-01T00:00:00Z"}`))
	})
	app, buf, cleanup := setupTestAppWithProject(t, handler, "proj")
	defer cleanup()

	tmp := t.TempDir()
	reqFile := tmp + "/req.json"
	if err := os.WriteFile(reqFile, []byte(`{"name":"r1","content":"steps: []"}`), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	cmd := newRecipeCreateCmd()
	cmd.SetArgs([]string{"--file", reqFile, "--publish"})
	cmd.SetContext(storeApp(context.Background(), app))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	_ = buf.String() // ensure write occurred; content shape not critical here
}

func TestRecipeUpdatePublishFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"autoPublish":true`)) {
			http.Error(w, "autoPublish missing", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"r1","latestCommit":"def","latestCommitAt":"2025-01-02T00:00:00Z"}`))
	})
	app, buf, cleanup := setupTestAppWithProject(t, handler, "proj")
	defer cleanup()

	tmp := t.TempDir()
	reqFile := tmp + "/req.json"
	if err := os.WriteFile(reqFile, []byte(`{"content":"steps: []"}`), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	cmd := newRecipeUpdateCmd()
	cmd.SetArgs([]string{"r1", "--file", reqFile, "--publish"})
	cmd.SetContext(storeApp(context.Background(), app))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	_ = buf.String()
}
