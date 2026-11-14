package cortex

import (
	"context"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func TestJobActions(t *testing.T) {
	for _, mode := range []string{"toy", "remote"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t)}
			var srv *Server
			var err error
			if mode == "remote" {
				backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(toy.New())))
				t.Cleanup(backend.Close)
				cfg.JobDBURL = backend.URL
				srv, err = NewRemoteServer(cfg)
			} else {
				srv, err = NewUITestServer(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewServer(srv)
			defer ts.Close()
			for _, tenant := range []string{"a", "b"} {
				postJSON[jobdb.Cell](t, ts.URL+"/api/projects/"+tenant+"/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
				for _, id := range []string{"first", "second", "untouched"} {
					postJSON[map[string]interface{}](t, ts.URL+"/api/projects/"+tenant+"/jobs", map[string]interface{}{"job_id": id, "recipe": "smoke", "cell": "self", "repo": "github.com/acme/boo-self"})
				}
			}
			action := func(method, path string, want int) {
				t.Helper()
				req, err := http.NewRequest(method, ts.URL+"/api/projects/"+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != want {
					t.Fatalf("%s %s: status %d, want %d", method, path, resp.StatusCode, want)
				}
			}
			for _, tenant := range []string{"missing", "a"} {
				id := "first"
				if tenant == "a" {
					id = "missing"
				}
				action(http.MethodPost, tenant+"/jobs/"+id+"/cancel", http.StatusNotFound)
				action(http.MethodDelete, tenant+"/jobs/"+id, http.StatusNotFound)
			}
			action(http.MethodPost, "a/jobs/first/cancel", http.StatusNoContent)
			action(http.MethodPost, "a/jobs/first/cancel", http.StatusNoContent)
			cancelled := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/a/jobs/first")
			if cancelled["status"] != "CANCELLED" {
				t.Fatalf("cancelled job = %#v", cancelled)
			}
			// Deletion hides both archived and unfinished jobs; JobDB retains history.
			for _, id := range []string{"first", "second"} {
				action(http.MethodDelete, "a/jobs/"+id, http.StatusNoContent)
				job, err := srv.engine.GetJobRun(testjobdb.Context(context.Background()), jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: id}})
				if err != nil {
					t.Fatal(err)
				}
				if job.Job.Status != jobdb.JobStatusCancelled {
					t.Fatalf("deleted job = %#v", job)
				}
				archived := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/a/jobs/"+id)
				if archived["status"] != "CANCELLED" || archived["completion_detail"] != job.Job.CompletionDetail {
					t.Fatalf("archived detail unavailable or changed: %#v", archived)
				}
				other := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/b/jobs/"+id)
				if other["status"] != "READY" {
					t.Fatalf("other tenant changed: %#v", other)
				}
			}
			list := getJSON[struct {
				Jobs []struct {
					ID string `json:"job_id"`
				} `json:"jobs"`
			}](t, ts.URL+"/api/projects/a/jobs?status=all")
			if len(list.Jobs) != 1 || list.Jobs[0].ID != "untouched" {
				t.Fatalf("remaining jobs = %#v", list)
			}
			srv.jobDeletion = nil
			action(http.MethodDelete, "a/jobs/untouched", http.StatusNotImplemented)
		})
	}
}

// Exercise the actual router with real generic jobs in queued and leased states.
func TestGenericJobCancellationHTTP(t *testing.T) {
	for _, mode := range []string{"toy", "remote"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t)}
			var srv *Server
			var err error
			if mode == "remote" {
				backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(toy.New())))
				defer backend.Close()
				cfg.JobDBURL = backend.URL
				srv, err = NewRemoteServer(cfg)
			} else {
				srv, err = NewUITestServer(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			api := httptest.NewServer(srv)
			defer api.Close()
			ctx := testjobdb.Context(context.Background())
			for _, tenant := range []string{"a", "b"} {
				postJSON[jobdb.Cell](t, api.URL+"/api/projects/"+tenant+"/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
				for _, id := range []string{"ready", "active", "hidden"} {
					_, err := srv.engine.SubmitJob(ctx, jobdb.SubmitJob{TenantId: tenant, JobID: id, Cell: "self", CellRepo: "github.com/acme/boo-self", JobType: "generic", Data: jobdb.NewTaskDataOrPanic(1)})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			lease, err := srv.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: "active"}, WorkerID: "cancel-test", Routes: []jobdb.Route{{JobType: "generic"}}, LeaseDuration: time.Minute})
			if err != nil || lease == nil {
				t.Fatalf("lease %v %v", lease, err)
			}
			defer lease.StopKeepAlive()
			for _, id := range []string{"ready", "active"} {
				before := getJSON[struct {
					Node treeNode `json:"node"`
				}](t, api.URL+"/api/projects/a/jobs/"+id+"/tree-node").Node
				want := jobdb.JobStatusReady
				if id == "active" {
					want = jobdb.JobStatusActive
				}
				if before.Status != want {
					t.Fatalf("fixture %s: %+v", id, before)
				}
				req, _ := http.NewRequest("POST", api.URL+"/api/projects/a/jobs/"+id+"/cancel", strings.NewReader(`{"reason":" Selected 雪 job "}`))
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 204 {
					t.Fatalf("cancel %s: %d", id, response.StatusCode)
				}
				fresh := getJSON[struct {
					Node treeNode `json:"node"`
				}](t, api.URL+"/api/projects/a/jobs/"+id+"/tree-node").Node
				if fresh.CompletionStatus != "cancelled" || fresh.ArchivedAt == nil || fresh.Store != jobdb.JobStoreArchived {
					t.Fatalf("persisted cancellation: %+v", fresh)
				}
				run, err := srv.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: id}})
				if err != nil || run.Job.CompletionDetail != "Selected 雪 job" {
					t.Fatalf("reason: %+v %v", run, err)
				}
				other := getJSON[struct {
					Node treeNode `json:"node"`
				}](t, api.URL+"/api/projects/b/jobs/"+id+"/tree-node").Node
				if other.Status != jobdb.JobStatusReady || other.CancelRequested {
					t.Fatalf("tenant escaped: %+v", other)
				}
			}
			// Generic jobs remain outside recipe-only mutation routes.
			response, err := http.Post(api.URL+"/api/projects/b/jobs/ready/restart", "application/json", strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 404 {
				t.Fatal(response.StatusCode)
			}
			_, err = srv.jobDeletion.DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: "a", JobIDs: []string{"hidden"}})
			if err != nil {
				t.Fatal(err)
			}
			getJSON[struct {
				Node treeNode `json:"node"`
			}](t, api.URL+"/api/projects/a/jobs/hidden/tree-node")
			for _, path := range []string{"a/jobs/hidden", "a/jobs/missing", "missing/jobs/ready"} {
				response, err := http.Post(api.URL+"/api/projects/"+path+"/cancel", "application/json", nil)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 404 {
					t.Fatalf("%s %d", path, response.StatusCode)
				}
			}
		})
	}
}

type cancellationGuardEngine struct {
	*compactListEngine
	denied bool
	t      *testing.T
}

func (e cancellationGuardEngine) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	if ctx.Value(treeAuthorityKey{}) != "caller" {
		e.t.Fatal("caller context lost")
	}
	if e.denied {
		return jobdb.ListJobsResponse{}, jobdb.ErrLeasePermissionDenied
	}
	return e.compactListEngine.ListJobs(ctx, req)
}
func TestCancellationGuardIdentityAndAccess(t *testing.T) {
	for _, scenario := range []string{"match", "foreign-tenant", "wrong-job", "absent", "denied"} {
		t.Run(scenario, func(t *testing.T) {
			jobs := compactJobs(1)
			jobs[0].JobType = "generic"
			if scenario == "foreign-tenant" {
				jobs[0].JobKey.TenantId = "other"
			}
			if scenario == "wrong-job" {
				jobs[0].JobKey.JobId = "neighbor"
			}
			if scenario == "absent" {
				jobs = nil
			}
			e := cancellationGuardEngine{compactListEngine: &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs}}}, denied: scenario == "denied", t: t}
			s := &Server{engine: e}
			called := false
			request := mux.SetURLVars(httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(context.Background(), treeAuthorityKey{}, "caller")), map[string]string{"projectId": "tenant", "jobId": "0"})
			w := httptest.NewRecorder()
			s.requireCancellableJob(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) })(w, request)
			if called != (scenario == "match") {
				t.Fatalf("unexpected mutation for %s", scenario)
			}
			if scenario == "denied" {
				if w.Code < 400 {
					t.Fatal(w.Code)
				}
				return
			}
			if scenario != "match" && w.Code != 404 {
				t.Fatal(w.Code)
			}
			q := e.requests[0]
			if len(q.TenantIds) != 1 || q.TenantIds[0] != "tenant" || len(q.JobKeys) != 1 || q.JobKeys[0] != (jobdb.JobKey{TenantId: "tenant", JobId: "0"}) || q.IncludeHidden || len(q.Stores) != 2 || len(q.JobTypes) != 0 || len(q.Statuses) != 0 || q.PageSize != 1 {
				t.Fatalf("guard query %+v", q)
			}
		})
	}
}
