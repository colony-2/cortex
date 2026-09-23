package cortex

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

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func TestTenantCellsAndRecipeJobs(t *testing.T) {
	for _, mode := range []string{"toy", "remote"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t), DefaultTenantID: "1"}
			var srv *Server
			var err error
			if mode == "remote" {
				jobdbServer := httptest.NewServer(remote.NewServer(toy.New()))
				t.Cleanup(jobdbServer.Close)
				cfg.JobDBURL = jobdbServer.URL
				srv, err = NewRemoteServer(cfg)
			} else {
				srv, err = NewUITestServer(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			testTenantCellsAndRecipeJobs(t, srv)
		})
	}
}

func testTenantCellsAndRecipeJobs(t *testing.T, srv *Server) {
	t.Helper()
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

	story := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/job-alpha-1/story")
	if story["job_id"] != "job-alpha-1" {
		t.Fatalf("story = %#v", story)
	}
	getJSON[[]interface{}](t, ts.URL+"/api/projects/42/user-inputs/pending")
	other := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/1/jobs")
	if jobs, ok := other["jobs"].([]interface{}); !ok || len(jobs) != 0 {
		t.Fatalf("jobs leaked across tenants: %#v", other)
	}
	// Schema registration is tenant-local, including after a prior submission.
	postJSON[map[string]interface{}](t, ts.URL+"/api/projects/1/jobs", map[string]interface{}{
		"recipe": "smoke", "job_id": "job-self-1",
	})
	for _, path := range []string{
		"/api/projects/1/jobs/job-alpha-1",
		"/api/projects/1/jobs/job-alpha-1/story",
		"/api/projects/1/jobs/job-alpha-1/outcome",
		"/api/projects/1/jobs/job-alpha-1/tasks/0/artifacts/missing",
	} {
		req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		doJSON[errorResponse](t, req, http.StatusNotFound)
	}
	missingRestart, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/1/jobs/job-alpha-1/restart", bytes.NewBufferString(`{"step_offset":0}`))
	if err != nil {
		t.Fatal(err)
	}
	doJSON[errorResponse](t, missingRestart, http.StatusNotFound)
	invalidRestart, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", bytes.NewBufferString(`{"step_offset":0}`))
	if err != nil {
		t.Fatal(err)
	}
	doJSON[errorResponse](t, invalidRestart, http.StatusBadRequest)
	// Seed a recorded outcome using the runtime API; Cortex itself runs no workers.
	ctx := context.Background()
	lease, err := srv.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{
		JobKey:   jobdb.JobKey{TenantId: "42", JobId: "job-alpha-1"},
		WorkerID: "cortex-test", Routes: []jobdb.Route{{JobType: "recipe"}},
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lease == nil {
		t.Fatal("expected a job lease")
	}
	defer lease.StopKeepAlive()
	artifact := jobdb.NewArtifactFromBytes("result.txt", []byte("test result"))
	err = lease.Complete(ctx, jobdb.CompleteExecutionRequest{
		Status: "completed",
		Chapter: &jobdb.Chapter{
			Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now().UTC(),
			Metadata: jobdb.ChapterMetadata{Fields: map[string]jobdb.ChapterMetadataValue{
				"attempt": {Kind: jobdb.ChapterMetadataInt, Int: 1},
			}},
			Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{
				Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"ok":true}`)},
			}},
		},
		ArtifactUploads: []jobdb.ArtifactUpload{{Name: artifact.Name(), Size: artifact.Size(), Open: artifact.Open}},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifactResponse, err := http.Get(ts.URL + "/api/projects/42/jobs/job-alpha-1/tasks/1/artifacts/result.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer artifactResponse.Body.Close()
	content, err := io.ReadAll(artifactResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if artifactResponse.StatusCode != http.StatusOK || string(content) != "test result" {
		t.Fatalf("artifact status=%d body=%s", artifactResponse.StatusCode, content)
	}
	newJob := postJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", map[string]interface{}{"step_offset": 1})
	newID, ok := newJob["job_id"].(string)
	if !ok || newID == "" || newID == "job-alpha-1" {
		t.Fatalf("restart = %#v", newJob)
	}
	getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/"+newID)
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
