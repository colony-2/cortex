package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/c2j/pkg/jobinspect/changestest"
	"github.com/colony-2/c2j/pkg/jobinspect/workspacedifftest"
	coretask "github.com/colony-2/c2j/pkg/task"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceDiffHTTPNative(t *testing.T) {
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
			_, err = rt.(jobdb.CellRuntime).CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "saved-tests", Name: "saved", Repo: "example.test/saved"})
			require.NoError(t, err)
			s, err := NewRemoteServer(Config{DefaultTenantID: "saved-tests", JobDBURL: backend.URL})
			require.NoError(t, err)
			// Exercise the chosen backend directly, independently of lease history.
			s.configureChanges(rt)
			s.leaseEvents = nil
			api := httptest.NewServer(testjobdb.Handler(s))
			defer api.Close()
			get := func(path string, status int) []byte {
				t.Helper()
				response, err := http.Get(api.URL + path)
				require.NoError(t, err)
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.Equal(t, status, response.StatusCode, "%s", body)
				return body
			}
			for _, scenario := range []string{"available", "missing", "empty"} {
				t.Run(scenario, func(t *testing.T) {
					f, err := workspacedifftest.BuildWithOptions(ctx, rt, t.TempDir(), workspacedifftest.Options{Scope: true, EmptyComparison: scenario == "empty", Transform: func(steps []workspacedifftest.Step) []workspacedifftest.Step {
						if scenario == "missing" {
							steps[2].Omit = true
						}

						return steps
					}})
					require.NoError(t, err)
					// Git and live checkouts are not available to the read path.
					forbidden := t.TempDir()
					sentinel := filepath.Join(forbidden, "git-called")
					require.NoError(t, os.WriteFile(filepath.Join(forbidden, "git"), []byte("#!/bin/sh\nprintf called > '"+sentinel+"'\nexit 97\n"), 0700))
					t.Setenv("PATH", forbidden+string(os.PathListSeparator)+os.Getenv("PATH"))
					defer func() {
						_, err := os.Stat(sentinel)
						require.True(t, os.IsNotExist(err), "saved-diff read invoked Git")
					}()
					before, err := changestest.Inventory(ctx, rt, f.Job)
					require.NoError(t, err)
					root := "/api/projects/saved-tests/jobs/" + f.Job.JobId
					read := func(query string) workspaceDiffPreview {
						t.Helper()
						var p workspaceDiffPreview
						require.NoError(t, json.Unmarshal(get(root+"/workspace-diff"+query, 200), &p))
						return p
					}
					p := read("")
					require.Equal(t, map[string]string{"available": "available", "missing": "unavailable", "empty": "empty"}[scenario], p.Metadata.Status)
					if scenario == "available" {
						require.Equal(t, string(f.Steps[2].Base), p.Patch)
						require.True(t, p.PreviewComplete)
						require.Contains(t, p.DownloadURL, "/artifacts/diff_from_base.diff")
						// A before-boundary and task pin resolve earlier original bytes.
						service := &jobinspect.Service{Runtime: rt, Tenant: "saved-tests"}
						tasks, err := service.Inspect(ctx, jobinspect.Request{Operation: "log", JobID: f.Job.JobId, Limit: 100})
						require.NoError(t, err)
						require.Len(t, tasks.Tasks, 4)
						task := tasks.Tasks[2].ID
						query := "?task=" + url.QueryEscape(task) + "&attempt=1&boundary=before&workspace=root"
						earlier := read(query)
						require.Equal(t, string(f.Steps[1].Base), earlier.Patch)
						originalURL := earlier.DownloadURL
						require.Equal(t, task, earlier.Metadata.Resolved.Task)
						require.Equal(t, "before", earlier.Metadata.Resolved.Boundary)
						_ = read("")
						require.Equal(t, originalURL, read(query).DownloadURL)
						scopes, err := service.Inspect(ctx, jobinspect.Request{Operation: "story", JobID: f.Job.JobId, Limit: 100})
						require.NoError(t, err)
						require.NotEmpty(t, scopes.Scopes)
						selected := read("?scope=" + url.QueryEscape(scopes.Scopes[len(scopes.Scopes)-1].ID) + "&attempt=1")
						require.Equal(t, p.Patch, selected.Patch)
						entry := read("?task=" + url.QueryEscape(tasks.Tasks[0].ID) + "&boundary=before")
						require.Equal(t, "empty", entry.Metadata.Status)
						require.Empty(t, entry.DownloadURL)
					} else if scenario == "empty" {
						require.NotNil(t, p.Metadata.Source)
						require.Contains(t, p.DownloadURL, "/artifacts/diff_from_base.diff")
						require.Empty(t, p.Patch)
					} else {
						require.Empty(t, p.DownloadURL)
						require.Empty(t, p.Patch)
					}
					for _, query := range []string{"?task=unknown", "?workspace=unknown", "?attempt=0", "?task=a&scope=b", "?boundary=after", "?task=", "?attempt=-1", "?attempt=1", "?attempt=1&task=a&attempt=2", "?all=true", "?base=x", "?format=patch", "?boundary=%zz"} {
						get(root+"/workspace-diff"+query, 400)
					}
					get(root+"/changes", 410)
					get(root+"/changes/diff?all=true", 410)
					get(strings.Replace(root, "saved-tests", "other", 1)+"/workspace-diff", 403)
					if p.DownloadURL != "" {
						get(strings.Replace(p.DownloadURL, "saved-tests", "other", 1), 404)
					}
					after, err := changestest.Inventory(ctx, rt, f.Job)
					require.NoError(t, err)
					require.Equal(t, string(before), string(after))
				})
			}
		})
	}
}

type workspaceDiffFunc func(context.Context, jobinspect.WorkspaceDiffRequest) (*jobinspect.WorkspaceDiff, io.ReadCloser, error)

func (f workspaceDiffFunc) OpenWorkspaceDiff(ctx context.Context, q jobinspect.WorkspaceDiffRequest) (*jobinspect.WorkspaceDiff, io.ReadCloser, error) {
	return f(ctx, q)
}

type trackedDiffBody struct {
	io.Reader
	closed   bool
	closeErr error
}

func (b *trackedDiffBody) Close() error { b.closed = true; return b.closeErr }

func TestWorkspaceDiffHTTPForwardingAndFailures(t *testing.T) {
	for _, scenario := range []string{"selected", "large", "close", "read", "denied", "deadline", "unsupported", "unavailable", "malformed", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			called := false
			payload := "original saved bytes"
			if scenario == "large" {
				payload = strings.Repeat("x", changesPreviewBytes+128)
			}
			body := &trackedDiffBody{Reader: strings.NewReader(payload)}
			if scenario == "close" {
				body.closeErr = errors.New("close failed")
			}
			if scenario == "read" {
				body.Reader = io.MultiReader(strings.NewReader("partial"), errorReader{})
			}
			s := &Server{workspaceDiff: func(tenant string) (jobinspect.WorkspaceDiffAPI, error) {
				require.Equal(t, "tenant", tenant)
				return workspaceDiffFunc(func(ctx context.Context, q jobinspect.WorkspaceDiffRequest) (*jobinspect.WorkspaceDiff, io.ReadCloser, error) {
					called = true
					require.Equal(t, `/scope:"x/μ"#2`, *q.Scope)
					require.Equal(t, 2, *q.Attempt)
					require.Equal(t, "before", *q.Boundary)
					require.Equal(t, "cell/a", *q.Workspace)
					if scenario == "cancel" {
						require.ErrorIs(t, ctx.Err(), context.Canceled)
						return nil, body, ctx.Err()
					}
					if scenario == "denied" {
						return nil, nil, jobinspect.WorkspaceDiffDiagnostic("denied", "secret foreign producer", nil)
					}
					if scenario == "deadline" {
						return nil, nil, context.DeadlineExceeded
					}
					if scenario == "unsupported" {
						return nil, nil, jobinspect.WorkspaceDiffDiagnostic("unsupported", "Upgrade helper", nil)
					}
					n := int64(len(payload))
					key := jobdb.ArtifactKey{JobId: "producer/a", TaskOrdinal: 7, Name: "diff_from_base.diff", SizeBytes: n}
					producer := &jobinspect.WorkspaceDiffProducer{JobID: "job", Task: "state", Attempt: 1}
					m := &jobinspect.WorkspaceDiff{Version: 1, Status: "available", Requested: q, Resolved: jobinspect.WorkspaceDiffPoint{Scope: *q.Scope, Attempt: 2, Status: "succeeded", Boundary: "before", Workspace: "cell/a"}, Observation: "observation", Cutoff: "cutoff", Workspace: &coretask.WorkspaceState{Repository: "example.test/repo", ScopeID: "cell/a", BaseCommit: strings.Repeat("1", 40), HeadCommit: strings.Repeat("2", 40)}, StateProducer: producer, Source: &jobinspect.WorkspaceDiffSource{Producer: jobinspect.WorkspaceDiffProducer{JobID: key.JobId, Task: "source", Attempt: 1}, Key: key}, Bytes: &n}
					if scenario == "unavailable" {
						m.Status = "unavailable"
						m.Source = nil
						m.Bytes = nil
						m.Reasons = []string{"missing evidence"}
						return m, nil, nil
					}
					if scenario == "malformed" {
						m.Version = 99
					}
					return m, body, nil
				}), nil
			}}
			query := url.Values{"scope": {`/scope:"x/μ"#2`}, "attempt": {"2"}, "workspace": {"cell/a"}, "boundary": {"before"}}
			request := mux.SetURLVars(httptest.NewRequest("GET", "/workspace-diff?"+query.Encode(), nil), map[string]string{"projectId": "tenant", "jobId": "job"})
			if scenario == "cancel" {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			s.handleWorkspaceDiff(w, request)
			require.True(t, called)
			status := 200
			switch scenario {
			case "close", "read", "malformed", "cancel":
				status = 502
			case "denied":
				status = 403
			case "deadline":
				status = 504
			case "unsupported":
				status = 501
			}
			require.Equal(t, status, w.Code, w.Body.String())
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			if scenario == "selected" || scenario == "large" {
				var p workspaceDiffPreview
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
				require.Equal(t, "/api/projects/tenant/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff", p.DownloadURL)
				require.Equal(t, scenario != "large", p.PreviewComplete)
				require.Equal(t, payload[:min(len(payload), changesPreviewBytes)], p.Patch)
			}
			if scenario == "denied" {
				require.NotContains(t, w.Body.String(), "secret foreign")
			}
			if scenario != "denied" && scenario != "deadline" && scenario != "unsupported" && scenario != "unavailable" {
				require.True(t, body.closed)
			}
		})
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// The browser tests consume this same fixture, so URL encoding changes must
// remain compatible with actual responses from the saved-diff HTTP handler.
func TestWorkspaceDiffHTTPPathEncoding(t *testing.T) {
	raw, err := os.ReadFile("testdata/workspace-diff-paths.json")
	require.NoError(t, err)
	var fixtures []struct {
		Tenant      string `json:"tenant"`
		Producer    string `json:"producer"`
		DownloadURL string `json:"download_url"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	for _, f := range fixtures {
		t.Run(f.Tenant, func(t *testing.T) {
			body := &trackedDiffBody{Reader: strings.NewReader("patch")}
			s := &Server{workspaceDiff: func(tenant string) (jobinspect.WorkspaceDiffAPI, error) {
				require.Equal(t, f.Tenant, tenant)
				return workspaceDiffFunc(func(_ context.Context, q jobinspect.WorkspaceDiffRequest) (*jobinspect.WorkspaceDiff, io.ReadCloser, error) {
					n := int64(5)
					return &jobinspect.WorkspaceDiff{
						Version: 1, Status: "available", Requested: q,
						Resolved:    jobinspect.WorkspaceDiffPoint{Status: "persisted", Boundary: "after", Workspace: "main"},
						Observation: "obs", Cutoff: "cutoff",
						Workspace:     &coretask.WorkspaceState{Repository: "example/repo", ScopeID: "main", BaseCommit: strings.Repeat("1", 40), HeadCommit: strings.Repeat("2", 40)},
						StateProducer: &jobinspect.WorkspaceDiffProducer{JobID: q.JobID, Task: "state", Attempt: 1},
						Source: &jobinspect.WorkspaceDiffSource{
							Producer: jobinspect.WorkspaceDiffProducer{JobID: f.Producer, Task: "source", Attempt: 1},
							Key:      jobdb.ArtifactKey{JobId: f.Producer, TaskOrdinal: 7, Name: "diff_from_base.diff", SizeBytes: n},
						}, Bytes: &n,
					}, body, nil
				}), nil
			}}
			r := mux.SetURLVars(httptest.NewRequest("GET", "/workspace-diff", nil), map[string]string{"projectId": f.Tenant, "jobId": "j"})
			w := httptest.NewRecorder()
			s.handleWorkspaceDiff(w, r)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var p workspaceDiffPreview
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
			require.Equal(t, f.DownloadURL, p.DownloadURL)
			require.Equal(t, "patch", p.Patch)
			require.True(t, body.closed)
		})
	}
}

func TestWorkspaceDiffInvalidSelectorsBeforeReader(t *testing.T) {
	s := &Server{workspaceDiff: func(string) (jobinspect.WorkspaceDiffAPI, error) {
		t.Fatal("invalid query reached the reader")
		return nil, nil
	}}
	for _, query := range []string{"task=a&scope=b", "attempt=0&task=a", "boundary=after", "task=", "workspace=", "attempt=1.5&scope=x", "scope=a&scope=b", "base=evil", "boundary=%zz"} {
		r := mux.SetURLVars(httptest.NewRequest("GET", "/workspace-diff?"+query, nil), map[string]string{"projectId": "tenant", "jobId": "job"})
		w := httptest.NewRecorder()
		s.handleWorkspaceDiff(w, r)
		require.Equal(t, 400, w.Code, query)
	}
	for _, path := range []string{"/changes", "/changes/diff?all=true"} {
		w := httptest.NewRecorder()
		s.handleRetiredChanges(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 410, w.Code)
	}
}
