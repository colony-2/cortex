package cortex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	postJSON[map[string]any](t, ts.URL+"/api/projects/tenant/jobs", map[string]any{"job_id": "source", "recipe": "smoke"})
	inspection := &contextInspectionService{storyService: srv.story}
	srv.story = inspection
	base := ts.URL + "/api/projects/tenant/jobs/source/restart-context"
	for _, query := range []string{"", "?chapter_ordinal=invalid", "?chapter_ordinal=1.5"} {
		req, _ := http.NewRequest(http.MethodGet, base+query, nil)
		doJSON[errorResponse](t, req, http.StatusBadRequest)
	}
	if inspection.request.JobID != "" {
		t.Fatal("invalid query reached inspection")
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
