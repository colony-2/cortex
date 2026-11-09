package cortex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/execution"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/gorilla/mux"
)

// GS-001–005, GS-020: a real Cortex -> remote JobDB -> SQLite round trip.
func TestRootScopeSQLiteHTTP(t *testing.T) {
	ctx := celltest.Context(context.Background())
	path := filepath.Join(t.TempDir(), "roots.db")
	runtime, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(runtime)))
	defer backend.Close()
	srv, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	submit := func(id, tenant, cell, metadata string) jobdb.JobHandle {
		t.Helper()
		h, err := celltest.Submit(runtime, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, Cell: cell, CellRepo: "example.test/" + cell, JobID: id, JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(nil), Metadata: json.RawMessage(metadata)}})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	parent := submit("parent", "tenant", "alpha", `{}`)
	lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: parent.JobKey, WorkerID: "roots", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
	if err != nil || lease == nil {
		t.Fatalf("lease %v %v", lease, err)
	}
	defer lease.StopKeepAlive()
	child, err := lease.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", Cell: "alpha", JobID: "child", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(nil), Metadata: json.RawMessage(`{"parent_job_id":"parent","parent_tenant_id":"tenant"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	childLease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: child.JobKey, WorkerID: "child", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
	if err != nil || childLease == nil {
		t.Fatalf("child lease %v %v", childLease, err)
	}
	complete := func(lease jobdb.ExecutionLease, status string) {
		t.Helper()
		err := lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: status, Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"ok":true}`)}}}}})
		lease.StopKeepAlive()
		if err != nil {
			t.Fatal(err)
		}
	}
	complete(childLease, "failed_app")
	restart := func(id string, leased bool) {
		t.Helper()
		req := jobdb.SubmitRestartJobRequest{Job: jobdb.SubmitRestartJob{Cell: "alpha", JobID: id, PriorJobKey: child.JobKey, ShareGoal: !leased}}
		var err error
		if leased {
			_, err = lease.SubmitRestartJob(ctx, req)
		} else {
			_, err = runtime.SubmitRestartJob(ctx, req)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	restart("direct", false)
	restart("leased", true)
	for _, id := range []string{"root", "dangling", "legacy-root", "legacy-child", "legacy-dangling", "explicit-empty", "hidden", "archived"} {
		submit(id, "tenant", "alpha", `{}`)
	}
	submit("beta", "tenant", "beta", `{}`)
	submit("other-tenant", "other", "alpha", `{}`)
	// Simulate persisted older rows using the same public SQLite schema; no
	// migration/reconstruction or alternate implementation of RootOnly.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`UPDATE jobdb_jobs SET parent_job_id='missing' WHERE job_id='dangling'`,
		`UPDATE jobdb_jobs SET parent_job_id=NULL, metadata='{"app":{},"internal":{}}' WHERE job_id='legacy-root'`,
		`UPDATE jobdb_jobs SET parent_job_id=NULL, metadata='{"app":{},"internal":{"parentJobId":"parent"}}' WHERE job_id='legacy-child'`,
		`UPDATE jobdb_jobs SET parent_job_id=NULL, metadata='{"app":{},"internal":{"parentJobId":"missing"}}' WHERE job_id='legacy-dangling'`,
		`UPDATE jobdb_jobs SET parent_job_id='', metadata='{"app":{},"internal":{"parentJobId":"parent"}}' WHERE job_id='explicit-empty'`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: "tenant", JobIDs: []string{"hidden"}}); err != nil {
		t.Fatal(err)
	}
	archiveLease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "archived"}, WorkerID: "archive", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
	if err != nil || archiveLease == nil {
		t.Fatalf("archive lease %v %v", archiveLease, err)
	}
	complete(archiveLease, "success")
	list := func(query string) []recipejob.RecipeJob {
		t.Helper()
		var jobs []recipejob.RecipeJob
		token := ""
		seen := map[string]bool{}
		for i := 0; ; i++ {
			if i > 30 {
				t.Fatal("cursor loop")
			}
			page := getJSON[recipejob.ListRecipeJobsResponse](t, api.URL+"/api/projects/tenant/jobs?pageSize=1&"+query+"&pageToken="+url.QueryEscape(token))
			for _, job := range page.Jobs {
				if seen[job.JobID] {
					t.Fatal("duplicate", job.JobID)
				}
				seen[job.JobID] = true
				jobs = append(jobs, job)
			}
			if page.NextPageToken == "" {
				break
			}
			token = page.NextPageToken
		}
		return jobs
	}
	ids := func(jobs []recipejob.RecipeJob) []string {
		out := []string{}
		for _, j := range jobs {
			out = append(out, j.JobID)
		}
		sort.Strings(out)
		return out
	}
	all := list("status=all")
	if !reflect.DeepEqual(ids(all), ids(list("status=all&rootOnly=false"))) {
		t.Fatal("false changed All goals")
	}
	if !reflect.DeepEqual(ids(all), []string{"archived", "beta", "child", "dangling", "direct", "explicit-empty", "leased", "legacy-child", "legacy-dangling", "legacy-root", "parent", "root"}) {
		t.Fatalf("all jobs: %v", ids(all))
	}
	roots := list("status=all&rootOnly=true")
	want := []string{"archived", "beta", "direct", "explicit-empty", "legacy-root", "parent", "root"}
	if !reflect.DeepEqual(ids(roots), want) {
		t.Fatalf("roots=%v want=%v", ids(roots), want)
	}
	for _, job := range roots {
		if job.JobID == "direct" && (job.Parent == nil || job.Parent.JobID != "parent") {
			t.Fatalf("restart metadata lost: %+v", job)
		}
	}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"rootOnly=true&status=all&includeHidden=true&cellNames=alpha", []string{"archived", "direct", "explicit-empty", "hidden", "legacy-root", "parent", "root"}},
		{"rootOnly=true&displayStatus=success", []string{"archived"}},
		{"rootOnly=true&status=READY&cellNames=beta", []string{"beta"}},
		{"rootOnly=true&status=READY&goal=" + child.GoalID, []string{"direct"}},
	} {
		if got := ids(list(tc.query)); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v want %v", tc.query, got, tc.want)
		}
	}
}

func TestRootScopeValidationAndBackendErrors(t *testing.T) {
	for _, query := range []string{"rootOnly=", "rootOnly=1", "rootOnly=True", "rootOnly=no", "rootOnly=true&rootOnly=true", "rootOnly=true&rootOnly=false"} {
		w := httptest.NewRecorder()
		(&Server{}).handleListJobs(w, httptest.NewRequest("GET", "/jobs?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", query, w.Code)
		}
	}
	engine := &rootErrorEngine{}
	w := httptest.NewRecorder()
	// Embed an engine so all non-list operations would panic.
	srv := &Server{engine: engine}
	srv.handleListJobs(w, mux.SetURLVars(httptest.NewRequest("GET", "/jobs?rootOnly=true", nil), map[string]string{"projectId": "tenant"}))
	if w.Code < 500 || engine.calls != 1 {
		t.Fatalf("error broadened: %d calls=%d %s", w.Code, engine.calls, w.Body)
	}
}

type rootErrorEngine struct {
	compactListEngine
	calls int
}

func (e *rootErrorEngine) ListJobs(_ context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.calls++
	if !req.RootOnly {
		panic("root scope lost")
	}
	return jobdb.ListJobsResponse{}, fmt.Errorf("root filtering unsupported")
}

func TestRootScopeCompactAdapter(t *testing.T) {
	for _, roots := range []bool{false, true} {
		jobs := compactJobs(2)
		jobs[1].JobType = "unrelated"
		engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs, NextPageToken: "opaque"}}}
		req := recipejob.ListRecipeJobsRequest{TenantID: "tenant", PageSize: 2, Stores: []jobdb.JobStore{jobdb.JobStoreArchived}}
		page, err := listScopedRecipeJobs(context.Background(), engine, req, roots)
		if err != nil || len(page.Jobs) != 1 || page.NextPageToken != "opaque" {
			t.Fatalf("page=%+v err=%v", page, err)
		}
		if page.Jobs[0].Prompt != "Build it" || page.Jobs[0].ClientPayloadRevision != 8 || page.Jobs[0].BlockingWait != nil || len(page.Jobs[0].OtherGoalIDs) != 0 {
			t.Fatalf("compact projection lost: %+v", page.Jobs[0])
		}
		r := engine.requests[0]
		if r.RootOnly != roots || len(r.ParentJobIDs) != 0 || !reflect.DeepEqual(r.Stores, req.Stores) {
			t.Fatalf("projection/filter: %+v", r)
		}
		req.ExecutionFilter = &execution.Filter{IncludeUnresolved: true}
		if _, err := listScopedRecipeJobs(context.Background(), engine, req, roots); err == nil || len(engine.requests) != 1 {
			t.Fatal("execution filter must reject before listing")
		}
		req.ExecutionFilter = nil
		jobs[0].Metadata = json.RawMessage(`{"prompt":42}`)
		if _, err := listScopedRecipeJobs(context.Background(), engine, req, roots); err == nil {
			t.Fatal("conversion error suppressed")
		}
	}
}
