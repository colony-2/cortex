package cortex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
)

func TestJobsListGoalMetadataAndStatusPagination(t *testing.T) {
	runtime, err := sqliteruntime.StartEmbeddedRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	backend := httptest.NewServer(remote.NewServer(runtime.Runtime))
	defer backend.Close()
	srv, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/goals-test"
	var goal string
	for i := 0; i < 26; i++ {
		job := postJSON[recipejob.RecipeJob](t, base+"/jobs", map[string]any{
			"job_id": fmt.Sprintf("attempt-%02d", i), "cell": "alpha", "inputs": map[string]any{"prompt": "Build a useful tool", "type": "build"},
		})
		if i == 0 {
			goal = job.GoalID
		}
		if job.Prompt != "Build a useful tool" || job.GoalID == "" {
			t.Fatalf("submission metadata: %#v", job)
		}
		if i > 0 {
			_, err := srv.engine.(jobdb.GoalRuntime).AddGoalCandidate(context.Background(), jobdb.AddGoalCandidateRequest{GoalKey: jobdb.GoalKey{TenantId: "goals-test", GoalID: goal}, JobID: job.JobID})
			if err != nil {
				t.Fatal(err)
			}
		}
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
	// The matching jobs are outside the initial unfiltered page of size one.
	// Filtering must happen before pagination, including additional goal memberships.
	for _, job := range []recipejob.RecipeJob{first.Jobs[24], last.Jobs[0]} {
		req, _ := http.NewRequest(http.MethodPost, base+"/jobs/"+job.JobID+"/cancel", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("cancel status: %d", resp.StatusCode)
		}
	}
	filteredQuery := "/jobs?status=CANCELLED&cell=alpha&goal=" + url.QueryEscape(goal) + "&pageSize=1"
	filtered := getJSON[page](t, base+filteredQuery)
	if len(filtered.Jobs) != 1 || string(filtered.Jobs[0].Status) != "CANCELLED" || filtered.Next == "" {
		t.Fatalf("status filter before pagination: %#v", filtered)
	}
	filteredLast := getJSON[page](t, base+filteredQuery+"&pageToken="+url.QueryEscape(filtered.Next))
	if len(filteredLast.Jobs) != 1 || string(filteredLast.Jobs[0].Status) != "CANCELLED" || filteredLast.Jobs[0].JobID == filtered.Jobs[0].JobID || filteredLast.Next != "" {
		t.Fatalf("filtered continuation: %#v", filteredLast)
	}
	other := getJSON[page](t, base+"/jobs?status=all&cell=beta&goal="+url.QueryEscape(goal))
	if len(other.Jobs) != 0 {
		t.Fatalf("repository filter ignored: %#v", other)
	}
	isolated := getJSON[page](t, api.URL+"/api/projects/another-tenant"+query)
	if len(isolated.Jobs) != 0 {
		t.Fatalf("tenant isolation: %#v", isolated)
	}
}

func TestJobsListMultipleCellsBeforePagination(t *testing.T) {
	runtime, err := sqliteruntime.StartEmbeddedRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	backend := httptest.NewServer(remote.NewServer(runtime.Runtime))
	defer backend.Close()
	srv, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/cells-test/jobs"
	var goal string
	for _, entry := range []struct{ id, cell string }{
		{"alpha-one", "alpha"}, {"beta-one", "beta"}, {"alpha-two", "alpha"}, {"self", "self"}, {"other-goal", "beta"},
	} {
		job := postJSON[recipejob.RecipeJob](t, base, map[string]any{"job_id": entry.id, "cell": entry.cell, "recipe": "smoke"})
		if goal == "" {
			goal = job.GoalID
		} else if entry.id != "other-goal" {
			_, err := srv.engine.(jobdb.GoalRuntime).AddGoalCandidate(context.Background(), jobdb.AddGoalCandidateRequest{
				GoalKey: jobdb.GoalKey{TenantId: "cells-test", GoalID: goal}, JobID: job.JobID,
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if entry.id == "alpha-one" || entry.id == "beta-one" {
			if err := srv.engine.CancelJob(context.Background(), jobdb.CancelJob{JobKey: jobdb.JobKey{TenantId: "cells-test", JobId: job.JobID}}); err != nil {
				t.Fatal(err)
			}
		}
	}
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
			query := base + "?cell=alpha&cell=beta&" + test.status + "&goal=" + url.QueryEscape(goal) + "&pageSize=1"
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
	// Preserve the legacy repository alias and normalization of a single selector.
	legacy := getJSON[page](t, base+"?repo="+url.QueryEscape("github.com/acme/boo-alpha")+"&status=all")
	if len(legacy.Jobs) != 2 {
		t.Fatalf("legacy repo filter: %#v", legacy)
	}
}
