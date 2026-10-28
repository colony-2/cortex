package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	"github.com/gorilla/mux"
)

type chapterReaderFunc func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error)

func (f chapterReaderFunc) ListChapters(ctx context.Context, req jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
	return f(ctx, req)
}

type brokenStoryService struct{ storyService }

func (s brokenStoryService) GetJobRunStory(context.Context, story.GetJobRunStoryRequest) (*story.JobRunStory, error) {
	return nil, errors.New("replay unavailable")
}

func TestChapterDownloadRuntimeIntegration(t *testing.T) {
	for _, mode := range []string{"toy", "remote"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t)}
			var srv *Server
			var err error
			if mode == "remote" {
				backend := httptest.NewServer(remote.NewServer(toy.New()))
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
			ctx := context.Background()
			for _, tenant := range []string{"a", "b"} {
				postJSON[map[string]any](t, api.URL+"/api/projects/"+tenant+"/jobs", map[string]any{"job_id": "same", "recipe": "smoke"})
			}
			path := api.URL + "/api/projects/a/jobs/same/chapters/download"
			download := func() chapterExport {
				t.Helper()
				resp, err := http.Get(path)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				data, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != 200 {
					t.Fatalf("download %d: %s", resp.StatusCode, data)
				}
				disposition, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
				if err != nil || disposition != "attachment" || params["filename"] != "job-same-chapters.json" || resp.Header.Get("Content-Type") != "application/json" || resp.Header.Get("Cache-Control") != "no-store" {
					t.Fatalf("headers: %v, %v", resp.Header, err)
				}
				var export chapterExport
				if err := json.Unmarshal(data, &export); err != nil {
					t.Fatal(err)
				}
				if export.FormatVersion != 1 || export.ProjectID != "a" || export.JobID != "same" || export.ExportedAt.IsZero() {
					t.Fatalf("envelope: %s", data)
				}
				for i, c := range export.Chapters {
					if i > 0 && export.Chapters[i-1].Ordinal >= c.Ordinal {
						t.Fatal("duplicate or unordered ordinal")
					}
				}
				return export
			}
			srv.story = brokenStoryService{srv.story}
			storyResponse, err := http.Get(api.URL + "/api/projects/a/jobs/same/story")
			if err != nil {
				t.Fatal(err)
			}
			storyResponse.Body.Close()
			if storyResponse.StatusCode != 500 {
				t.Fatalf("expected replay failure, got %d", storyResponse.StatusCode)
			}
			initial := download()
			if len(initial.Chapters) != 1 || initial.Chapters[0].Body.(map[string]any)["kind"] != "job_start" {
				t.Fatalf("initial chapters: %#v", initial.Chapters)
			}
			lease, err := srv.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: "same"}, WorkerID: "export-test", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
			if err != nil || lease == nil {
				t.Fatalf("lease: %v", err)
			}
			defer lease.StopKeepAlive()
			runtime := srv.engine.(jobdbschema.WorkflowEngine).Registry.(jobdb.WorkflowRuntime)
			child, err := lease.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "a", JobID: "child", JobType: "child-export-test", Data: jobdb.NewTaskDataOrPanic(map[string]any{"child_only": "excluded"})}, RequestTime: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			children, err := runtime.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"a"}, ParentJobIDs: []string{"same"}})
			if err != nil || len(children.Jobs) != 1 || children.Jobs[0].JobKey != child.JobKey {
				t.Fatalf("child fixture: %#v, %v", children, err)
			}
			childChapters, err := runtime.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: child.JobKey})
			if err != nil || len(childChapters) != 1 {
				t.Fatalf("child chapters: %#v, %v", childChapters, err)
			}

			token := ""
			if l, ok := lease.(interface{ LeaseToken() string }); ok {
				token = l.LeaseToken()
			}
			records := []json.RawMessage{
				json.RawMessage(`{"version":1,"kind":"header","versions":{"execution_pattern_version":1,"jobdb_version":"test"}}`),
				json.RawMessage(`{"version":1,"kind":"restart","id":"retained","descriptor":[{"version":1,"kind":"scope","id":"old"}]}`),
			}
			for i, record := range records {
				ordinal := int64(i + 1)
				err := runtime.PutChapter(ctx, jobdb.PutChapterRequest{LeaseID: lease.LeaseID(), LeaseToken: token, Ref: jobdb.ChapterRef{JobKey: jobdb.JobKey{TenantId: "a", JobId: "same"}, Ordinal: ordinal}, Chapter: jobdb.Chapter{Ordinal: ordinal, TaskType: "__execution__", CreatedAt: time.Now().UTC(), Body: &jobdb.ExecutionChapter{Record: record}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			stored, err := runtime.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: jobdb.JobKey{TenantId: "a", JobId: "same"}})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "toy" {
				if len(stored) != 3 {
					t.Fatalf("stored chapters: %d", len(stored))
				}
				if _, ok := stored[1].Body.(*jobdb.ExecutionChapter); !ok {
					t.Fatalf("pointer fixture normalized unexpectedly: %T", stored[1].Body)
				}
			}
			running := download()
			if len(running.Chapters) != 3 {
				t.Fatalf("running chapter count: %d", len(running.Chapters))
			}
			artifact := jobdb.NewArtifactFromBytes("雪.txt", []byte("artifact-body-must-not-be-exported"))
			err = lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: "success", Chapter: &jobdb.Chapter{Ordinal: 3, TaskType: "recipe", CreatedAt: time.Now().UTC(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"result":"雪","large":9007199254740993}`)}}}}, ArtifactUploads: []jobdb.ArtifactUpload{{Name: artifact.Name(), Size: artifact.Size(), Open: artifact.Open}}})
			if err != nil {
				t.Fatal(err)
			}
			completed := download()
			if len(completed.Chapters) != 4 || completed.Chapters[3].Artifacts[0].Name != "雪.txt" {
				t.Fatalf("completed chapters: %#v", completed.Chapters)
			}
			raw, _ := json.Marshal(completed)
			if strings.Contains(string(raw), "artifact-body-must-not-be-exported") {
				t.Fatal("artifact body included")
			}
			other := getJSON[chapterExport](t, api.URL+"/api/projects/b/jobs/same/chapters/download")
			if len(other.Chapters) != 1 || other.ProjectID != "b" {
				t.Fatalf("tenant leak: %#v", other)
			}
			for _, suffix := range []string{"missing/jobs/same", "a/jobs/missing"} {
				resp, err := http.Get(api.URL + "/api/projects/" + suffix + "/chapters/download")
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != 404 || resp.Header.Get("Content-Disposition") != "" {
					t.Fatalf("guard: %v", resp)
				}
			}
			req, _ := http.NewRequest(http.MethodDelete, api.URL+"/api/projects/a/jobs/same", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			resp, err = http.Get(path)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 404 || resp.Header.Get("Content-Disposition") != "" {
				t.Fatalf("deleted: %v", resp)
			}
		})
	}
}

func TestChapterDownloadFailureAndContext(t *testing.T) {
	key := jobdb.JobKey{TenantId: "tenant", JobId: "job"}
	for _, tc := range []struct {
		name   string
		reader chapterReader
		want   int
	}{
		{"unavailable", nil, 500},
		{"read failure", chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
			return nil, errors.New("reader failed")
		}), 500},
		{"serialization failure", chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
			return []jobdb.Chapter{{Body: jobdb.ExecutionChapter{Record: json.RawMessage(`{`)}}}, nil
		}), 500},
		{"typed nil body", chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
			return []jobdb.Chapter{{Body: (*jobdb.ExecutionChapter)(nil)}}, nil
		}), 500},
		{"typed nil outcome", chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
			return []jobdb.Chapter{{Body: &jobdb.JobAttemptOutcomeChapter{Outcome: (*jobdb.ApplicationOutputOutcome)(nil)}}}, nil
		}), 500},
		{"empty", chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) { return nil, nil }), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{chapters: tc.reader}
			req := mux.SetURLVars(httptest.NewRequest("GET", "/", nil), map[string]string{"projectId": key.TenantId, "jobId": key.JobId})
			w := httptest.NewRecorder()
			s.handleChapterDownload(w, req)
			if w.Code != tc.want {
				t.Fatalf("status: %d: %s", w.Code, w.Body)
			}
			if tc.want != 200 && (w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), `"chapters"`)) {
				t.Fatalf("partial export: %s", w.Body)
			}
			if tc.want == 200 && !strings.Contains(w.Body.String(), `"chapters": []`) {
				t.Fatalf("empty: %s", w.Body)
			}
		})
	}
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &Server{chapters: chapterReaderFunc(func(got context.Context, req jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
		calls++
		if got.Err() != context.Canceled || req.JobKey != key || req.StartOrdinal != 0 || req.EndOrdinal != nil {
			t.Fatalf("request: %#v", req)
		}
		return nil, got.Err()
	})}
	req := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(ctx), map[string]string{"projectId": key.TenantId, "jobId": key.JobId})
	s.handleChapterDownload(httptest.NewRecorder(), req)
	if calls != 1 {
		t.Fatalf("reader calls: %d", calls)
	}
}
