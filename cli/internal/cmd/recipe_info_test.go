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

func TestRecipeInfo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/projects/proj/recipes/r1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"name":"r1",
			"commitHash":"abc",
			"isPublished":true,
			"content":{},
			"rawYaml":"steps: []",
			"publishedAt":"2025-01-01T00:00:00Z"
		}`))
	})
	mux.HandleFunc("/api/projects/proj/recipes/r1/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"versions":[{"author":"a","commitHash":"abc","createdAt":"2025-01-01T00:00:00Z","isPublished":true,"message":"init","shortHash":"abc"}]}`))
	})

	srv := httptest.NewServer(mux)
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

	cmd := newRecipeInfoCmd()
	cmd.SetArgs([]string{"r1"})
	cmd.SetContext(storeApp(context.Background(), app))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); !bytes.Contains(buf.Bytes(), []byte(`"publishedCommit": "abc"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}
