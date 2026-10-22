package cortex

import (
	"context"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func TestJobActions(t *testing.T) {
	for _, mode := range []string{"toy", "remote"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t)}
			var srv *Server
			var err error
			if mode == "remote" {
				backend := httptest.NewServer(remote.NewServer(toy.New()))
				t.Cleanup(backend.Close)
				cfg.JobDBURL = backend.URL
				srv, err = NewRemoteServer(cfg)
			} else {
				srv, err = NewUITestServer(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewServer(srv)
			defer ts.Close()
			for _, tenant := range []string{"a", "b"} {
				for _, id := range []string{"first", "second", "untouched"} {
					postJSON[map[string]interface{}](t, ts.URL+"/api/projects/"+tenant+"/jobs", map[string]interface{}{"job_id": id, "recipe": "smoke"})
				}
			}
			action := func(method, path string, want int) {
				t.Helper()
				req, err := http.NewRequest(method, ts.URL+"/api/projects/"+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != want {
					t.Fatalf("%s %s: status %d, want %d", method, path, resp.StatusCode, want)
				}
			}
			for _, tenant := range []string{"missing", "a"} {
				id := "first"
				if tenant == "a" {
					id = "missing"
				}
				action(http.MethodPost, tenant+"/jobs/"+id+"/cancel", http.StatusNotFound)
				action(http.MethodDelete, tenant+"/jobs/"+id, http.StatusNotFound)
			}
			action(http.MethodPost, "a/jobs/first/cancel", http.StatusNoContent)
			action(http.MethodPost, "a/jobs/first/cancel", http.StatusNoContent)
			cancelled := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/a/jobs/first")
			if cancelled["status"] != "CANCELLED" {
				t.Fatalf("cancelled job = %#v", cancelled)
			}
			// Deletion hides both archived and unfinished jobs; JobDB retains history.
			for _, id := range []string{"first", "second"} {
				action(http.MethodDelete, "a/jobs/"+id, http.StatusNoContent)
				job, err := srv.engine.GetJobRun(context.Background(), jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: id}})
				if err != nil {
					t.Fatal(err)
				}
				if job.Job.Status != jobdb.JobStatusCancelled {
					t.Fatalf("deleted job = %#v", job)
				}
				other := getJSON[map[string]interface{}](t, ts.URL+"/api/projects/b/jobs/"+id)
				if other["status"] != "READY" {
					t.Fatalf("other tenant changed: %#v", other)
				}
			}
			list := getJSON[struct {
				Jobs []struct {
					ID string `json:"job_id"`
				} `json:"jobs"`
			}](t, ts.URL+"/api/projects/a/jobs?status=all")
			if len(list.Jobs) != 1 || list.Jobs[0].ID != "untouched" {
				t.Fatalf("remaining jobs = %#v", list)
			}
			srv.jobDeletion = nil
			action(http.MethodDelete, "a/jobs/untouched", http.StatusNotImplemented)
		})
	}
}
