package cortex

import (
	"context"
	"fmt"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/gorilla/mux"
)

func TestJobsListGoalMetadataAndStatusPagination(t *testing.T) {
	runtime, err := sqliteruntime.StartEmbeddedRuntime(testjobdb.Context(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	backend := httptest.NewServer(primaryFixtureHandler(remote.NewServer(runtime.Runtime)))
	defer backend.Close()
	srv, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/goals-test"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "alpha", "repo": "github.com/acme/boo-alpha"})
	var goal string
	for i := 0; i < 26; i++ {
		var job recipejob.RecipeJob
		if i == 0 {
			job = postJSON[recipejob.RecipeJob](t, base+"/jobs", map[string]any{"job_id": fmt.Sprintf("attempt-%02d", i), "cell": "alpha", "repo": "github.com/acme/boo-alpha", "inputs": map[string]any{"prompt": "Build a useful tool", "type": "build"}})
			goal = job.GoalID
		} else {
			job = submitPrimaryFixture(t, srv, "goals-test", fmt.Sprintf("attempt-%02d", i), "alpha", goal, map[string]interface{}{"prompt": "Build a useful tool", "type": "build"})
		}
		if job.Prompt != "Build a useful tool" || job.GoalID != goal {
			t.Fatalf("submission metadata: %#v", job)
		}
		outcome := "failed_app"
		if i < 2 {
			outcome = "cancelled"
		}
		completePrimaryFixture(t, srv.engine, "goals-test", job.JobID, outcome)
	}
	type page struct {
		Jobs []recipejob.RecipeJob `json:"jobs"`
		Next string                `json:"next_page_token"`
	}
	query := "/jobs?status=all&goal=" + url.QueryEscape(goal) + "&pageSize=25"
	first := getJSON[page](t, base+query)
	if len(first.Jobs) != 25 || first.Next == "" {
		t.Fatalf("pagination: %#v", first)
	}
	for _, job := range first.Jobs {
		if job.Prompt != "Build a useful tool" || job.CellName != "alpha" || job.SubmittedAt == nil || job.RepositorySource == "" {
			t.Fatalf("list metadata missing: %#v", job)
		}
	}
	last := getJSON[page](t, base+query+"&pageToken="+url.QueryEscape(first.Next))
	if len(last.Jobs) != 1 || last.Next != "" {
		t.Fatalf("last page: %#v", last)
	}
	// Completed unsuccessful predecessors retain primary membership. Display
	// status intersects cell/goal scope before result pagination.
	filteredQuery := "/jobs?displayStatus=CANCELLED&cellNames=alpha&goal=" + url.QueryEscape(goal) + "&pageSize=1"
	filtered := getJSON[page](t, base+filteredQuery)
	for len(filtered.Jobs) == 0 && filtered.Next != "" {
		filtered = getJSON[page](t, base+filteredQuery+"&pageToken="+url.QueryEscape(filtered.Next))
	}
	if len(filtered.Jobs) != 1 || jobDisplayStatus(filtered.Jobs[0]) != "CANCELLED" || filtered.Next == "" {
		t.Fatalf("status filter before pagination: %#v", filtered)
	}
	filteredLast := getJSON[page](t, base+filteredQuery+"&pageToken="+url.QueryEscape(filtered.Next))
	if len(filteredLast.Jobs) != 1 || jobDisplayStatus(filteredLast.Jobs[0]) != "CANCELLED" || filteredLast.Jobs[0].JobID == filtered.Jobs[0].JobID || filteredLast.Next != "" {
		t.Fatalf("filtered continuation: %#v", filteredLast)
	}
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "beta", "repo": "github.com/acme/beta"})
	other := getJSON[page](t, base+"/jobs?status=all&cellNames=beta&goal="+url.QueryEscape(goal))
	if len(other.Jobs) != 0 {
		t.Fatalf("repository filter ignored: %#v", other)
	}
	isolated := getJSON[page](t, api.URL+"/api/projects/another-tenant"+query)
	if len(isolated.Jobs) != 0 {
		t.Fatalf("tenant isolation: %#v", isolated)
	}
}

func TestJobsListMultipleCellsBeforePagination(t *testing.T) {
	// Stored translated history can contain multi-cell attempts that current
	// primary-goal admission rejects. Keep real runtime setup in the preceding test.
	jobs := compactJobs(5)
	for i, entry := range []struct{ id, cell string }{{"alpha-one", "alpha"}, {"beta-one", "beta"}, {"alpha-two", "alpha"}, {"self", "self"}, {"other-goal", "beta"}} {
		jobs[i].JobKey.JobId = entry.id
		jobs[i].Cell = entry.cell
		jobs[i].GoalID = "goal"
		jobs[i].Status = jobdb.JobStatusCompleted
		jobs[i].CompletionStatus = "failed_app"
		if i < 2 {
			jobs[i].CompletionStatus = "cancelled"
		}
		if i == 4 {
			jobs[i].GoalID = "elsewhere"
		}
	}
	engine := &historicalSummaryEngine{rows: jobs}
	srv := &Server{engine: engine}
	router := mux.NewRouter()
	router.HandleFunc("/api/projects/{projectId}/jobs", srv.handleListJobs)
	api := httptest.NewServer(router)
	defer api.Close()
	base := api.URL + "/api/projects/tenant/jobs"
	goal := "goal"
	type page struct {
		Jobs []recipejob.RecipeJob `json:"jobs"`
		Next string                `json:"next_page_token"`
	}
	for _, test := range []struct {
		name, status string
		want         map[string]bool
	}{
		{"all statuses", "status=all", map[string]bool{"alpha-one": true, "beta-one": true, "alpha-two": true}},
		{"display status intersection", "displayStatus=CANCELLED", map[string]bool{"alpha-one": true, "beta-one": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := base + "?cellNames=alpha&cellNames=beta&" + test.status + "&goal=" + url.QueryEscape(goal) + "&pageSize=1"
			next := ""
			seen := map[string]bool{}
			for i := 0; i < 10; i++ {
				result := getJSON[page](t, query+"&pageToken="+url.QueryEscape(next))
				if len(result.Jobs) > 1 {
					t.Fatalf("page size ignored: %#v", result)
				}
				for _, job := range result.Jobs {
					if !test.want[job.JobID] || seen[job.JobID] {
						t.Fatalf("unexpected or duplicate job %s: %#v", job.JobID, result)
					}
					seen[job.JobID] = true
				}
				next = result.Next
				if next == "" {
					break
				}
			}
			if next != "" || len(seen) != len(test.want) {
				t.Fatalf("incomplete multi-cell results: %v, cursor %q", seen, next)
			}
		})
	}
	// Legacy repository selectors must not silently broaden the query.
	req, _ := http.NewRequest(http.MethodGet, base+"?repo=github.com/acme/boo-alpha", nil)
	doJSON[errorResponse](t, req, http.StatusBadRequest)
}
