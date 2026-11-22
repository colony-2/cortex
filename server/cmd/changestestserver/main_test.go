package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/git/gitstate"
	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/c2j/pkg/jobinspect/changestest"
	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/stretchr/testify/require"
)

// The native browser fixture is also an HTTP regression: no response stubs,
// review patch attachments or live checkout participate in saved-byte reads.
func TestWorkspaceDiffHTTPUnchangedHandoffs(t *testing.T) {
	t.Chdir("../../..")
	for _, mode := range []string{"sqlite", "http-sqlite"} {
		t.Run(mode, func(t *testing.T) {
			ctx := testjobdb.Context(context.Background())
			db, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: filepath.Join(t.TempDir(), "jobs.db")})
			require.NoError(t, err)
			defer db.Close(ctx)
			var bundleReads atomic.Int64
			handler := testjobdb.Handler(remote.NewServer(db))
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, gitstate.ThinPackArtifactName) {
					bundleReads.Add(1)
				}
				handler.ServeHTTP(w, r)
			}))
			defer backend.Close()
			var rt jobdb.WorkflowRuntime = db
			if mode == "http-sqlite" {
				rt, err = remote.New(backend.URL, backend.Client())
				require.NoError(t, err)
			}
			_, err = rt.(jobdb.CellRuntime).CreateCell(ctx, jobdb.CreateCellRequest{TenantID: tenant, Name: "changes", Repo: "example.test/changes"})
			require.NoError(t, err)
			doc := jobdb.NewArtifactFromBytes("overview.md", []byte("# Review\n"))
			_, err = db.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: "documents", JobType: "source", Cell: "changes", CellRepo: "example.test/changes", Data: jobdb.NewTaskDataOrPanic(nil, doc)}})
			require.NoError(t, err)
			key, err := doc.ArtifactKey()
			require.NoError(t, err)
			s, err := cortex.NewRemoteServer(cortex.Config{DefaultTenantID: tenant, JobDBURL: backend.URL})
			require.NoError(t, err)
			api := httptest.NewServer(testjobdb.Handler(s))
			defer api.Close()
			get := func(path string) []byte {
				t.Helper()
				response, e := http.Get(api.URL + path)
				require.NoError(t, e)
				defer response.Body.Close()
				data, e := io.ReadAll(response.Body)
				require.NoError(t, e)
				require.Equal(t, 200, response.StatusCode, "%s", data)
				return data
			}
			for _, scenario := range []string{"v2-handoff-control", "v2-handoff"} {
				t.Run(scenario, func(t *testing.T) {
					dir := t.TempDir()
					f, e := seed(ctx, rt, dir, scenario, artifacts.NewStoredRef(key))
					require.NoError(t, e)
					_, e = os.Stat(filepath.Join(dir, scenario, "checkout"))
					require.True(t, os.IsNotExist(e), "writer checkout must be deleted")
					forbidden := t.TempDir()
					sentinel := filepath.Join(forbidden, "git-called")
					require.NoError(t, os.WriteFile(filepath.Join(forbidden, "git"), []byte("#!/bin/sh\nprintf called > '"+sentinel+"'\nexit 97\n"), 0700))
					t.Setenv("PATH", forbidden+string(os.PathListSeparator)+os.Getenv("PATH"))
					before, e := changestest.Inventory(ctx, rt, jobdb.JobKey{TenantId: tenant, JobId: f.Job})
					require.NoError(t, e)
					initialBundleReads := bundleReads.Load()
					root := "/api/projects/" + tenant
					details := get(root + "/user-inputs/" + f.Job)
					var form struct {
						Form struct {
							Request struct {
								Documents map[string]artifacts.Ref `json:"documents"`
							} `json:"request"`
						} `json:"form"`
					}
					require.NoError(t, json.Unmarshal(details, &form))
					require.Len(t, form.Form.Request.Documents, 2)
					for _, ref := range form.Form.Request.Documents {
						require.Equal(t, "overview.md", ref.Name)
					}
					type preview struct {
						Metadata        *jobinspect.WorkspaceDiff `json:"metadata"`
						Patch           string                    `json:"patch"`
						DownloadURL     string                    `json:"download_url"`
						PreviewComplete bool                      `json:"preview_complete"`
					}
					read := func(query string) preview {
						t.Helper()
						var p preview
						require.NoError(t, json.Unmarshal(get(root+"/jobs/"+f.Job+"/workspace-diff"+query), &p))
						return p
					}
					original := read("?task=" + url.QueryEscape(f.SourceTask))
					require.Equal(t, "available", original.Metadata.Status, "%+v", original.Metadata.Reasons)
					require.NotEqual(t, f.SourceTask, f.StateTask)
					require.Equal(t, f.Base, original.Metadata.Workspace.BaseCommit)
					require.Equal(t, f.Head, original.Metadata.Workspace.HeadCommit)
					require.Equal(t, f.All, original.Patch)
					require.Equal(t, f.SourceTask, original.Metadata.Source.Producer.Task)
					require.Equal(t, f.All, string(get(original.DownloadURL)))
					t.Logf("original producer %s, key %+v, oracle %d bytes", f.SourceTask, original.Metadata.Source.Key, len(f.All))
					for _, selection := range []struct{ name, query string }{{"latest", ""}, {"selected", "?task=" + url.QueryEscape(f.StateTask)}} {
						t.Run(selection.name, func(t *testing.T) {
							query := selection.query
							p := read(query)
							t.Logf("query %q: status=%s reasons=%v workspace=%+v cutoff=%s", query, p.Metadata.Status, p.Metadata.Reasons, p.Metadata.Workspace, p.Metadata.Cutoff)
							require.Equal(t, "available", p.Metadata.Status, "%+v", p.Metadata.Reasons)
							require.Equal(t, original.Metadata.Workspace, p.Metadata.Workspace)
							require.Equal(t, f.All, p.Patch)
							require.True(t, p.PreviewComplete)
							require.Equal(t, f.StateTask, p.Metadata.StateProducer.Task)
							require.Equal(t, original.Metadata.Source, p.Metadata.Source)
							require.Equal(t, original.DownloadURL, p.DownloadURL)
							require.Equal(t, f.All, string(get(p.DownloadURL)))
						})
					}
					require.JSONEq(t, string(details), string(get(root+"/user-inputs/"+f.Job)))
					// Inventory itself opens every retained file to compare bytes.
					// Count only the saved-diff and review HTTP reads above.
					require.Equal(t, initialBundleReads, bundleReads.Load(), "saved reads opened a bundle")
					after, e := changestest.Inventory(ctx, rt, jobdb.JobKey{TenantId: tenant, JobId: f.Job})
					require.NoError(t, e)
					require.Equal(t, strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
					_, e = os.Stat(sentinel)
					require.True(t, os.IsNotExist(e), "saved read invoked Git")
				})
			}
		})
	}
}
