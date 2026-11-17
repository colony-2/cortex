package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type treeWaitEngine struct {
	*compactListEngine
	page                 jobdb.ListDependencyWaitsResponse
	waitErr, snapshotErr error
	requests             []jobdb.ListDependencyWaitsRequest
	snapshots            int
}

func (e *treeWaitEngine) ListDependencyWaits(ctx context.Context, q jobdb.ListDependencyWaitsRequest) (jobdb.ListDependencyWaitsResponse, error) {
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
		panic("missing wait deadline")
	}
	e.requests = append(e.requests, q)
	return e.page, e.waitErr
}
func (e *treeWaitEngine) GetJob(ctx context.Context, key jobdb.JobKey) (jobdb.JobInfo, error) {
	e.snapshots++
	if key != (jobdb.JobKey{TenantId: "tenant", JobId: "0"}) {
		panic("wrong exact key")
	}
	return jobdb.JobInfo{Status: jobdb.JobStatusPendingJobs, WaitFor: []string{"legacy"}}, e.snapshotErr
}
func waitFixture() jobdb.DependencyWaitInfo {
	return jobdb.DependencyWaitInfo{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "0"}, WaitID: "wait", State: jobdb.DependencyPending,
		Dependencies: []jobdb.DependencyResolution{
			{Target: jobdb.DependencyTarget{JobID: "independent"}, Condition: jobdb.JobPrereqComplete, State: jobdb.DependencyPending},
			{Target: jobdb.DependencyTarget{GoalID: "literal-goal"}, Condition: jobdb.JobPrereqSuccess, State: jobdb.DependencyPending, BlockedReason: "No successful attempt"},
			{Target: jobdb.DependencyTarget{JobID: "done"}, Condition: jobdb.JobPrereqSuccess, State: jobdb.DependencySatisfied},
		}}
}
func waitEngine() *treeWaitEngine {
	return &treeWaitEngine{compactListEngine: &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: compactJobs(1)}}}, page: jobdb.ListDependencyWaitsResponse{Waits: []jobdb.DependencyWaitInfo{waitFixture()}, NextPageToken: "next+/="}}
}
func readWait(t *testing.T, e *treeWaitEngine, query string) (int, treeWaitsResponse, string) {
	t.Helper()
	w := httptest.NewRecorder()
	r := mux.SetURLVars(httptest.NewRequest("GET", "/tree-waits"+query, nil), map[string]string{"projectId": "tenant", "jobId": "0"})
	(&Server{engine: e}).handleTreeWaits(w, r)
	var body treeWaitsResponse
	if w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, body, w.Body.String()
}
func TestTreeWaitsScopeProjectionAndPaging(t *testing.T) {
	for _, token := range []string{"", "?pageToken=a%2B%2F%3D"} {
		e := waitEngine()
		e.page.Waits[0].Failure = &jobdb.GoalFailure{Code: "private"}
		code, body, raw := readWait(t, e, token)
		if code != 200 || len(body.Waits) != 1 || body.Consumer.JobID != "0" || body.Waits[0].Dependencies[1].Target.GoalID != "literal-goal" || body.Waits[0].Dependencies[0].Condition != jobdb.JobPrereqComplete || strings.Contains(raw, "private") {
			t.Fatal(code, raw)
		}
		q := e.requests[0]
		if q.TenantId != "tenant" || q.ConsumerJobID != "0" || q.Target != nil || q.PageSize != 20 || len(q.States) != 1 || q.States[0] != jobdb.DependencyPending {
			t.Fatal(q)
		}
		exact := e.compactListEngine.requests[0]
		if !exact.IncludeHidden || len(exact.Stores) != 2 || len(exact.JobTypes) != 0 || exact.JobKeys[0].JobId != "0" {
			t.Fatal(exact)
		}
		if token == "" {
			if body.Snapshot == nil || body.Snapshot.State != "available" || body.Snapshot.LegacyWaitFor[0] != "legacy" || e.snapshots != 1 {
				t.Fatal(body)
			}
		} else if q.PageToken != "a+/=" || body.Snapshot != nil || e.snapshots != 0 {
			t.Fatal(q, body)
		}
	}
}
func TestTreeWaitsUnavailableAndPartial(t *testing.T) {
	for _, mode := range []string{"missing", "forbidden", "timeout", "snapshot", "foreign", "malformed", "unknown", "nil"} {
		t.Run(mode, func(t *testing.T) {
			e := waitEngine()
			switch mode {
			case "missing":
				e.pages[""] = jobdb.ListJobsResponse{}
			case "forbidden":
				e.waitErr = jobdb.ErrLeasePermissionDenied
			case "timeout":
				e.waitErr = context.DeadlineExceeded
			case "snapshot":
				e.snapshotErr = errors.New("private failure")
			case "foreign":
				e.page.Waits[0].JobKey.TenantId = "secret-foreign-tenant"
			case "malformed":
				e.page.Waits[0].Dependencies[0].Target.GoalID = "ambiguous"
			case "unknown":
				e.page.Waits[0].Dependencies[0].State = "FUTURE"
			case "nil":
				e.page.Waits = nil
			}
			code, body, raw := readWait(t, e, "")
			if strings.Contains(raw, "private") || strings.Contains(raw, "secret-foreign") {
				t.Fatal(raw)
			}
			switch mode {
			case "missing":
				if code != 404 || len(e.requests) != 0 {
					t.Fatal(code)
				}
			case "forbidden":
				if code != 403 {
					t.Fatal(code)
				}
			case "timeout":
				if code != 504 {
					t.Fatal(code)
				}
			case "snapshot":
				if code != 200 || body.Snapshot.State != "unavailable" || len(body.Waits) != 1 {
					t.Fatal(code, raw)
				}
			default:
				if code != 200 || len(body.Issues) == 0 {
					t.Fatal(code, raw)
				}
			}
		})
	}
	w := httptest.NewRecorder()
	e := waitEngine().compactListEngine
	r := mux.SetURLVars(httptest.NewRequest("GET", "/", nil), map[string]string{"projectId": "tenant", "jobId": "0"})
	(&Server{engine: e}).handleTreeWaits(w, r)
	if w.Code != 501 {
		t.Fatal(w.Code, w.Body)
	}
}
