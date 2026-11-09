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

func TestTreeProjection(t *testing.T) {
	summary := compactJobs(1)[0]
	summary.ParentJobID = "runtime"
	summary.Metadata = json.RawMessage(`{"parent_job_id":"recipe","parent_tenant_id":"tenant","secret":"private"}`)
	n := projectTreeNode(summary)
	if n.ParentJobID != "runtime" || n.RecipeParentJobID != "recipe" {
		t.Fatal(n)
	}
	summary.ParentJobID = ""
	if projectTreeNode(summary).ParentJobID != "" {
		t.Fatal("metadata fabricated edge")
	}
	summary.Metadata = json.RawMessage(`{"parent_job_id":"foreign","parent_tenant_id":"other"}`)
	if projectTreeNode(summary).RecipeParentJobID != "" {
		t.Fatal("foreign attribution")
	}
}
func TestTreeScopePaginationAndProjection(t *testing.T) {
	for _, size := range []string{"", "?pageSize=1000&pageToken=a%2B%2F%3D", "?pageSize=2"} {
		jobs := compactJobs(2)
		jobs[0].JobType = "generic"
		jobs[0].ParentJobID = "parent"
		jobs[1].JobKey.TenantId = "foreign"
		token := ""
		if strings.Contains(size, "pageToken") {
			token = "a+/="
		}
		engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{token: {Jobs: jobs, NextPageToken: "next+/="}}}
		w := httptest.NewRecorder()
		req := mux.SetURLVars(httptest.NewRequest("GET", "/tree-children"+size, nil), map[string]string{"projectId": "tenant", "jobId": "parent"})
		(&Server{engine: engine}).handleTreeChildren(w, req)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		q := engine.requests[0]
		if len(q.TenantIds) != 1 || q.TenantIds[0] != "tenant" || len(q.ParentJobIDs) != 1 || q.ParentJobIDs[0] != "parent" || !q.IncludeHidden || len(q.Stores) != 2 || len(q.JobTypes) != 0 || len(q.CellNames) != 0 || len(q.GoalIDs) != 0 || len(q.Statuses) != 0 || q.MetadataFilter != nil || q.PageToken != token || q.PageSize > 100 {
			t.Fatal(q)
		}
		body := w.Body.String()
		if !strings.Contains(body, `"job_type":"generic"`) || !strings.Contains(body, `next+/=`) || strings.Contains(body, "foreign") || strings.Contains(body, "prompt") || strings.Contains(body, "client_payload") {
			t.Fatal(body)
		}
	}
}
func TestTreeInvalidSizes(t *testing.T) {
	for _, query := range []string{"0", "-1", "x", "1.5", "1&pageSize=2", ""} {
		e := &compactListEngine{}
		w := httptest.NewRecorder()
		r := mux.SetURLVars(httptest.NewRequest("GET", "/?pageSize="+query, nil), map[string]string{"projectId": "tenant", "jobId": "parent"})
		(&Server{engine: e}).handleTreeChildren(w, r)
		if w.Code != 400 || len(e.requests) != 0 {
			t.Fatal(query, w.Code)
		}
	}
}

type treeAuthorityKey struct{}

type treeDeadlineEngine struct {
	*compactListEngine
	t      *testing.T
	cancel bool
}

func (e treeDeadlineEngine) ListJobs(ctx context.Context, q jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	if ctx.Value(treeAuthorityKey{}) != "caller" {
		e.t.Fatal("caller context lost")
	}
	d, ok := ctx.Deadline()
	if !ok || time.Until(d) > 10*time.Second {
		e.t.Fatal("missing deadline")
	}
	if e.cancel {
		<-ctx.Done()
		return jobdb.ListJobsResponse{}, ctx.Err()
	}
	return jobdb.ListJobsResponse{}, errors.New("private credentials backend failure")
}
func TestTreeSafeErrorsAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		ctx, stop := context.WithCancel(context.WithValue(context.Background(), treeAuthorityKey{}, "caller"))
		if cancel {
			stop()
		}
		defer stop()
		w := httptest.NewRecorder()
		r := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "parent"})
		(&Server{engine: treeDeadlineEngine{t: t, cancel: cancel}}).handleTreeChildren(w, r)
		if w.Code < 400 || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestTreeNodeExactIdentity(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		jobs := compactJobs(1)
		jobs[0].JobType = "generic"
		if foreign {
			jobs[0].JobKey.TenantId = "other"
		}
		e := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs}}}
		w := httptest.NewRecorder()
		r := mux.SetURLVars(httptest.NewRequest("GET", "/", nil), map[string]string{"projectId": "tenant", "jobId": "0"})
		(&Server{engine: e}).handleTreeNode(w, r)
		if foreign && w.Code != 404 || !foreign && w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		q := e.requests[0]
		if len(q.JobKeys) != 1 || q.JobKeys[0].TenantId != "tenant" || q.JobKeys[0].JobId != "0" || !q.IncludeHidden {
			t.Fatal(q)
		}
	}
}

func TestTreeKnownErrorStatuses(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{jobdb.ErrJobNotFound, 404}, {jobdb.ErrCellForbidden, 403}, {jobdb.ErrLeasePermissionDenied, 403}, {context.DeadlineExceeded, 504}} {
		w := httptest.NewRecorder()
		treeError(w, test.err)
		if w.Code != test.status || strings.Contains(w.Body.String(), test.err.Error()) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
