package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

// All unimplemented operations panic through the nil embedded engine: these
// tests cannot accidentally pass if a list starts inspecting jobs or artifacts.
type compactListEngine struct {
	jobworkflow.Engine
	requests []jobdb.ListJobsRequest
	pages    map[string]jobdb.ListJobsResponse
}

func (e *compactListEngine) ListJobs(_ context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.requests = append(e.requests, req)
	return e.pages[req.PageToken], nil
}
func compactJobs(count int) []jobdb.JobSummary {
	jobs := make([]jobdb.JobSummary, count)
	for i := range jobs {
		jobs[i] = jobdb.JobSummary{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: fmt.Sprint(i)},
			JobType: "recipe", Status: jobdb.JobStatusReady, ClientPayloadRevision: 8,
			Metadata:  json.RawMessage(`{"cell_name":"platform","prompt":"Build it"}`),
			NextRoute: &jobdb.Route{JobType: "recipe", TaskType: "input:collect_user_input"},
		}
	}
	return jobs
}
func TestCompactJobListsUseOneOperationPerPage(t *testing.T) {
	engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{
		"":      {Jobs: compactJobs(200), NextPageToken: "empty"},
		"empty": {Jobs: []jobdb.JobSummary{}, NextPageToken: "last"},
		"last":  {Jobs: compactJobs(1)},
	}}
	s := &Server{engine: engine}
	for _, token := range []string{"", "", "empty", "last"} {
		before := len(engine.requests)
		w := httptest.NewRecorder()
		req := mux.SetURLVars(httptest.NewRequest("GET", "/jobs?status=all&pageSize=200&pageToken="+token, nil), map[string]string{"projectId": "tenant"})
		s.handleListJobs(w, req)
		if w.Code != 200 || len(engine.requests) != before+1 {
			t.Fatalf("%d %s requests=%d", w.Code, w.Body, len(engine.requests))
		}
		if token == "empty" {
			var page map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page["next_page_token"] != "last" {
				t.Fatal(page)
			}
		}
	}
}
func TestPendingSummariesTraversePagesWithoutInspection(t *testing.T) {
	engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{
		"":      {Jobs: compactJobs(200), NextPageToken: "empty"},
		"empty": {NextPageToken: "last"}, "last": {Jobs: compactJobs(1)},
	}}
	s := &Server{engine: engine}
	inputs, err := s.pendingInputSummaries(context.Background(), "tenant")
	if err != nil || len(inputs) != 201 || len(engine.requests) != 3 {
		t.Fatalf("inputs=%d calls=%d err=%v", len(inputs), len(engine.requests), err)
	}
	for _, req := range engine.requests {
		if len(req.TenantIds) != 1 || req.TenantIds[0] != "tenant" || len(req.JobTasks) != 1 || req.JobTasks[0].TaskType != "input:collect_user_input" || len(req.Statuses) != 1 || req.Statuses[0] != jobdb.JobStatusReady {
			t.Fatalf("incorrect pending filter: %+v", req)
		}
	}
	if inputs[0].CellName != "platform" || inputs[0].Prompt != "Build it" {
		t.Fatalf("missing labels: %+v", inputs[0])
	}
}
func TestRequireJobChecksCompactExistence(t *testing.T) {
	for _, exists := range []bool{true, false} {
		engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{}}
		if exists {
			engine.pages[""] = jobdb.ListJobsResponse{Jobs: compactJobs(1)}
		}
		s := &Server{engine: engine}
		called := false
		w := httptest.NewRecorder()
		s.requireJob(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(204) })(w, mux.SetURLVars(httptest.NewRequest("GET", "/story", nil), map[string]string{"projectId": "tenant", "jobId": "0"}))
		want := 404
		if exists {
			want = 204
		}
		if w.Code != want || called != exists || len(engine.requests) != 1 {
			t.Fatalf("%d called=%v requests=%d", w.Code, called, len(engine.requests))
		}
		req := engine.requests[0]
		if len(req.JobKeys) != 1 || req.JobKeys[0] != (jobdb.JobKey{TenantId: "tenant", JobId: "0"}) || len(req.Stores) != 2 {
			t.Fatalf("incorrect existence filter: %+v", req)
		}
	}
}
