package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/gorilla/mux"
)

func TestDisplayStatusFiltersMatchVisibleStates(t *testing.T) {
	archived := time.Now()
	jobs := compactJobs(0)
	add := func(id string, status jobdb.JobStatus, outcome string, archive, cancel bool) {
		job := compactJobs(1)[0]
		job.JobKey.JobId, job.Status, job.CompletionStatus, job.CancelRequested = id, status, outcome, cancel
		if archive {
			job.ArchivedAt = &archived
		}
		jobs = append(jobs, job)
	}
	for _, outcome := range []string{"success", "failed_app", "failed_system", "failed_timeout", "cancelled", "", "future_outcome"} {
		add("completed-"+outcome, jobdb.JobStatusCompleted, outcome, true, true)
	}
	add("ready", jobdb.JobStatusReady, "", false, false)
	add("retrying", jobdb.JobStatusReady, "failed_app", false, false)
	add("running", jobdb.JobStatusActive, "failed_system", false, false)
	add("scheduled", jobdb.JobStatusAwaitingFuture, "failed_timeout", false, false)
	add("request-ready", jobdb.JobStatusReady, "failed_app", false, true)
	add("request-cancelled", jobdb.JobStatusCancelled, "", false, true)
	add("archived-cancelled", jobdb.JobStatusCancelled, "", true, true)
	add("archived-outcome", jobdb.JobStatusCancelled, "failed_app", true, true)
	add("completed-active", jobdb.JobStatusCompleted, "success", false, true)
	add("expired", jobdb.JobStatusExpired, "", false, false)
	add("waiting", jobdb.JobStatusPendingJobs, "", false, false)
	add("unresponsive", jobdb.JobStatusCrashConcern, "", false, false)
	for _, tc := range []struct {
		statuses []string
		want     []string
	}{
		{[]string{"success"}, []string{"completed-success", "completed-active"}},
		{[]string{"failed_app"}, []string{"completed-failed_app", "archived-outcome"}},
		{[]string{"failed_system"}, []string{"completed-failed_system"}},
		{[]string{"failed_timeout"}, []string{"completed-failed_timeout"}},
		{[]string{"CANCELLED"}, []string{"completed-cancelled", "archived-cancelled"}},
		{[]string{"CANCEL_REQUESTED"}, []string{"request-ready", "request-cancelled"}},
		{[]string{"OUTCOME_UNAVAILABLE"}, []string{"completed-"}},
		{[]string{"completion:future_outcome"}, []string{"completed-future_outcome"}},
		{[]string{"READY"}, []string{"ready", "retrying"}},
		{[]string{"ACTIVE"}, []string{"running"}},
		{[]string{"AWAITING_FUTURE"}, []string{"scheduled"}},
		{[]string{"EXPIRED"}, []string{"expired"}},
		{[]string{"PENDING_JOBS"}, []string{"waiting"}},
		{[]string{"CRASH_CONCERN"}, []string{"unresponsive"}},
		{[]string{"READY", "failed_app", "failed_app"}, []string{"completed-failed_app", "ready", "retrying", "archived-outcome"}},
	} {
		t.Run(fmt.Sprint(tc.statuses), func(t *testing.T) {
			engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs}}}
			query := url.Values{"displayStatus": tc.statuses}
			page := displayStatusPage(t, &Server{engine: engine}, query)
			var ids []string
			for _, job := range page.Jobs {
				ids = append(ids, job.JobID)
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("got %v, want %v", ids, tc.want)
			}
			if len(engine.requests) != 1 || len(engine.requests[0].Statuses) != 0 || len(engine.requests[0].Stores) != 2 {
				t.Fatalf("display filters must consider both stores: %+v", engine.requests)
			}
		})
	}
}

func displayStatusPage(t *testing.T, server *Server, query url.Values) recipejob.ListRecipeJobsResponse {
	t.Helper()
	w := httptest.NewRecorder()
	server.handleListJobs(w, mux.SetURLVars(httptest.NewRequest("GET", "/jobs?"+query.Encode(), nil), map[string]string{"projectId": "tenant"}))
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var page recipejob.ListRecipeJobsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestDisplayStatusPaginationAcrossEmptyPages(t *testing.T) {
	jobs := compactJobs(3)
	for i := range jobs {
		jobs[i].Status, jobs[i].CompletionStatus = jobdb.JobStatusCompleted, "failed_app"
	}
	engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{
		"":          {Jobs: compactJobs(2), NextPageToken: "empty"},
		"empty":     {NextPageToken: "first"},
		"first":     {Jobs: jobs[:1], NextPageToken: "second"},
		"second":    {Jobs: jobs[1:2], NextPageToken: "cursor+/="},
		"cursor+/=": {Jobs: jobs[2:]},
	}}
	server := &Server{engine: engine}
	query := url.Values{"displayStatus": {"failed_app"}, "pageSize": {"2"}, "goal": {"goal"}}
	first := displayStatusPage(t, server, query)
	if len(first.Jobs) != 2 || first.Jobs[0].JobID != "0" || first.Jobs[1].JobID != "1" || first.NextPageToken != "cursor+/=" {
		t.Fatalf("first page: %+v", first)
	}
	query.Set("pageToken", first.NextPageToken)
	last := displayStatusPage(t, server, query)
	if len(last.Jobs) != 1 || last.Jobs[0].JobID != "2" || last.NextPageToken != "" {
		t.Fatalf("last page: %+v", last)
	}
	for i, req := range engine.requests {
		if !reflect.DeepEqual(req.TenantIds, []string{"tenant"}) || !reflect.DeepEqual(req.GoalIDs, []string{"goal"}) {
			t.Fatalf("lost scope on request %d: %+v", i, req)
		}
	}
	if engine.requests[3].PageSize != 1 {
		t.Fatal("must limit backend page to remaining result capacity")
	}
}

func TestDisplayStatusScanLimitPreservesContinuation(t *testing.T) {
	engine := &compactListEngine{pages: make(map[string]jobdb.ListJobsResponse)}
	token := ""
	for i := 0; i < maxDisplayStatusPages; i++ {
		next := fmt.Sprint(i)
		engine.pages[token] = jobdb.ListJobsResponse{Jobs: compactJobs(1), NextPageToken: next}
		token = next
	}
	jobs := compactJobs(1)
	jobs[0].Status, jobs[0].CompletionStatus = jobdb.JobStatusCompleted, "success"
	engine.pages[token] = jobdb.ListJobsResponse{Jobs: jobs}
	server := &Server{engine: engine}
	query := url.Values{"displayStatus": {"success"}, "pageSize": {"1"}}
	first := displayStatusPage(t, server, query)
	if len(first.Jobs) != 0 || first.NextPageToken != token || len(engine.requests) != maxDisplayStatusPages {
		t.Fatalf("bounded scan: %+v requests=%d", first, len(engine.requests))
	}
	query.Set("pageToken", first.NextPageToken)
	last := displayStatusPage(t, server, query)
	if len(last.Jobs) != 1 || last.NextPageToken != "" {
		t.Fatalf("lost continuation: %+v", last)
	}
}

func TestInvalidDisplayStatuses(t *testing.T) {
	for _, query := range []string{"displayStatus=COMPLETED", "displayStatus=constructor", "displayStatus=completion:", "displayStatus=success&status=READY"} {
		w := httptest.NewRecorder()
		(&Server{}).handleListJobs(w, httptest.NewRequest("GET", "/jobs?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("query=%s status=%d body=%s", query, w.Code, w.Body)
		}
	}
}

func TestDisplayStatusFilteringWithRemoteSQLite(t *testing.T) {
	ctx := context.Background()
	runtime, err := sqliteruntime.StartEmbeddedRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	backend := httptest.NewServer(remote.NewServer(runtime.Runtime))
	defer backend.Close()
	server, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(server)
	defer api.Close()
	base := api.URL + "/api/projects/tenant/jobs"
	var goal string
	outcomes := []string{"failed_app", "success", "failed_app", "success", ""}
	for i := range outcomes {
		id := fmt.Sprintf("job-%d", i)
		job := postJSON[recipejob.RecipeJob](t, base, map[string]any{"job_id": id, "cell": "alpha", "recipe": "smoke"})
		if i == 0 {
			goal = job.GoalID
		} else {
			_, err := server.engine.(jobdb.GoalRuntime).AddGoalCandidate(ctx, jobdb.AddGoalCandidateRequest{GoalKey: jobdb.GoalKey{TenantId: "tenant", GoalID: goal}, JobID: id})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, outcome := range outcomes {
		if outcome == "" {
			continue
		}
		id := fmt.Sprintf("job-%d", i)
		lease, err := server.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: id}, WorkerID: "status-test", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if lease == nil {
			t.Fatal("expected job lease")
		}
		err = lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: outcome, Chapter: &jobdb.Chapter{
			Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now().UTC(),
			Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{
				Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"ok":true}`)},
			}},
		}})
		lease.StopKeepAlive()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, statuses := range [][]string{{"failed_app"}, {"READY", "failed_app"}, {"success"}} {
		query := url.Values{"displayStatus": statuses, "pageSize": {"1"}, "cell": {"alpha"}, "goal": {goal}}
		seen := make(map[string]bool)
		for pages := 0; ; pages++ {
			if pages > 6 {
				t.Fatal("pagination did not finish")
			}
			page := getJSON[recipejob.ListRecipeJobsResponse](t, base+"?"+query.Encode())
			if len(page.Jobs) > 1 {
				t.Fatalf("page exceeds requested size: %+v", page)
			}
			for _, job := range page.Jobs {
				if seen[job.JobID] {
					t.Fatalf("duplicate job %s", job.JobID)
				}
				seen[job.JobID] = true
			}
			if page.NextPageToken == "" {
				break
			}
			query.Set("pageToken", page.NextPageToken)
		}
		want := map[string]bool{"job-0": true, "job-2": true}
		if statuses[0] == "READY" {
			want["job-4"] = true
		}
		if statuses[0] == "success" {
			want = map[string]bool{"job-1": true, "job-3": true}
		}
		if !reflect.DeepEqual(seen, want) {
			t.Fatalf("statuses=%v got=%v want=%v", statuses, seen, want)
		}
	}
	for _, query := range []string{"displayStatus=success&cell=beta", "displayStatus=success&goal=missing"} {
		if page := getJSON[recipejob.ListRecipeJobsResponse](t, base+"?"+query); len(page.Jobs) != 0 {
			t.Fatalf("scope ignored: %+v", page)
		}
	}
	if page := getJSON[recipejob.ListRecipeJobsResponse](t, api.URL+"/api/projects/another/jobs?displayStatus=success"); len(page.Jobs) != 0 {
		t.Fatalf("tenant leaked: %+v", page)
	}
}
