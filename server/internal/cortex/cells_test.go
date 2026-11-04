package cortex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func TestCellRegistryAndSubmission(t *testing.T) {
	for _, mode := range []string{"toy", "remote", "sqlite"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t)}
			var srv *Server
			var err error
			if mode != "toy" {
				var runtime jobdb.WorkflowRuntime = toy.New()
				if mode == "sqlite" {
					embedded, startErr := sqliteruntime.StartEmbeddedRuntime(context.Background())
					if startErr != nil {
						t.Fatal(startErr)
					}
					defer embedded.Shutdown()
					runtime = embedded.Runtime
				}
				backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(runtime)))
				defer backend.Close()
				cfg.JobDBURL = backend.URL
				srv, err = NewRemoteServer(cfg)
			} else {
				srv, err = NewUITestServer(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			api := httptest.NewServer(srv)
			defer api.Close()
			base := api.URL + "/api/projects/registry"
			created := postJSON[jobdb.Cell](t, base+"/cells", map[string]any{"name": "alpha", "repo": "https://github.com/acme/registered.git", "description": "A cell without jobs", "metadata": map[string]any{"nested": map[string]any{"values": []any{nil, true, "text"}}}, "requestId": "create-alpha"})
			if created.ID == created.Repo || created.Repo != "github.com/acme/registered" || created.State != jobdb.CellActive {
				t.Fatalf("cell: %+v", created)
			}
			repeated := postJSON[jobdb.Cell](t, base+"/cells", map[string]any{"name": "alpha", "repo": "https://github.com/acme/registered.git", "description": "A cell without jobs", "metadata": map[string]any{"nested": map[string]any{"values": []any{nil, true, "text"}}}, "requestId": "create-alpha"})
			if repeated.ID != created.ID {
				t.Fatal("create retry changed identity")
			}
			if page := getJSON[recipejob.ListRecipeJobsResponse](t, base+"/jobs"); len(page.Jobs) != 0 {
				t.Fatal("creation submitted work")
			}
			for i := 0; i < 3; i++ {
				postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": fmt.Sprintf("empty-%d", i), "repo": fmt.Sprintf("github.com/acme/empty-%d", i)})
			}
			seen := map[string]bool{}
			token := ""
			for {
				page := getJSON[jobdb.ListCellsResponse](t, base+"/cells?pageSize=1&pageToken="+url.QueryEscape(token))
				for _, cell := range page.Cells {
					if seen[cell.ID] {
						t.Fatal("duplicate cell")
					}
					seen[cell.ID] = true
				}
				token = page.NextPageToken
				if token == "" {
					break
				}
			}
			if len(seen) != 4 {
				t.Fatal(seen)
			}
			if page := getJSON[jobdb.ListCellsResponse](t, api.URL+"/api/projects/other/cells"); len(page.Cells) != 0 {
				t.Fatal("tenant leak")
			}
			req, _ := http.NewRequest("GET", base+"/cells/"+created.ID, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Header.Get("ETag") != fmt.Sprintf("%q", created.Revision) {
				t.Fatal("missing revision")
			}
			resp.Body.Close()
			// Local configuration maps alpha elsewhere. Name-only must use the registry.
			job := postJSON[recipejob.RecipeJob](t, base+"/jobs", map[string]any{"cell": "alpha", "recipe": "smoke", "job_id": "source"})
			if job.CellID != created.ID || job.RepositorySource != created.Repo {
				t.Fatalf("wrong target: %+v", job)
			}
			ctx := testjobdb.Context(context.Background())
			renamed := "renamed"
			repo := "github.com/acme/new-source"
			updated, err := srv.cells.UpdateCell(ctx, jobdb.UpdateCellRequest{CellKey: jobdb.CellKey{TenantID: "registry", CellID: created.ID}, IfMatch: fmt.Sprintf("%q", created.Revision), Name: &renamed, Repo: &repo})
			if err != nil {
				t.Fatal(err)
			}
			alias := getJSON[jobdb.ListCellsResponse](t, base+"/cells?name=alpha&state=ALL")
			if len(alias.Cells) != 1 || alias.Cells[0].ID != created.ID || alias.Cells[0].Name != renamed {
				t.Fatal(alias)
			}
			historic := getJSON[recipejob.RecipeJob](t, base+"/jobs/source")
			if historic.CellName != renamed || historic.RepositorySource != created.Repo {
				t.Fatalf("history rewritten: %+v", historic)
			}
			newer := postJSON[recipejob.RecipeJob](t, base+"/jobs", map[string]any{"cell": "alpha", "recipe": "smoke", "job_id": "new"})
			if newer.CellID != created.ID || newer.RepositorySource != repo {
				t.Fatal(newer)
			}
			page := getJSON[recipejob.ListRecipeJobsResponse](t, base+"/jobs?cellNames=renamed&status=all")
			if len(page.Jobs) != 2 {
				t.Fatal(page)
			}
			missing, _ := http.NewRequest("POST", base+"/jobs", strings.NewReader(`{"cell":"first-use","repo":"github.com/acme/first","recipe":"smoke"}`))
			doJSON[errorResponse](t, missing, http.StatusNotFound)
			if page := getJSON[jobdb.ListCellsResponse](t, base+"/cells?name=first-use"); len(page.Cells) != 0 {
				t.Fatal("job submission created a cell")
			}
			postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "first-use", "repo": "github.com/acme/first"})
			first := postJSON[recipejob.RecipeJob](t, base+"/jobs", map[string]any{"cell": "first-use", "repo": "github.com/acme/first", "recipe": "smoke"})
			if first.CellID == "" {
				t.Fatal(first)
			}
			empty := postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "retired", "repo": "github.com/acme/retired"})
			_, err = srv.cells.RetireCell(ctx, jobdb.RetireCellRequest{CellKey: jobdb.CellKey{TenantID: "registry", CellID: empty.ID}, IfMatch: fmt.Sprintf("%q", empty.Revision)})
			if err != nil {
				t.Fatal(err)
			}
			retired := getJSON[jobdb.ListCellsResponse](t, base+"/cells?state=RETIRED")
			if len(retired.Cells) != 1 || retired.Cells[0].ID != empty.ID {
				t.Fatal(retired)
			}
			for _, test := range []struct {
				body   string
				status int
				code   string
			}{
				{`{"cell":"renamed","repo":"github.com/acme/registered"}`, 409, "cell_repo_mismatch"},
				{`{"cell":"retired"}`, 409, "cell_retired"},
				{`{"cell":"https://github.com/acme/a"}`, 400, "invalid_request"},
				{`{"repo":"github.com/acme/a"}`, 400, "invalid_request"},
				{`{"cell":"root","repo":"github.com/acme/root"}`, 404, "not_found"},
			} {
				req, _ := http.NewRequest("POST", base+"/jobs", strings.NewReader(test.body))
				got := doJSON[errorResponse](t, req, test.status)
				if got.Code != test.code {
					t.Fatalf("error: %+v", got)
				}
			}
			ordinary := postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "root", "repo": "github.com/acme/root"})
			rootName := "ordinary"
			renamedRoot, err := srv.cells.UpdateCell(ctx, jobdb.UpdateCellRequest{CellKey: jobdb.CellKey{TenantID: "registry", CellID: ordinary.ID}, IfMatch: fmt.Sprintf("%q", ordinary.Revision), Name: &rootName})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := srv.cells.RetireCell(ctx, jobdb.RetireCellRequest{CellKey: jobdb.CellKey{TenantID: "registry", CellID: ordinary.ID}, IfMatch: fmt.Sprintf("%q", renamedRoot.Revision)}); err != nil {
				t.Fatal(err)
			}
			legacyFilter, _ := http.NewRequest("GET", base+"/cells?isRoot=true", nil)
			doJSON[errorResponse](t, legacyFilter, http.StatusBadRequest)
			_ = updated
		})
	}
}

// JobDB's cell-envelope validation must not run on job RPCs merely because a
// tenant or job ID starts with "cells". Exercise the actual remote SQLite path.
func TestCellRouteValidationDoesNotMatchTenantOrJobNames(t *testing.T) {
	embedded, err := sqliteruntime.StartEmbeddedRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer embedded.Shutdown()
	backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(embedded.Runtime)))
	defer backend.Close()
	srv, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	for _, tenant := range []string{"registry-example", "cells", "cells-example"} {
		t.Run(tenant, func(t *testing.T) {
			base := api.URL + "/api/projects/" + tenant
			cell := postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "example", "repo": "github.com/acme/example"})
			if page := getJSON[recipejob.ListRecipeJobsResponse](t, base+"/jobs?status=all"); len(page.Jobs) != 0 {
				t.Fatal("cell registration unexpectedly submitted a job")
			}
			if pending := getJSON[[]map[string]any](t, base+"/user-inputs/pending"); len(pending) != 0 {
				t.Fatal("unexpected pending inputs")
			}
			job := postJSON[recipejob.RecipeJob](t, base+"/jobs", map[string]string{"cell": "example", "recipe": "smoke", "job_id": "cells-job"})
			if job.CellID != cell.ID || job.TenantID != tenant {
				t.Fatalf("wrong submission ownership: %+v", job)
			}
			page := getJSON[recipejob.ListRecipeJobsResponse](t, base+"/jobs?status=all&cellNames=example")
			if len(page.Jobs) != 1 || page.Jobs[0].JobID != "cells-job" {
				t.Fatalf("wrong job list: %+v", page)
			}
			getJSON[recipejob.RecipeJob](t, base+"/jobs/cells-job")
			req, err := http.NewRequest(http.MethodPost, base+"/jobs/cells-job/cancel", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("cancel status: %d", response.StatusCode)
			}
			if cancelled := getJSON[recipejob.RecipeJob](t, base+"/jobs/cells-job"); cancelled.Status != jobdb.JobStatusCancelled {
				t.Fatalf("job was not cancelled: %+v", cancelled)
			}
		})
	}
}
