package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/openapi"
	"github.com/colony-2/colony2/cli/internal/output"
)

// helper to build an App wired to a test server and buffer.
func setupTestApp(t *testing.T, handler http.Handler) (*App, *bytes.Buffer, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	// real client against the test server
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
		},
		Client:  cl,
		Printer: output.New("json", buf),
	}
	cleanup := func() { srv.Close() }
	return app, buf, cleanup
}

func TestProjectList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/projects" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"id":"p1","name":"proj1","gitRepoPath":"/tmp/repo1","createdAt":"2025-01-01T00:00:00Z","updatedAt":"2025-01-02T00:00:00Z"},
			{"id":"p2","name":"proj2","gitRepoPath":"/tmp/repo2","createdAt":"2025-01-03T00:00:00Z","updatedAt":"2025-01-04T00:00:00Z"}
		]`))
	})

	app, buf, cleanup := setupTestApp(t, handler)
	defer cleanup()

	cmd := newProjectListCmd()
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`"id": "p1"`)) || !bytes.Contains([]byte(got), []byte(`"id": "p2"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestProjectUpdate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/projects/p1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"defaultTicketRecipe":"recipe/a"`)) {
			http.Error(w, "missing default ticket recipe", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"p1","name":"new-name","gitRepoPath":"/tmp/repo","defaultTicketRecipe":"recipe/a","createdAt":"2025-01-01T00:00:00Z","updatedAt":"2025-01-05T00:00:00Z"}`))
	})

	app, buf, cleanup := setupTestApp(t, handler)
	defer cleanup()

	cmd := newProjectUpdateCmd()
	cmd.SetArgs([]string{"p1", "--name", "new-name", "--default-ticket-recipe", "recipe/a"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`"name": "new-name"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestProjectCreate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/projects" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p3","name":"proj3","gitRepoPath":"/tmp/repo3","createdAt":"2025-01-01T00:00:00Z","updatedAt":"2025-01-01T00:00:00Z"}`))
	})

	app, buf, cleanup := setupTestApp(t, handler)
	defer cleanup()

	cmd := newProjectCreateCmd()
	cmd.SetArgs([]string{"--name", "proj3", "--git-repo-path", "/tmp/repo3"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`"id": "p3"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestProjectDelete(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/projects/p4" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	app, buf, cleanup := setupTestApp(t, handler)
	defer cleanup()
	// text output ok for delete; keep JSON printer but expect plain text.
	cmd := newProjectDeleteCmd()
	cmd.SetArgs([]string{"p4"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); got != "deleted\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
