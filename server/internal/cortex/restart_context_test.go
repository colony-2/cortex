package cortex

import (
	"context"
	"errors"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/story"
)

type contextInspectionService struct {
	storyService
	request story.GetRestartContextRequest
	err     error
}

func (s *contextInspectionService) GetRestartContext(_ context.Context, req story.GetRestartContextRequest) (*story.RestartContext, error) {
	s.request = req
	return &story.RestartContext{
		ProjectID: req.ProjectID, JobID: req.JobID, ChapterOrdinal: req.ChapterOrdinal, ScopeID: "recorded-scope",
		Job: map[string]any{"unchanged": true}, Visible: map[string]any{"vars": map[string]any{"token": "[REDACTED]"}},
		RedactedPaths: []string{"/visible/vars/token"},
	}, s.err
}

func TestRestartContextHTTP(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	postJSON[map[string]any](t, ts.URL+"/api/projects/tenant/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
	postJSON[map[string]any](t, ts.URL+"/api/projects/tenant/jobs", map[string]any{"job_id": "source", "recipe": "smoke", "cell": "self", "repo": "github.com/acme/boo-self"})
	inspection := &contextInspectionService{storyService: srv.story}
	srv.story = inspection
	base := ts.URL + "/api/projects/tenant/jobs/source/restart-context"
	for _, query := range []string{"", "?chapter_ordinal=invalid", "?chapter_ordinal=1.5", "?chapter_ordinal=-1"} {
		req, _ := http.NewRequest(http.MethodGet, base+query, nil)
		doJSON[errorResponse](t, req, http.StatusBadRequest)
	}
	if inspection.request.JobID != "" {
		t.Fatal("invalid query reached inspection")
	}
	getJSON[story.RestartContext](t, base+"?chapter_ordinal=0")
	if inspection.request != (story.GetRestartContextRequest{ProjectID: "tenant", JobID: "source", ChapterOrdinal: 0}) {
		t.Fatalf("submission inspection request = %#v", inspection.request)
	}
	result := getJSON[story.RestartContext](t, base+"?chapter_ordinal=17")
	if inspection.request != (story.GetRestartContextRequest{ProjectID: "tenant", JobID: "source", ChapterOrdinal: 17}) {
		t.Fatalf("inspection request = %#v", inspection.request)
	}
	if result.Job["unchanged"] != true || len(result.RedactedPaths) != 1 || result.Visible["vars"] == nil {
		t.Fatalf("context lost fields: %#v", result)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{
		{story.ErrInvalidRestartBoundary, http.StatusBadRequest},
		{errors.Join(story.ErrRestartContextUnavailable, errors.New("missing recipe history")), http.StatusUnprocessableEntity},
		{story.ErrNotFound, http.StatusNotFound},
	} {
		inspection.err = tc.err
		req, _ := http.NewRequest(http.MethodGet, base+"?chapter_ordinal=17", nil)
		doJSON[errorResponse](t, req, tc.status)
	}
	inspection.request = story.GetRestartContextRequest{}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/projects/other/jobs/source/restart-context?chapter_ordinal=17", nil)
	doJSON[errorResponse](t, req, http.StatusNotFound)
	if inspection.request.JobID != "" {
		t.Fatal("other tenant reached inspection")
	}
	for _, body := range []string{`{}`, `{"chapter_ordinal":17,"scope_id":"scope"}`, `{"chapter_ordinal":"17"}`} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/projects/tenant/jobs/source/restart", strings.NewReader(body))
		doJSON[errorResponse](t, req, http.StatusBadRequest)
	}
}

// The forwarding boundary must not turn a saved replay into fresh admission.
// Unimplemented engine methods panic: completion/goal/sibling preflights fail this test.
type replayForwardService struct {
	storyService
	calls []story.RestartRecipeJobRequest
}

func (s *replayForwardService) RestartRecipeJob(_ context.Context, req story.RestartRecipeJobRequest) (*story.RestartRecipeJobResponse, error) {
	s.calls = append(s.calls, req)
	return &story.RestartRecipeJobResponse{TenantID: req.ProjectID, JobID: "accepted-successor", GoalID: "returned-primary"}, nil
}

func TestSavedReplayForwardingWithoutAdmissionPreflights(t *testing.T) {
	source := compactJobs(1)
	source[0].JobKey.JobId = "source"
	source[0].GoalID = "returned-primary"
	// A source may still look active after a saved request was accepted. Cortex
	// cannot determine closure from this read; the owning service reconciles it.
	source[0].Status = jobdb.JobStatusActive
	source[0].CompletionStatus = "success"
	engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: source}}}
	service := &replayForwardService{}
	server := &Server{engine: engine, story: service}
	for _, body := range []string{
		`{"request_id":"saved","scope_id":"retained-boundary","goal":{"mode":"continue"},"patch":[{"op":"add","path":"/sequence/a~1b/outputs/n","value":9007199254740993}]}`,
		`{"request_id":"legacy","chapter_ordinal":0}`,
		`{"request_id":"fork","chapter_ordinal":12,"goal":{"mode":"new"}}`,
	} {
		before := len(service.calls)
		// Repeat the identical accepted request with a live successor, then after
		// closure. Those facts belong to the service and add no Cortex veto.
		for range 3 {
			w := httptest.NewRecorder()
			req := mux.SetURLVars(httptest.NewRequest("POST", "/restart", strings.NewReader(body)), map[string]string{"projectId": "tenant", "jobId": "source"})
			server.requireJob(server.handleRestartJob)(w, req)
			if w.Code != http.StatusCreated {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		}
		first := service.calls[before]
		if first.ProjectID != "tenant" || first.JobID != "source" || first.RequestID == "" {
			t.Fatalf("identity lost: %+v", first)
		}
		if !reflect.DeepEqual(first, service.calls[before+1]) || !reflect.DeepEqual(first, service.calls[before+2]) {
			t.Fatal("saved request changed in forwarding")
		}
	}
	if len(engine.requests) != 9 {
		t.Fatal("unexpected visibility reads")
	}
	for _, req := range engine.requests {
		if len(req.GoalIDs) != 0 || !reflect.DeepEqual(req.JobKeys, []jobdb.JobKey{{TenantId: "tenant", JobId: "source"}}) {
			t.Fatalf("not exact visibility: %+v", req)
		}
	}
	before := len(service.calls)
	for _, body := range []string{`{"scope_id":"point"}`, `{"request_id":"x","scope_id":"point","goal":{"mode":"bad"}}`, `{"request_id":"x","scope_id":"point","chapter_ordinal":1}`, `{"request_id":"x","scope_id":"point"} {}`, `{"request_id":"x","scope_id":"point","patch":[{"op":"remove","path":"/context/workspace"}]}`} {
		w := httptest.NewRecorder()
		req := mux.SetURLVars(httptest.NewRequest("POST", "/restart", strings.NewReader(body)), map[string]string{"projectId": "tenant", "jobId": "source"})
		server.requireJob(server.handleRestartJob)(w, req)
		if w.Code != 400 {
			t.Fatalf("invalid request accepted: %s status %d", body, w.Code)
		}
	}
	if len(service.calls) != before {
		t.Fatal("validation bypassed")
	}
	engine.pages[""] = jobdb.ListJobsResponse{}
	w := httptest.NewRecorder()
	req := mux.SetURLVars(httptest.NewRequest("POST", "/restart", strings.NewReader(`{"request_id":"saved","scope_id":"point"}`)), map[string]string{"projectId": "other", "jobId": "source"})
	server.requireJob(server.handleRestartJob)(w, req)
	if w.Code != 404 || len(service.calls) != before {
		t.Fatal("visibility bypassed")
	}
}
