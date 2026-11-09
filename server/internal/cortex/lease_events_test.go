package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

type leaseEventsEngine struct {
	jobworkflow.Engine          // Unrelated reads panic: no row enrichment is allowed.
	visibility                  []jobdb.ListJobsRequest
	reads                       []jobdb.ListLeaseEventsRequest
	page                        jobdb.ListLeaseEventsResponse
	missing                     bool
	visibilityError, eventError error
	ctx                         context.Context
}

func (e *leaseEventsEngine) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.visibility = append(e.visibility, req)
	e.ctx = ctx
	if e.visibilityError != nil {
		return jobdb.ListJobsResponse{}, e.visibilityError
	}
	if e.missing {
		return jobdb.ListJobsResponse{}, nil
	}
	jobs := compactJobs(1)
	jobs[0].JobKey.JobId = "selected"
	return jobdb.ListJobsResponse{Jobs: jobs}, nil
}
func (e *leaseEventsEngine) ListLeaseEvents(ctx context.Context, req jobdb.ListLeaseEventsRequest) (jobdb.ListLeaseEventsResponse, error) {
	e.reads = append(e.reads, req)
	e.ctx = ctx
	return e.page, e.eventError
}
func leaseRequest(t *testing.T, engine *leaseEventsEngine, query string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{engine: engine, leaseEvents: leaseEventsRuntime{engine: engine}}
	router := mux.NewRouter()
	router.HandleFunc("/projects/{projectId}/jobs/{jobId}/events", s.handleJobLeaseEvents)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/projects/tenant/jobs/selected/events"+query, nil))
	return w
}
func TestLeaseEventsOneCanonicalPage(t *testing.T) {
	at, _ := time.Parse(time.RFC3339Nano, "2026-11-08T05:00:00.123456789Z")
	for _, empty := range []bool{false, true} {
		events := []jobdb.LeaseEvent{{EventID: "999999999999999999999999999999", EventType: jobdb.LeaseEventRenewed, OccurredAt: at, WorkerID: "<script>alert(1)</script>", LeaseExpiresAt: at.Add(-time.Second)}, {EventID: "1", EventType: jobdb.LeaseEventAcquired, OccurredAt: at}}
		if empty {
			events = []jobdb.LeaseEvent{}
		}
		engine := &leaseEventsEngine{page: jobdb.ListLeaseEventsResponse{Events: events, NextPageToken: "opaque+/=雪"}}
		got := leaseRequest(t, engine, "?pageSize=2&pageToken="+url.QueryEscape("cursor+/=雪"))
		want, _ := json.Marshal(engine.page)
		if got.Code != 200 || got.Body.String() != string(want)+"\n" {
			t.Fatalf("%d %s", got.Code, got.Body)
		}
		if len(engine.visibility) != 1 || len(engine.reads) != 1 {
			t.Fatalf("unexpected reads: %+v", engine)
		}
		vis := engine.visibility[0]
		if !vis.IncludeHidden || !reflect.DeepEqual(vis.TenantIds, []string{"tenant"}) || !reflect.DeepEqual(vis.JobKeys, []jobdb.JobKey{{TenantId: "tenant", JobId: "selected"}}) || len(vis.Stores) != 2 || vis.PageSize != 1 {
			t.Fatalf("visibility: %+v", vis)
		}
		wantReq := jobdb.ListLeaseEventsRequest{TenantID: "tenant", JobIDs: []string{"selected"}, IncludeHidden: true, PageSize: 2, PageToken: "cursor+/=雪"}
		if !reflect.DeepEqual(engine.reads[0], wantReq) {
			t.Fatalf("event scope: %+v", engine.reads[0])
		}
	}
}
func TestLeaseEventsQueryValidation(t *testing.T) {
	for _, query := range []string{"?tenantId=other", "?projectId=other", "?jobId=other", "?runtime=evil", "?includeHidden=false", "?eventTypes=lease_acquired", "?pageSize=0", "?pageSize=201", "?pageSize=-1", "?pageSize=", "?pageSize=1.2", "?pageSize=1&pageSize=2", "?pageToken=a&pageToken=b", "?pageToken=%zz", "?x=1"} {
		t.Run(query, func(t *testing.T) {
			engine := &leaseEventsEngine{}
			got := leaseRequest(t, engine, query)
			if got.Code != 400 || len(engine.reads) != 0 || len(engine.visibility) != 0 {
				t.Fatalf("%d %s %+v", got.Code, got.Body, engine)
			}
		})
	}
	for _, query := range []string{"", "?pageSize=1", "?pageSize=200", "?pageToken="} {
		engine := &leaseEventsEngine{page: jobdb.ListLeaseEventsResponse{Events: []jobdb.LeaseEvent{}}}
		got := leaseRequest(t, engine, query)
		if got.Code != 200 || len(engine.reads) != 1 {
			t.Fatalf("%d %s", got.Code, got.Body)
		}
		if query == "" && engine.reads[0].PageSize != 100 {
			t.Fatal("incorrect default")
		}
	}
}
func TestLeaseEventsMissingAndDeniedVisibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing bool
		err     error
		status  int
	}{
		{"missing", true, nil, 404}, {"cell denied", false, jobdb.ErrCellForbidden, 403}, {"backend", false, errors.New("offline"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &leaseEventsEngine{missing: tc.missing, visibilityError: tc.err}
			got := leaseRequest(t, engine, "")
			if got.Code != tc.status || len(engine.reads) != 0 || len(engine.visibility) != 1 {
				t.Fatalf("%d %s", got.Code, got.Body)
			}
		})
	}
}
func TestLeaseEventsTypedErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{jobdb.ErrInvalidLeaseEventRequest, 400, "invalid_request"},
		{fmt.Errorf("wrapped: %w", jobdb.ErrInvalidLeaseEventPageToken), 400, "invalid_lease_event_page_token"},
		{jobdb.ErrCellForbidden, 403, "forbidden"}, {jobdb.ErrCellNotFound, 404, "not_found"},
		{jobdb.ErrLeasePermissionDenied, 403, "lease_permission_denied"},
		{&jobdb.CellError{Code: "authentication_required", Status: 401}, 401, "authentication_required"},
		{errors.New("list lease events: http 404: unsupported API"), 500, ""}, {errors.New("backend offline"), 500, ""},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			engine := &leaseEventsEngine{eventError: tc.err}
			got := leaseRequest(t, engine, "")
			var body errorResponse
			if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if got.Code != tc.status || body.Code != tc.code || body.Error == "" || len(engine.reads) != 1 {
				t.Fatalf("%d %s", got.Code, got.Body)
			}
		})
	}
}
func TestLeaseEventsContext(t *testing.T) {
	engine := &leaseEventsEngine{}
	ctx, cancel := context.WithCancel(context.Background())
	req := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "selected"})
	(&Server{engine: engine, leaseEvents: leaseEventsRuntime{engine: engine}}).handleJobLeaseEvents(httptest.NewRecorder(), req)
	if engine.ctx != req.Context() {
		t.Fatal("lost context")
	}
	cancel()
	if engine.ctx.Err() != context.Canceled {
		t.Fatal("cancellation did not reach runtime")
	}
	engine.reads = nil
	(&Server{engine: engine, leaseEvents: leaseEventsRuntime{engine: engine}}).handleJobLeaseEvents(httptest.NewRecorder(), req)
	if len(engine.reads) != 0 {
		t.Fatal("canceled request reached event read")
	}
}

type leaseEventsRuntime struct {
	jobdb.WorkflowRuntime
	engine *leaseEventsEngine
}

func (r leaseEventsRuntime) ListLeaseEvents(ctx context.Context, req jobdb.ListLeaseEventsRequest) (jobdb.ListLeaseEventsResponse, error) {
	return r.engine.ListLeaseEvents(ctx, req)
}
