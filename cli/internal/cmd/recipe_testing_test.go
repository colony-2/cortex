package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/openapi"
	"github.com/colony-2/colony2/cli/internal/output"
)

func TestRecipeTestCompile(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	app, _, cleanup := setupTestAppWithProject(t, handler, "proj")
	defer cleanup()

	tmp := t.TempDir()
	suitePath := filepath.Join(tmp, "suite.yaml")
	outPath := filepath.Join(tmp, "compiled.json")
	suite := "cases:\n  - id: c1\n    type: recipe_case\n"
	if err := os.WriteFile(suitePath, []byte(suite), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	recipePath := filepath.Join(tmp, "recipe.yaml")
	if err := os.WriteFile(recipePath, []byte("version: '1.0'\nid: x\nop: input\ninputs:\n  form:\n    question: q\n    type: short_answer\n"), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	cmd := newRecipeTestCompileCmd()
	cmd.SetArgs([]string{"--recipe-file", recipePath, "--file", suitePath, "--out", outPath})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected compiled output: %v", err)
	}
}

func TestRecipeTestValidateCallsEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/projects/proj/recipe-tests/cases/validate" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"case"`)) {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"case_hash":"abc"}`))
	})
	app, _, cleanup := setupTestAppWithProject(t, handler, "proj")
	defer cleanup()

	tmp := t.TempDir()
	suitePath := filepath.Join(tmp, "suite.yaml")
	recipePath := filepath.Join(tmp, "recipe.yaml")
	_ = os.WriteFile(suitePath, []byte("cases:\n  - id: c1\n    type: recipe_case\n"), 0o644)
	_ = os.WriteFile(recipePath, []byte("version: '1.0'\nid: x\nop: input\ninputs:\n  form:\n    question: q\n    type: short_answer\n"), 0o644)

	cmd := newRecipeTestValidateCmd()
	cmd.SetArgs([]string{"--recipe-file", recipePath, "--file", suitePath})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestRecipeTestRunWritesArtifacts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/projects/proj/recipe-tests/cases/execute" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		resp := map[string]interface{}{
			"case_id": "c1",
			"status":  "passed",
			"artifacts": map[string]interface{}{
				"log.txt": map[string]interface{}{
					"content_base64": "aGVsbG8=",
					"size_bytes":     5,
					"truncated":      false,
				},
			},
		}
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()
	cl, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	buf := &bytes.Buffer{}
	app := &App{Config: config.Config{APIURL: srv.URL, Output: "json", Timeout: 5 * time.Second, Project: "proj"}, Client: cl, Printer: output.New("json", buf)}

	tmp := t.TempDir()
	suitePath := filepath.Join(tmp, "suite.yaml")
	recipePath := filepath.Join(tmp, "recipe.yaml")
	outDir := filepath.Join(tmp, "out")
	_ = os.WriteFile(suitePath, []byte("cases:\n  - id: c1\n    type: recipe_case\n"), 0o644)
	_ = os.WriteFile(recipePath, []byte("version: '1.0'\nid: x\nop: input\ninputs:\n  form:\n    question: q\n    type: short_answer\n"), 0o644)

	cmd := newRecipeTestRunCmd()
	cmd.SetArgs([]string{"--recipe-file", recipePath, "--file", suitePath, "--out-dir", outDir, "--artifact-mode", "inline"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "summary.json")); err != nil {
		t.Fatalf("summary missing: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, "cases", "c1", "artifacts", "log.txt"))
	if err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected artifact content %q", string(content))
	}
}
