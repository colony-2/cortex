package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	workflow "github.com/colony-2/jobdb/pkg/workflow"
)

func TestCancellationReasonValidation(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
			if err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewServer(srv)
			defer ts.Close()
			postJSON[jobdb.Cell](t, ts.URL+"/api/projects/a/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
			cases := []struct {
				body   string
				status int
				reason string
			}{
				{"", 204, defaultCancellationReason}, {"{}", 204, defaultCancellationReason}, {`{"other":1}`, 204, defaultCancellationReason},
				{`{"reason":"   \n "}`, 204, defaultCancellationReason}, {`{"reason":"  雪🧪\n<script>text</script>  "}`, 204, "雪🧪\n<script>text</script>"},
				{`{"reason":null}`, 400, ""}, {`{"reason":1}`, 400, ""}, {`{"reason":true}`, 400, ""}, {`{"reason":[]}`, 400, ""}, {`{"reason":{}}`, 400, ""},
				{`{"reason":`, 400, ""}, {`{} {}`, 400, ""}, {`null`, 400, ""}, {`[]`, 400, ""},
				{`{"reason":"` + strings.Repeat("🧪", 4000) + `"}`, 204, strings.Repeat("🧪", 4000)},
				{`{"reason":"` + strings.Repeat("🧪", 4001) + `"}`, 400, ""},
				{`{"reason":"` + strings.Repeat("a", 65536) + `"}`, 413, ""},
			}
			for i, tc := range cases {
				id := "case-" + strings.Repeat("x", i+1)
				postJSON[map[string]interface{}](t, ts.URL+"/api/projects/a/jobs", map[string]interface{}{"job_id": id, "recipe": "smoke", "cell": "self", "repo": "github.com/acme/boo-self"})
				path := ts.URL + "/api/projects/a/jobs/" + id
				if method == http.MethodPost {
					path += "/cancel"
				}
				req, _ := http.NewRequest(method, path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != tc.status {
					t.Fatalf("case %d status = %d, want %d", i, resp.StatusCode, tc.status)
				}
				run, err := srv.engine.GetJobRun(testjobdb.Context(context.Background()), jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: id}})
				if err != nil {
					t.Fatal(err)
				}
				if tc.status != 204 {
					if run.Job.Status != jobdb.JobStatusReady {
						t.Fatalf("invalid request mutated job: %#v", run.Job)
					}
					continue
				}
				want := tc.reason
				if method == http.MethodDelete && !strings.Contains(tc.body, `"reason"`) {
					want = "job deleted"
				}
				if run.Job.CompletionDetail != want {
					t.Fatalf("case %d persisted detail = %q, want %q", i, run.Job.CompletionDetail, want)
				}
			}
		})
	}
}

type failingCancellationEngine struct {
	workflow.Engine
	calls int
	fail  bool
}

func (e *failingCancellationEngine) CancelJob(ctx context.Context, req jobdb.CancelJob) error {
	e.calls++
	if e.fail {
		return errors.New("injected cancellation failure")
	}
	return e.Engine.CancelJob(ctx, req)
}

type failingArchiveRuntime struct {
	jobdb.JobDeletionRuntime
	calls int
	fail  bool
}

func (d *failingArchiveRuntime) DeleteJobs(ctx context.Context, req jobdb.DeleteJobsRequest) (jobdb.DeleteJobsResponse, error) {
	d.calls++
	if d.fail {
		return jobdb.DeleteJobsResponse{}, errors.New("injected archive failure")
	}
	return d.JobDeletionRuntime.DeleteJobs(ctx, req)
}
func TestCancellationArchivePartialFailure(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	postJSON[jobdb.Cell](t, ts.URL+"/api/projects/a/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
	postJSON[map[string]interface{}](t, ts.URL+"/api/projects/a/jobs", map[string]interface{}{"job_id": "job", "recipe": "smoke", "cell": "self", "repo": "github.com/acme/boo-self"})
	engine := &failingCancellationEngine{Engine: srv.engine, fail: true}
	deletion := &failingArchiveRuntime{JobDeletionRuntime: srv.jobDeletion, fail: true}
	srv.engine = engine
	srv.jobDeletion = deletion
	archive := func(reason string) (int, errorResponse) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"reason": reason})
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/projects/a/jobs/job", strings.NewReader(string(body)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var result errorResponse
		if resp.StatusCode != 204 {
			json.NewDecoder(resp.Body).Decode(&result)
		}
		return resp.StatusCode, result
	}
	if status, _ := archive("first"); status != 500 || deletion.calls != 0 {
		t.Fatalf("failed cancellation deleted: %d, %d", status, deletion.calls)
	}
	engine.fail = false
	if status, result := archive("accepted 🧪"); status != 500 || result.Code != "archive_failed_after_cancel" {
		t.Fatalf("partial result: %d %#v", status, result)
	}
	// This is a fresh HTTP request, with no browser state carrying the reason.
	deletion.fail = false
	if status, _ := archive("replacement after reload"); status != 204 {
		t.Fatal(status)
	}
	if engine.calls != 2 || deletion.calls != 2 {
		t.Fatalf("retry repeated successful cancellation: %d / %d", engine.calls, deletion.calls)
	}
	run, err := engine.GetJobRun(testjobdb.Context(context.Background()), jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: "job"}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Job.CompletionDetail != "accepted 🧪" {
		t.Fatalf("accepted reason overwritten: %#v", run.Job)
	}
}
