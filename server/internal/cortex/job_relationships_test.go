package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func TestChildrenSummaryPagination(t *testing.T) {
	for _, token := range []string{"", "cursor+/=", "empty"} {
		jobs := compactJobs(2)
		jobs[1].JobType = "other"
		if token == "empty" {
			jobs = jobs[1:]
		}
		engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{token: {Jobs: jobs, NextPageToken: "next+/="}}}
		request := mux.SetURLVars(httptest.NewRequest("GET", "/children?pageToken="+url.QueryEscape(token), nil), map[string]string{"projectId": "tenant", "jobId": "parent"})
		w := httptest.NewRecorder()
		(&Server{engine: engine}).handleJobChildren(w, request)
		if w.Code != 200 || len(engine.requests) != 1 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		req := engine.requests[0]
		if !reflect.DeepEqual(req.TenantIds, []string{"tenant"}) || !reflect.DeepEqual(req.ParentJobIDs, []string{"parent"}) || !req.IncludeHidden || len(req.Stores) != 2 || len(req.Statuses) != 0 || req.PageSize != 20 || req.PageToken != token {
			t.Fatalf("bad query: %+v", req)
		}
		var response struct {
			Jobs  []json.RawMessage `json:"jobs"`
			Token string            `json:"next_page_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		want := 1
		if token == "empty" {
			want = 0
		}
		if response.Jobs == nil || len(response.Jobs) != want || response.Token != "next+/=" {
			t.Fatalf("bad response %s", w.Body)
		}
	}
}
func TestChildrenPageSize(t *testing.T) {
	for _, value := range []string{"0", "-1", "no", "", "1.2", "1&pageSize=2", "999", "2"} {
		t.Run(value, func(t *testing.T) {
			engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {}}}
			w := httptest.NewRecorder()
			req := mux.SetURLVars(httptest.NewRequest("GET", "/children?pageSize="+value, nil), map[string]string{"projectId": "tenant", "jobId": "parent"})
			(&Server{engine: engine}).handleJobChildren(w, req)
			if value == "999" || value == "2" {
				want := 2
				if value == "999" {
					want = jobdb.MaxListJobsPageSize
				}
				if w.Code != 200 || engine.requests[0].PageSize != want {
					t.Fatalf("%d %s", w.Code, w.Body)
				}
			} else if w.Code != 400 || len(engine.requests) != 0 {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

type childrenErrorEngine struct{ *compactListEngine }

func (e childrenErrorEngine) ListJobs(context.Context, jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	return jobdb.ListJobsResponse{}, errors.New("backend unavailable")
}
func TestChildrenBackendError(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{engine: childrenErrorEngine{}}).handleJobChildren(w, mux.SetURLVars(httptest.NewRequest("GET", "/children", nil), map[string]string{"projectId": "tenant", "jobId": "parent"}))
	if w.Code != 500 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
