package cortex

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUITestServerTenantCellsAndRecipeJobs(t *testing.T) {
	root := writeC2JConfig(t)
	srv, err := NewUITestServer(Config{
		WorkingDir:      root,
		DefaultTenantID: "1",
	})
	if err != nil {
		t.Fatalf("NewUITestServer(): %v", err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()

	projects := getJSON[[]Project](t, ts.URL+"/api/projects")
	if len(projects) != 1 || projects[0].TenantID != "1" {
		t.Fatalf("projects = %#v, want tenant 1", projects)
	}

	cells := getJSON[[]Cell](t, ts.URL+"/api/projects/42/cells")
	if len(cells) != 3 {
		t.Fatalf("cells len = %d, want 3: %#v", len(cells), cells)
	}
	if cells[0].Kind != "self" || cells[0].TenantID != "42" || cells[0].Name != "self" {
		t.Fatalf("self cell = %#v", cells[0])
	}

	submitted := postJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs", map[string]interface{}{
		"cell":   "alpha",
		"recipe": "smoke",
		"inputs": map[string]interface{}{"x": float64(1)},
		"job_id": "job-alpha-1",
	})
	if submitted["job_id"] != "job-alpha-1" {
		t.Fatalf("submitted job_id = %#v", submitted["job_id"])
	}
	if submitted["tenant_id"] != "42" {
		t.Fatalf("submitted tenant_id = %#v", submitted["tenant_id"])
	}
	if submitted["cell_name"] != "alpha" {
		t.Fatalf("submitted cell_name = %#v", submitted["cell_name"])
	}

	job := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/job-alpha-1")
	if job["job_id"] != "job-alpha-1" || job["recipe"] != "smoke" || job["cell_name"] != "alpha" {
		t.Fatalf("job = %#v", job)
	}

	list := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs?cell=alpha&status=READY")
	jobs, ok := list["jobs"].([]interface{})
	if !ok || len(jobs) != 1 {
		t.Fatalf("list jobs = %#v", list)
	}
}

func TestRemoteServerRequiresJobDBURL(t *testing.T) {
	if _, err := NewRemoteServer(Config{WorkingDir: t.TempDir()}); err == nil {
		t.Fatal("NewRemoteServer() succeeded without JobDB URL")
	}
}

func writeC2JConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	configPath := filepath.Join(root, ".c2j", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := `
pattern: 'github.com/acme/boo-${{ cell }}'
dependents:
  - github.com/acme/boo-alpha
  - github.com/acme/boo-beta
self:
  repo: github.com/acme/boo-self
  ref: release
root:
  repo: root
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return root
}

func getJSON[T any](t *testing.T, url string) T {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	return doJSON[T](t, req, http.StatusOK)
}

func postJSON[T any](t *testing.T, url string, body interface{}) T {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal(): %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON[T](t, req, http.StatusCreated)
}

func doJSON[T any](t *testing.T, req *http.Request, wantStatus int) T {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		var errResp errorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("%s %s status = %d, want %d: %s", req.Method, req.URL.Path, resp.StatusCode, wantStatus, errResp.Error)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("Decode(): %v", err)
	}
	return out
}
