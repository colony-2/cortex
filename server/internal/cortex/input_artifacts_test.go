package cortex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// An external native submitter need not use Cortex's multipart submission API.
func TestInputArtifactsExternalSubmissionHTTP(t *testing.T) {
	s, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(s)
	defer api.Close()
	base := api.URL + "/api/projects/external"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "self", "repo": "github.com/acme/self"})
	name := "docs/雪 space ?#%+.md"
	content := "# External submission\n"
	_, err = s.engine.SubmitJob(testjobdb.Context(context.Background()), jobdb.SubmitJob{
		TenantId: "external", JobID: "native", JobType: "recipe", Cell: "self", CellRepo: "github.com/acme/self",
		Data: jobdb.NewTaskDataOrPanic(map[string]any{"tenantId": "external", "cell": "self", "recipe": "test", "context": map[string]any{}, "inputs": map[string]any{"reference": "excluded.md"}}, jobdb.NewArtifactFromBytes(name, []byte(content))),
	})
	if err != nil {
		t.Fatal(err)
	}
	inventory := getJSON[struct {
		Artifacts []inputArtifact `json:"artifacts"`
	}](t, base+"/jobs/native/input-artifacts")
	if !reflect.DeepEqual(inventory.Artifacts, []inputArtifact{{"native", 0, name, int64(len(content))}}) {
		t.Fatalf("external inventory: %#v", inventory)
	}
	response, err := http.Get(base + "/jobs/native/tasks/0/artifacts/" + url.PathEscape(name))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(data) != content {
		t.Fatalf("external bytes: status=%d data=%q error=%v", response.StatusCode, data, err)
	}
}

func TestInputArtifactsSelection(t *testing.T) {
	for _, body := range []jobdb.ChapterBody{jobdb.JobStartChapter{}, &jobdb.JobStartChapter{}} {
		start := jobdb.Chapter{Ordinal: 7, Body: body, Artifacts: []jobdb.StoredArtifact{{Name: "docs/雪 ?#.md", Size: 120}}}
		chapters := []jobdb.Chapter{{Ordinal: 1, Body: jobdb.RestartExtraChapter{}, Artifacts: start.Artifacts}, start, {Ordinal: 8, Body: jobdb.ExecutionChapter{}, Artifacts: start.Artifacts}}
		got, err := inputArtifacts("job", chapters)
		if err != nil || !reflect.DeepEqual(got, []inputArtifact{{"job", 7, "docs/雪 ?#.md", 120}}) {
			t.Fatalf("%#v %v", got, err)
		}
		if _, err = inputArtifacts("job", append(chapters, start)); err == nil {
			t.Fatal("accepted duplicate start")
		}
	}
	for _, chapters := range [][]jobdb.Chapter{nil, {{Body: jobdb.ExecutionChapter{}}}, {{Body: jobdb.JobStartChapter{}}}} {
		got, err := inputArtifacts("job", chapters)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("empty inventory: %#v %v", got, err)
		}
	}
}

func TestInputArtifactsHandlerContextAndErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := &Server{chapters: chapterReaderFunc(func(received context.Context, req jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
			if received.Err() != context.Canceled || req.JobKey != (jobdb.JobKey{TenantId: "tenant", JobId: "job"}) {
				t.Fatal("lost context or tenant")
			}
			if fail {
				return nil, errors.New("reader failed")
			}
			return nil, nil
		})}
		req := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "job"})
		w := httptest.NewRecorder()
		s.handleInputArtifacts(w, req)
		want := 200
		if fail {
			want = 500
		}
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if !fail && w.Body.String() != "{\"artifacts\":[]}\n" {
			t.Fatal(w.Body.String())
		}
	}
}

func TestInputArtifactsHTTPMetadata(t *testing.T) {
	for _, body := range []jobdb.ChapterBody{jobdb.JobStartChapter{}, &jobdb.JobStartChapter{}} {
		s := &Server{chapters: chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
			return []jobdb.Chapter{{Ordinal: 17, Body: body, Artifacts: []jobdb.StoredArtifact{{Name: "nested/雪 ?#.txt", Size: 42}}}, {Ordinal: 18, Body: jobdb.RestartExtraChapter{}, Artifacts: []jobdb.StoredArtifact{{Name: "excluded", Size: 100}}}}, nil
		})}
		r := mux.NewRouter()
		r.HandleFunc("/projects/{projectId}/jobs/{jobId}/input-artifacts", s.handleInputArtifacts)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/projects/tenant/jobs/job/input-artifacts", nil))
		if w.Code != 200 || w.Body.String() != "{\"artifacts\":[{\"jobId\":\"job\",\"taskOrdinal\":17,\"name\":\"nested/雪 ?#.txt\",\"sizeBytes\":42}]}\n" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}

func TestInputArtifactsPreservesRuntimeErrors(t *testing.T) {
	for _, err := range []error{jobdb.ErrJobNotFound, errors.New("access denied"), errors.New("runtime unavailable")} {
		s := &Server{chapters: chapterReaderFunc(func(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) { return nil, err })}
		got := httptest.NewRecorder()
		s.handleInputArtifacts(got, mux.SetURLVars(httptest.NewRequest("GET", "/", nil), map[string]string{"projectId": "tenant", "jobId": "job"}))
		want := httptest.NewRecorder()
		writeError(want, err)
		if got.Code != want.Code || got.Body.String() != want.Body.String() {
			t.Fatalf("mapping changed: %d %s", got.Code, got.Body.String())
		}
	}
}
