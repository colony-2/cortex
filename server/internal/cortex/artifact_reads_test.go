package cortex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestStoredDocumentMakesOneJobDBRequest(t *testing.T) {
	const name = "review/雪 ?#%+.md"
	var requests []string
	status := http.StatusOK
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-reader", r.Header.Get("Authorization"))
		requests = append(requests, r.Method+" "+r.URL.EscapedPath())
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte("original bytes"))
		}
	}))
	defer backend.Close()
	s := &Server{cfg: Config{JobDBURL: backend.URL}, documentClient: backend.Client()}
	router := mux.NewRouter()
	router.HandleFunc("/projects/{projectId}/jobs/{jobId}/stored-document", s.handleStoredDocument)
	for _, code := range []int{200, 403, 404} {
		status = code
		requests = nil
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/projects/c2/jobs/producer/stored-document?ordinal=83&name="+url.QueryEscape(name), nil)
		req.Header.Set("Authorization", "Bearer test-reader")
		router.ServeHTTP(w, req)
		require.Equal(t, code, w.Code, w.Body.String())
		require.Equal(t, []string{"GET /v1/tenants/c2/jobs/producer/chapters/83/artifacts/" + url.PathEscape(name)}, requests)
		if code == 200 {
			require.Equal(t, "original bytes", w.Body.String())
		}
	}
}

type recordingChapters struct {
	chapterReader // A chapter-list call fails this test.
	reads         []int64
	chapters      map[int64]jobdb.Chapter
}

func (r *recordingChapters) GetChapter(ctx context.Context, ref jobdb.ChapterRef) (jobdb.Chapter, error) {
	r.reads = append(r.reads, ref.Ordinal)
	if ch, ok := r.chapters[ref.Ordinal]; ok {
		return ch, nil
	}
	return jobdb.Chapter{}, jobdb.ErrChapterNotFound
}

func TestInputArtifactOrdinalReads(t *testing.T) {
	reader := &recordingChapters{chapters: map[int64]jobdb.Chapter{0: {Body: jobdb.JobStartChapter{}}, 7: {Ordinal: 7, Body: jobdb.AdditionalInputChapter{Artifacts: []jobdb.ArtifactKey{{JobId: "p", TaskOrdinal: 3, Name: "only.txt"}}}}}}
	s := &Server{chapters: reader}
	for _, tc := range []struct {
		query  string
		status int
		reads  []int64
	}{
		{"", 200, []int64{0}}, {"?ordinal=7", 200, []int64{7}}, {"?ordinal=8", 404, []int64{8}},
		{"?ordinal=-1", 400, nil}, {"?ordinal=0&ordinal=1", 400, nil}, {"?ordinal=x", 400, nil},
	} {
		reader.reads = nil
		req := mux.SetURLVars(httptest.NewRequest("GET", "/input-artifacts"+tc.query, nil), map[string]string{"projectId": "t", "jobId": "j"})
		w := httptest.NewRecorder()
		s.handleInputArtifacts(w, req)
		require.Equal(t, tc.status, w.Code)
		require.Equal(t, tc.reads, reader.reads)
		require.Equal(t, tc.query == "?ordinal=7", strings.Contains(w.Body.String(), "only.txt"))
	}
}

func TestInlineInputDocumentReadsOnlyItsChapter(t *testing.T) {
	data := jobdb.NewTaskDataOrPanic(map[string]any{"inputs": map[string]any{"documents": map[string]any{
		"note": map[string]any{"kind": "value", "format": "text", "name": "note.md", "value": "inline bytes"},
	}}})
	reader := &recordingChapters{chapters: map[int64]jobdb.Chapter{
		6: {Ordinal: 6, Body: jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: data.GetDataOrPanic()}}},
	}}
	s := &Server{chapters: reader}
	r := mux.SetURLVars(httptest.NewRequest("GET", "/input-document?binding=note&ordinal=6", nil), map[string]string{"projectId": "t", "jobId": "j"})
	w := httptest.NewRecorder()
	s.handleInputDocument(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "inline bytes", w.Body.String())
	require.Equal(t, []int64{6}, reader.reads)
}

func TestReviewIndexSurvivesReopening(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	key := jobdb.JobKey{TenantId: "t", JobId: "j"}
	generation, accepted := int64(17), int64(24)
	field := func(v string) json.RawMessage { return json.RawMessage(v) }
	form := map[string]json.RawMessage{"presentation": field(`{"type":"review/v2"}`), "request": field(`{"review_id":"review","question":"Ready?"}`), "request_id": field(`"native"`)}
	question, err := json.Marshal(form)
	require.NoError(t, err)
	answer := field(`{"receipt":{"request_id":"native","submission_id":"accepted"},"response":{"feedback":"Done"}}`)
	entry := input.JobInputStoryEntry{RequestID: "native", EntryID: "event", FormFields: form, Locator: input.HistoryLocator{GenerationOrdinal: &generation, AcceptedOrdinal: &accepted}, AcceptedResponse: input.HistoryValue{Present: true, Raw: answer}}
	cfg := normalizeConfig(Config{WorkingDir: t.TempDir()})
	s := &Server{cfg: cfg, logger: cfg.Logger}
	_, err = s.indexReviewPages(context.Background(), key, 31, &input.JobInputStoryPage{Entries: []input.JobInputStoryEntry{entry}, Completeness: "complete"}, "review")
	require.NoError(t, err)
	reader := &recordingChapters{chapters: map[int64]jobdb.Chapter{}}
	for ord, raw := range map[int64]json.RawMessage{generation: question, accepted: answer} {
		output, err := json.Marshal(map[string]any{"v": 2, "kind": "activity_output", "payload": map[string]any{"output": raw}})
		require.NoError(t, err)
		record, err := json.Marshal(jobdb.ExecutionRecord{Version: jobdb.ExecutionPatternVersion, Kind: "result", ID: "task", Output: &jobdb.ExecutionSnapshot{Data: output}})
		require.NoError(t, err)
		reader.chapters[ord] = jobdb.Chapter{Ordinal: ord, Body: jobdb.ExecutionChapter{Record: record}}
	}
	reopened := &Server{cfg: cfg, chapters: reader}
	page, err := reopened.indexedReview(context.Background(), key, "review")
	require.NoError(t, err)
	require.NotNil(t, page)
	require.Equal(t, "complete", page.Completeness)
	entries := page.Entries
	require.Len(t, entries, 1)
	got, err := json.Marshal(entries[0].FormFields)
	require.NoError(t, err)
	require.JSONEq(t, string(question), string(got))
	require.JSONEq(t, string(answer), string(entries[0].AcceptedResponse.Raw))
	require.Equal(t, []int64{31, generation, accepted}, reader.reads)
	reader.chapters[31] = jobdb.Chapter{Ordinal: 31}
	page, err = reopened.indexedReview(context.Background(), key, "review")
	require.NoError(t, err)
	require.Nil(t, page)
}
