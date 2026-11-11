package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

type historyStub struct {
	snapshot        *input.JobHistorySnapshot
	snapshotRequest input.GetJobHistorySnapshotRequest
	list            input.ListJobInputStoryRequest
	detail          input.GetJobInputStoryEntryRequest
	member          input.OpenJobInputStoryArtifactRequest
	err             error
	reader          io.ReadCloser
	ref             *artifacts.Ref
}

func (h *historyStub) GetJobHistorySnapshot(_ context.Context, req input.GetJobHistorySnapshotRequest) (*input.JobHistorySnapshot, error) {
	h.snapshotRequest = req
	if h.snapshot != nil {
		return h.snapshot, nil
	}
	return &input.JobHistorySnapshot{Events: []input.HistoryEvent{}, Merges: []input.HistoryMergeEntry{}}, h.err
}
func (h *historyStub) ListJobInputStory(_ context.Context, r input.ListJobInputStoryRequest) (*input.JobInputStoryPage, error) {
	h.list = r
	return &input.JobInputStoryPage{TenantID: r.TenantID, JobID: r.JobID, SnapshotToken: "opaque+/=", Completeness: "partial", Entries: []input.JobInputStoryEntry{}}, h.err
}
func (h *historyStub) GetJobInputStoryEntry(_ context.Context, r input.GetJobInputStoryEntryRequest) (*input.JobInputStoryEntryDetail, error) {
	h.detail = r
	return &input.JobInputStoryEntryDetail{JobInputStoryEntry: input.JobInputStoryEntry{EntryID: r.EntryID}, RequestManifest: []input.HistoryArtifactMember{{ArtifactID: "member", Ref: func() artifacts.Ref {
		if h.ref != nil {
			return *h.ref
		}
		return artifacts.Ref{}
	}()}}}, h.err
}
func (h *historyStub) OpenJobInputStoryArtifact(_ context.Context, r input.OpenJobInputStoryArtifactRequest) (*input.HistoryArtifact, error) {
	h.member = r
	return &input.HistoryArtifact{Name: "original.md", ReadCloser: h.reader}, h.err
}
func historyRouter(h input.HistoryService) http.Handler {
	s := &Server{history: h}
	r := mux.NewRouter()
	r.HandleFunc("/{projectId}/{jobId}", s.handleConversation)
	r.HandleFunc("/{projectId}/{jobId}/entry", s.handleConversationEntry)
	r.HandleFunc("/{projectId}/{jobId}/artifact", s.handleConversationArtifact)
	return r
}
func TestConversationTypedAdapter(t *testing.T) {
	h := &historyStub{reader: io.NopCloser(strings.NewReader("original producer bytes"))}
	r := historyRouter(h)
	t.Run("CT-013 CT-032 bounded pages and opaque cursors", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job?page_size=2&page_token=%2B%2F%3D", nil))
		if w.Code != 200 || h.list.TenantID != "tenant" || h.list.PageSize != 2 || h.list.PageToken != "+/=" || !strings.Contains(w.Body.String(), `"completeness":"partial"`) {
			t.Fatalf("%d %s %+v", w.Code, w.Body, h.list)
		}
		for _, q := range []string{"0", "501", "bad"} {
			w = httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job?page_size="+q, nil))
			if w.Code != 400 {
				t.Fatal(w.Code)
			}
		}
	})
	t.Run("CT-014 exact detail without a prior page", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/entry?entry_id=old%2F%3F", nil))
		if w.Code != 200 || h.detail.EntryID != "old/?" || h.detail.SnapshotToken != "" {
			t.Fatalf("%d %+v", w.Code, h.detail)
		}
	})
	t.Run("CT-017 historical member uses occurrence not current form", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/artifact?entry_id=old&artifact_id=member&snapshot_token=%2B%2F%3D", nil))
		if w.Code != 200 || w.Body.String() != "original producer bytes" || h.member.EntryID != "old" || h.member.ArtifactID != "member" || h.member.SnapshotToken != "+/=" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%d %s %+v", w.Code, w.Body, h.member)
		}
	})
	t.Run("CT-019 CT-020 CT-033 typed unavailable errors never become empty success", func(t *testing.T) {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{input.ErrHistorySnapshotExpired, 410, "history_snapshot_expired"},
			{input.ErrHistoryInvalidCursor, 400, "history_invalid_cursor"},
			{input.ErrHistoryEntryNotFound, 404, "history_not_found"},
			{input.ErrHistoryArtifactNotFound, 404, "history_not_found"},
			{input.ErrHistoryUnsupported, 501, "history_unsupported"},
			{input.ErrArtifactUnavailable, 410, "history_artifact_unavailable"},
			{errors.New("access denied"), 500, "history_unavailable"},
		} {
			h.err = tc.err
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job", nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		}
		h.err = nil
	})
}

type corruptReader struct{}

func (corruptReader) Read([]byte) (int, error) { return 0, errors.New("corrupt bytes") }
func (corruptReader) Close() error             { return nil }
func TestConversationCorruptMember(t *testing.T) {
	h := &historyStub{reader: corruptReader{}}
	w := httptest.NewRecorder()
	historyRouter(h).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/artifact?entry_id=old&artifact_id=member", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "corrupt bytes") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

type producerAccessEngine struct {
	workflow.Engine
	key jobdb.JobKey
	err error
}

func (e *producerAccessEngine) GetJobRun(_ context.Context, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	e.key = req.JobKey
	return jobdb.GetJobRunResponse{}, e.err
}
func TestConversationOriginalProducerAuthorization(t *testing.T) {
	ref := artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "original-child", TaskOrdinal: 3, Name: "protected.md"})
	h := &historyStub{ref: &ref, reader: io.NopCloser(strings.NewReader("protected original"))}
	engine := &producerAccessEngine{err: jobdb.ErrCellForbidden}
	s := &Server{history: h, engine: engine}
	r := mux.NewRouter()
	r.HandleFunc("/{projectId}/{jobId}/artifact", s.handleConversationArtifact)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/artifact?entry_id=old&artifact_id=member&snapshot_token=cached", nil))
	if w.Code != 403 || strings.Contains(w.Body.String(), "protected original") || h.member.ArtifactID != "" || engine.key.TenantId != "tenant" || engine.key.JobId != "original-child" {
		t.Fatalf("%d %s %+v", w.Code, w.Body, engine.key)
	}
}

// Generate a bounded stream without allocating another document-sized fixture.
type repeatedBytes struct{}

func (repeatedBytes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestConversationOversizedMember(t *testing.T) {
	h := &historyStub{reader: io.NopCloser(io.LimitReader(repeatedBytes{}, (32<<20)+1))}
	w := httptest.NewRecorder()
	historyRouter(h).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/artifact?entry_id=old&artifact_id=member", nil))
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "32 MiB download limit") || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestConversationExternalAndForgedMembers(t *testing.T) {
	ref := artifacts.NewExternalRef("external.md", "https://example.invalid/protected", false)
	for _, member := range []string{"member", "forged"} {
		t.Run(member, func(t *testing.T) {
			h := &historyStub{ref: &ref, reader: io.NopCloser(strings.NewReader("must not open"))}
			w := httptest.NewRecorder()
			historyRouter(h).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/artifact?entry_id=old&artifact_id="+member, nil))
			want := http.StatusGone
			if member == "forged" {
				want = http.StatusNotFound
			}
			if w.Code != want || h.member.ArtifactID != "" || strings.Contains(w.Body.String(), "must not open") {
				t.Fatalf("%d %s %+v", w.Code, w.Body, h.member)
			}
		})
	}
}

func TestConversationPublicationSelection(t *testing.T) {
	h := &historyStub{snapshot: &input.JobHistorySnapshot{Merges: []input.HistoryMergeEntry{
		{EntryID: "published", EvidenceAvailability: "recorded", PublicationDisposition: "published", CandidateSubject: nil},
		{EntryID: "present", EvidenceAvailability: "recorded", PublicationDisposition: "already_present"},
		{EntryID: "nochange", EvidenceAvailability: "recorded", PublicationDisposition: "no_changes"},
		{EntryID: "incomplete", EvidenceAvailability: "unavailable", PublicationDisposition: "published"},
		{EntryID: "unknown", EvidenceAvailability: "recorded", PublicationDisposition: "future"},
	}, Events: []input.HistoryEvent{{Ordinal: 2, Kind: "merge_completion", EntryID: "published"}}}}
	w := httptest.NewRecorder()
	historyRouter(h).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result struct {
		Merges   []input.HistoryMergeEntry `json:"merges"`
		Events   []input.HistoryEvent      `json:"events"`
		Coverage string                    `json:"publication_coverage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Merges) != 2 || result.Merges[0].EntryID != "published" || result.Merges[0].CandidateSubject != nil || result.Coverage != "partial" || len(result.Events) != 1 {
		t.Fatalf("unexpected projection %+v", result)
	}
	if h.snapshotRequest.SnapshotToken != "opaque+/=" || h.snapshotRequest.TenantID != "tenant" || h.snapshotRequest.JobID != "job" {
		t.Fatalf("wrong observation %+v", h.snapshotRequest)
	}
}
