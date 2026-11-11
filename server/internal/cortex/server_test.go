package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/jobdbschema"
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
				jobdbServer := httptest.NewServer(testjobdb.Handler(remote.NewServer(toy.New())))
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

	cells := getJSON[jobdb.ListCellsResponse](t, ts.URL+"/api/projects/42/cells")
	if len(cells.Cells) != 0 {
		t.Fatalf("config must not populate the registry: %+v", cells)
	}

	postJSON[jobdb.Cell](t, ts.URL+"/api/projects/42/cells", map[string]string{"name": "alpha", "repo": "github.com/acme/boo-alpha"})
	submitted := postJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs", map[string]interface{}{
		"cell": "alpha", "repo": "github.com/acme/boo-alpha",
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

	list := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs?cellNames=alpha&status=READY")
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
	postJSON[jobdb.Cell](t, ts.URL+"/api/projects/1/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
	postJSON[map[string]interface{}](t, ts.URL+"/api/projects/1/jobs", map[string]interface{}{
		"recipe": "smoke", "job_id": "job-self-1", "cell": "self", "repo": "github.com/acme/boo-self",
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
	missingRestart, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/1/jobs/job-alpha-1/restart", bytes.NewBufferString(`{"scope_id":"scope-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	doJSON[errorResponse](t, missingRestart, http.StatusNotFound)
	for _, body := range []string{`{}`, `{"scope_id":" "}`, `{"step_offset":1}`} {
		invalidRestart, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		doJSON[errorResponse](t, invalidRestart, http.StatusBadRequest)
	}
	// Seed a recorded outcome using the runtime API; Cortex itself runs no workers.
	ctx := testjobdb.Context(context.Background())
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
	// New restarts require a recorded execution header and a named scope.
	runtime := srv.engine.(jobdbschema.WorkflowEngine).Registry.(jobdb.WorkflowRuntime)
	leaseToken := ""
	if tokenLease, ok := lease.(interface{ LeaseToken() string }); ok {
		leaseToken = tokenLease.LeaseToken()
	}
	for i, record := range []jobdb.ExecutionRecord{
		{Version: jobdb.ExecutionPatternVersion, Kind: "header", Versions: &jobdb.ExecutionVersions{ExecutionPatternVersion: jobdb.ExecutionPatternVersion, JobDBVersion: "test"}},
		{Version: jobdb.ExecutionPatternVersion, Kind: "scope", ID: "scope-test", Descriptor: json.RawMessage(`{"restart":true}`)},
	} {
		// Compact history retains restartable scopes in a restart prefix.
		if record.Kind == "scope" {
			prefix, err := json.Marshal([]jobdb.ExecutionRecord{record})
			if err != nil {
				t.Fatal(err)
			}
			record = jobdb.ExecutionRecord{Version: jobdb.ExecutionPatternVersion, Kind: "restart", ID: "seed", Source: &jobdb.JobKey{TenantId: "42", JobId: "job-alpha-1"}, Descriptor: prefix}
		}
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		ordinal := int64(i + 1)
		if err := runtime.PutChapter(ctx, jobdb.PutChapterRequest{
			LeaseID: lease.LeaseID(), LeaseToken: leaseToken,
			Ref:     jobdb.ChapterRef{JobKey: jobdb.JobKey{TenantId: "42", JobId: "job-alpha-1"}, Ordinal: ordinal},
			Chapter: jobdb.Chapter{Ordinal: ordinal, TaskType: "__execution__", CreatedAt: time.Now().UTC(), Body: jobdb.ExecutionChapter{Record: raw}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	artifact := jobdb.NewArtifactFromBytes("result.txt", []byte("test result"))
	err = lease.Complete(ctx, jobdb.CompleteExecutionRequest{
		Status: "success",
		Detail: "Finished successfully",
		Chapter: &jobdb.Chapter{
			Ordinal: 3, TaskType: "recipe", CreatedAt: time.Now().UTC(),
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
	// Completion information must survive c2j projections and remote transport.
	completed := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/job-alpha-1")
	if completed["completion_status"] != "success" || completed["completion_detail"] != "Finished successfully" {
		t.Fatalf("completion fields missing: %#v", completed)
	}
	all := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs?cellNames=alpha&status=all")
	allJobs := all["jobs"].([]interface{})
	if len(allJobs) != 1 || allJobs[0].(map[string]interface{})["completion_status"] != "success" {
		t.Fatalf("all statuses must include completion data for archived jobs: %#v", all)
	}
	active := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs?cellNames=alpha&status=READY")
	if len(active["jobs"].([]interface{})) != 0 {
		t.Fatalf("ready filter included archived job: %#v", active)
	}
	artifactResponse, err := http.Get(ts.URL + "/api/projects/42/jobs/job-alpha-1/tasks/3/artifacts/result.txt")
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

	// Public ingress rejects malformed or unauthorized patches before submission.
	beforeInvalid := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs?status=all")
	for _, raw := range []string{
		`null`, `{}`, `{"job":{}}`, `true`, `1`, `"patch"`, `[{}]`,
		`[{"op":"unknown","path":"/context/git/author"}]`,
		`[{"op":"add","path":"/context/git/author"}]`,
		`[{"op":"remove","path":"/context/workspace"}]`,
		`[{"op":"remove","path":"/context/git/no_such_field"}]`,
		`[{"op":"copy","from":"/job_id","path":"/context/git/author"}]`,
		`[{"op":"copy","from":"/sequence/a/inputs/x","path":"/context/git/author"}]`,
		`[{"op":"add","path":"/sequence/a/outputs/b~2","value":1}]`,
		`[{"op":"add","path":"/context/git/author","value":123}]`,
	} {
		body := `{"request_id":"invalid","scope_id":"scope-test","goal":{"mode":"new"},"patch":` + raw + `}`
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		response := doJSON[errorResponse](t, req, http.StatusBadRequest)
		if response.Error == "" {
			t.Fatalf("no actionable error for %s", raw)
		}
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", bytes.NewBufferString(`{"request_id":"trailing","scope_id":"scope-test","goal":{"mode":"new"}} {}`))
	if err != nil {
		t.Fatal(err)
	}
	doJSON[errorResponse](t, req, http.StatusBadRequest)
	afterInvalid := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs?status=all")
	if len(beforeInvalid["jobs"].([]interface{})) != len(afterInvalid["jobs"].([]interface{})) {
		t.Fatal("invalid patch submitted a job")
	}
	for index, raw := range []string{"", `[]`, `[{"op":"add","path":"/sequence/a~1b/outputs/n","value":9007199254740993},{"op":"test","path":"/sequence/a~1b/outputs/n","value":9007199254740993},{"op":"copy","path":"/sequence/a~1b/outputs/c","from":"/sequence/a~1b/outputs/n"},{"op":"move","path":"/sequence/a~1b/outputs/m","from":"/sequence/a~1b/outputs/c"},{"op":"replace","path":"/sequence/a~1b/outputs/m","value":null},{"op":"remove","path":"/sequence/a~1b/outputs/m"}]`} {
		body := fmt.Sprintf(`{"request_id":"ordered-%d","scope_id":"scope-test","goal":{"mode":"new"}`, index)
		if raw != "" {
			body += `,"patch":` + raw
		}
		body += `}`
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		result := doJSON[map[string]interface{}](t, req, http.StatusCreated)
		for _, field := range []string{"job_id", "tenant_id", "goal_id"} {
			if result[field] == nil || result[field] == "" {
				t.Fatalf("missing %s: %#v", field, result)
			}
		}
		run, err := srv.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "42", JobId: result["job_id"].(string)}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range run.Execution {
			if entry.Record.Kind == "restart" && entry.Record.ID == "scope-test" {
				found = true
				if string(entry.Record.Payload) != raw {
					t.Fatalf("patch altered: got %s want %s", entry.Record.Payload, raw)
				}
			}
		}
		if !found {
			t.Fatal("restart record missing")
		}
	}
	newJob := postJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/job-alpha-1/restart", map[string]interface{}{"request_id": "restart-test", "goal": map[string]interface{}{"mode": "new"}, "scope_id": "scope-test", "patch": []map[string]interface{}{{"op": "add", "path": "/context/git/author", "value": "patched"}}})
	newID, ok := newJob["job_id"].(string)
	if !ok || newID == "" || newID == "job-alpha-1" {
		t.Fatalf("restart = %#v", newJob)
	}
	getJSON[map[string]interface{}](t, ts.URL+"/api/projects/42/jobs/"+newID)
	run, err := srv.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "42", JobId: newID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range run.Execution {
		if entry.Record.Kind == "restart" && entry.Record.ID == "scope-test" && string(entry.Record.Payload) == `[{"op":"add","path":"/context/git/author","value":"patched"}]` {
			return
		}
	}
	t.Fatalf("restart scope and patch missing from new job: %#v", run.Execution)
}

func TestRemoteServerRequiresJobDBURL(t *testing.T) {
	if _, err := NewRemoteServer(Config{WorkingDir: t.TempDir()}); err == nil {
		t.Fatal("NewRemoteServer() succeeded without JobDB URL")
	}
}

func writeC2JConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	configPath := filepath.Join(root, ".c2", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := `
pattern: 'github.com/acme/boo-${{ cell }}'
dependents:
  - github.com/acme/boo-alpha
  - github.com/acme/boo-beta
self:
  cell: self
  repo: github.com/acme/boo-self
  ref: release
root:
  repo: github.com/acme/root
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

// Explicit IDs remain opaque across the browser's transition to backend allocation.
func TestExplicitJobIDCompatibility(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/compat"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "platform", "repo": "github.com/acme/platform"})
	for _, multipartBody := range []bool{false, true} {
		for _, id := range []string{"1fac9e89-36cf-4a4f-a797-7d4fe887c65c", "3LfLqNNl2VVLo8Cg5a032Lm0igK", "custom-ID"} {
			t.Run(fmt.Sprintf("%t/%s", multipartBody, id), func(t *testing.T) {
				// Separate tenants allow the same ID to exercise both transports.
				tenantBase := base
				if multipartBody {
					tenantBase = api.URL + "/api/projects/compat-files"
				}
				if multipartBody && id == "1fac9e89-36cf-4a4f-a797-7d4fe887c65c" {
					postJSON[jobdb.Cell](t, tenantBase+"/cells", map[string]string{"name": "platform", "repo": "github.com/acme/platform"})
				}
				input := map[string]any{"cell": "platform", "job_id": id, "inputs": map[string]any{"prompt": "hello", "type": "build"}}
				raw, _ := json.Marshal(input)
				var req *http.Request
				if multipartBody {
					var body bytes.Buffer
					w := multipart.NewWriter(&body)
					_ = w.WriteField("submission", string(raw))
					part, e := w.CreateFormFile("files", "brief")
					if e != nil {
						t.Fatal(e)
					}
					_, _ = io.WriteString(part, "hello")
					_ = w.Close()
					req, _ = http.NewRequest(http.MethodPost, tenantBase+"/jobs", &body)
					req.Header.Set("Content-Type", w.FormDataContentType())
				} else {
					req, _ = http.NewRequest(http.MethodPost, tenantBase+"/jobs", bytes.NewReader(raw))
					req.Header.Set("Content-Type", "application/json")
				}
				submitted := doJSON[map[string]any](t, req, http.StatusCreated)
				if submitted["job_id"] != id {
					t.Fatalf("identity changed: %#v", submitted)
				}
				for _, suffix := range []string{"", "/story"} {
					job := getJSON[map[string]any](t, tenantBase+"/jobs/"+id+suffix)
					if job["job_id"] != id {
						t.Fatalf("read identity changed: %#v", job)
					}
				}
				archive, _ := http.NewRequest(http.MethodDelete, tenantBase+"/jobs/"+id, nil)
				response, e := http.DefaultClient.Do(archive)
				if e != nil {
					t.Fatal(e)
				}
				response.Body.Close()
				if response.StatusCode >= 300 {
					t.Fatalf("archive: %d", response.StatusCode)
				}
				job := getJSON[map[string]any](t, tenantBase+"/jobs/"+id)
				if job["job_id"] != id {
					t.Fatalf("archived identity changed: %#v", job)
				}
			})
		}
	}
}
