package cortex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/c2j/pkg/jobinspect/changestest"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/stretchr/testify/require"
)

func TestRecordedChangesHTTP(t *testing.T) {
	for _, mode := range []string{"sqlite", "http-sqlite"} {
		t.Run(mode, func(t *testing.T) {
			ctx := testjobdb.Context(context.Background())
			db, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: filepath.Join(t.TempDir(), "changes.db")})
			require.NoError(t, err)
			defer db.Close(ctx)
			backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(db)))
			defer backend.Close()
			var rt jobdb.WorkflowRuntime = db
			if mode == "http-sqlite" {
				rt, err = remote.New(backend.URL, backend.Client())
				require.NoError(t, err)
			}
			_, err = rt.(jobdb.CellRuntime).CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "changes", Repo: "example.test/changes"})
			require.NoError(t, err)
			s, err := NewRemoteServer(Config{DefaultTenantID: "tenant", JobDBURL: backend.URL})
			require.NoError(t, err)
			// Changes has its own read API and must not depend on lease history.
			s.leaseEvents = nil
			api := httptest.NewServer(s)
			defer api.Close()
			for _, scenario := range []string{"linear", "revert", "merge", "unchanged", "missing-closure"} {
				t.Run(scenario, func(t *testing.T) {
					f, err := changestest.Build(ctx, rt, changestest.Options{Directory: t.TempDir(), Commits: 4, FastForward: scenario == "linear", Revert: scenario == "revert", MergeAt: map[string]int{"merge": 3}[scenario], Published: scenario == "unchanged", OmitBaseBundle: scenario == "missing-closure"})
					require.NoError(t, err)
					if scenario == "unchanged" {
						f.Base = f.Head
					}
					oracle, err := f.Oracle(ctx, f.Base, f.Head)
					require.NoError(t, err)
					firstOracle, err := f.Oracle(ctx, f.Base, f.Commits[0])
					require.NoError(t, err)
					rangeOracle, err := f.Oracle(ctx, f.Commits[0], f.Commits[3])
					require.NoError(t, err)
					require.NoError(t, f.DeleteCheckout())
					before, err := changestest.Inventory(ctx, rt, f.Job)
					require.NoError(t, err)
					defer func() {
						after, err := changestest.Inventory(ctx, rt, f.Job)
						require.NoError(t, err)
						require.Equal(t, string(before), string(after))
					}()
					root := api.URL + "/api/projects/tenant/jobs/" + f.Job.JobId + "/changes"
					get := func(path string, status int) []byte {
						t.Helper()
						response, err := http.Get(path)
						require.NoError(t, err)
						defer response.Body.Close()
						body, err := io.ReadAll(response.Body)
						require.NoError(t, err)
						require.Equal(t, status, response.StatusCode, "%s", body)
						require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
						return body
					}
					if scenario == "missing-closure" {
						body := get(root, 422)
						require.Contains(t, string(body), `"missing_closure"`)
						return
					}
					var history jobinspect.ChangesHistory
					require.NoError(t, json.Unmarshal(get(root+"?limit=1", 200), &history))
					require.Equal(t, f.Head, history.Head)
					require.Equal(t, f.Base, history.Base)
					if scenario == "merge" {
						require.False(t, history.Complete)
						require.Empty(t, history.Commits)
						require.Equal(t, "unsupported_history", history.Diagnostic.Code)
					} else if scenario == "unchanged" {
						require.True(t, history.Complete)
						require.Zero(t, history.Total)
					} else {
						require.True(t, history.Complete)
						require.Equal(t, 4, history.Total)
						rows := append([]jobinspect.ChangesCommit{}, history.Commits...)
						cursor := history.NextCursor
						for cursor != "" {
							var page jobinspect.ChangesHistory
							require.NoError(t, json.Unmarshal(get(root+"?limit=1&cursor="+url.QueryEscape(cursor), 200), &page))
							require.Equal(t, history.Selection, page.Selection)
							rows = append(rows, page.Commits...)
							cursor = page.NextCursor
						}
						for i, commit := range rows {
							require.Equal(t, f.Commits[i], commit.Hash)
							require.Equal(t, i+1, commit.Position)
						}
					}
					prefix := root + "/diff?selection=" + url.QueryEscape(history.Selection)
					var preview struct {
						Metadata jobinspect.ChangesDiff `json:"metadata"`
						Patch    string                 `json:"patch"`
						Complete bool                   `json:"preview_complete"`
					}
					require.NoError(t, json.Unmarshal(get(prefix+"&all=true", 200), &preview))
					require.True(t, preview.Complete)
					require.Equal(t, string(oracle), preview.Patch)
					require.Equal(t, oracle, get(prefix+"&all=true&format=patch", 200))
					require.Equal(t, f.Base, preview.Metadata.FromCommit)
					require.Equal(t, f.Head, preview.Metadata.ToCommit)
					if scenario == "linear" || scenario == "revert" {
						require.Equal(t, firstOracle, get(prefix+"&commit="+f.Commits[0]+"&format=patch", 200))
						require.Equal(t, rangeOracle, get(prefix+"&from="+f.Commits[1]+"&through="+f.Commits[3]+"&format=patch", 200))
						get(prefix+"&from="+f.Commits[3]+"&through="+f.Commits[1], 400)
						get(prefix+"&commit="+f.Base, 400)
					}
					if scenario == "merge" {
						require.Contains(t, string(get(prefix+"&commit="+f.Head, 422)), `"unsupported_history"`)
					}
					for _, path := range []string{"?repository=evil", "?scope=child", "?limit=1&limit=2", "?artifact_key=secret"} {
						get(root+path, 400)
					}
					get(root+"/diff?all=true", 400)
					response, err := http.Get(api.URL + "/api/projects/other/jobs/" + f.Job.JobId + "/changes")
					require.NoError(t, err)
					require.Equal(t, 403, response.StatusCode)
					response.Body.Close()
				})
			}
		})
	}
}
