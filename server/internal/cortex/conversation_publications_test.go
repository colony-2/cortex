package cortex

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type snapshotHistoryStub struct {
	historyStub
	request input.GetJobHistorySnapshotRequest
}

func (h *snapshotHistoryStub) GetJobHistorySnapshot(_ context.Context, r input.GetJobHistorySnapshotRequest) (*input.JobHistorySnapshot, error) {
	h.request = r
	records := []input.HistoryMergeEntry{}
	for _, disposition := range []string{"published", "already_present", "no_changes", ""} {
		records = append(records, input.HistoryMergeEntry{EntryID: disposition, TenantID: r.TenantID, JobID: r.JobID, EvidenceAvailability: "recorded", PublicationDisposition: disposition})
	}
	records = append(records, input.HistoryMergeEntry{EntryID: "old", TenantID: r.TenantID, JobID: r.JobID, EvidenceAvailability: "unavailable"}, input.HistoryMergeEntry{EntryID: "child", TenantID: r.TenantID, JobID: "child", EvidenceAvailability: "recorded", PublicationDisposition: "published"})
	return &input.JobHistorySnapshot{SnapshotToken: r.SnapshotToken, Merges: records, Events: []input.HistoryEvent{{Ordinal: 1, Kind: "merge_completion", EntryID: "published"}}}, nil
}
func TestConversationPublicationAdapter(t *testing.T) {
	h := &snapshotHistoryStub{}
	w := httptest.NewRecorder()
	historyRouter(h).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "opaque+/=", h.request.SnapshotToken)
	var page struct {
		Merges   []input.HistoryMergeEntry `json:"merges"`
		Events   []input.HistoryEvent      `json:"events"`
		Coverage string                    `json:"publication_coverage"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Merges, 2)
	require.Len(t, page.Events, 1)
	require.Equal(t, "partial", page.Coverage)
}

type finalStoryStub struct {
	storyService
	manifest *story.FinalDocuments
	read     story.GetArtifactByOrdinalRequest
	err      error
}

func (s *finalStoryStub) GetFinalDocuments(context.Context, story.GetWorkflowOutcomeRequest) (*story.FinalDocuments, error) {
	return s.manifest, s.err
}
func (s *finalStoryStub) GetFinalDocument(context.Context, story.GetFinalDocumentRequest) (*story.ArtifactData, error) {
	panic("must validate original producer explicitly")
}
func (s *finalStoryStub) GetArtifactByOrdinal(_ context.Context, r story.GetArtifactByOrdinalRequest) (*story.ArtifactData, error) {
	s.read = r
	return &story.ArtifactData{Filename: "original.md", Content: []byte("retained bytes")}, nil
}
func TestFinalDocumentMembershipAndAuthorization(t *testing.T) {
	ordinal := int64(7)
	manifest := &story.FinalDocuments{JobID: "job", AttemptOrdinal: &ordinal, State: story.FinalManifestPresent, Documents: []story.FinalDocument{{Alias: "report.md", Resolution: "stored", Producer: &jobdb.ArtifactKey{JobId: "grandchild", TaskOrdinal: 4, Name: "original.md"}}, {Alias: "external", Resolution: "external"}}}
	svc := &finalStoryStub{manifest: manifest}
	engine := &producerAccessEngine{}
	server := &Server{story: svc, engine: engine}
	router := mux.NewRouter()
	router.HandleFunc("/{projectId}/{jobId}/manifest", server.handleFinalDocuments)
	router.HandleFunc("/{projectId}/{jobId}/file", server.handleFinalDocument)
	id := finalManifestID(manifest)
	read := func(alias, identity string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/file?"+url.Values{"alias": {alias}, "manifest_id": {identity}}.Encode(), nil))
		return w
	}
	require.Equal(t, 404, read("forged", id).Code)
	require.Equal(t, 409, read("report.md", "old").Code)
	require.NotEqual(t, 200, read("external", id).Code)
	require.Empty(t, svc.read.JobID)
	w := read("report.md", id)
	require.Equal(t, 200, w.Code)
	require.Equal(t, "retained bytes", w.Body.String())
	require.Equal(t, "grandchild", svc.read.JobID)
	require.Equal(t, "original.md", svc.read.ArtifactName)
	require.Equal(t, "tenant", engine.key.TenantId)
	engine.err = jobdb.ErrCellForbidden
	w = read("report.md", id)
	require.Equal(t, 403, w.Code)
	require.False(t, strings.Contains(w.Body.String(), "retained bytes"))
	svc.err = story.ErrOutcomePending
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job/manifest", nil))
	require.Equal(t, 202, w.Code)
}

// Controlled retained records pass through the actual c2j history accessor;
// fabricated merge DTOs cannot establish optional-subject evidence behavior.
type publicationRecordReader struct {
	input.HistoryReader
	run jobdb.GetJobRunResponse
}

func (r publicationRecordReader) GetJobRun(context.Context, jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	return r.run, nil
}

func TestConversationOptionalSubjectsThroughProjection(t *testing.T) {
	for _, subject := range []string{"absent", "null", "empty", "actual"} {
		for _, disposition := range []string{"published", "already_present"} {
			t.Run(disposition+"/"+subject, func(t *testing.T) {
				output := map[string]any{"candidate_sha": "abc", "publication_branch": "refs/heads/main", "publication_disposition": disposition}
				switch subject {
				case "null":
					output["candidate_subject"] = nil
				case "empty":
					output["candidate_subject"] = ""
				case "actual":
					output["candidate_subject"] = "  Actual message\t"
				}
				raw, err := json.Marshal(map[string]any{"v": 1, "kind": "activity_output", "payload": map[string]any{"output": output}})
				require.NoError(t, err)
				id := `/scope:"$job"#1/attempt:1/task:"merge:merge"#1`
				reader := publicationRecordReader{run: jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "job"}}, Execution: []jobdb.ExecutionHistoryEntry{
					{Ordinal: 1, Record: jobdb.ExecutionRecord{Version: jobdb.ExecutionPatternVersion, Kind: "task", ID: id, Descriptor: json.RawMessage(`{"call":{"task_type":"merge:merge"}}`)}},
					{Ordinal: 2, Record: jobdb.ExecutionRecord{Version: jobdb.ExecutionPatternVersion, Kind: "result", ID: id + "/attempt:1", Output: &jobdb.ExecutionSnapshot{Data: raw}}},
				}}}
				legacyID := `/scope:"$job"#1/attempt:1/task:"merge:merge"#2`
				reader.run.Execution = append(reader.run.Execution,
					jobdb.ExecutionHistoryEntry{Ordinal: 3, Record: jobdb.ExecutionRecord{Version: jobdb.ExecutionPatternVersion, Kind: "task", ID: legacyID, Descriptor: json.RawMessage(`{"call":{"task_type":"merge:merge"}}`)}},
					jobdb.ExecutionHistoryEntry{Ordinal: 4, Record: jobdb.ExecutionRecord{Version: jobdb.ExecutionPatternVersion, Kind: "result", ID: legacyID + "/attempt:1", Output: &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"v":1,"kind":"activity_output","payload":{"output":{"success":true,"merged_hash":"legacy"}}}`)}}},
				)
				history, err := input.NewHistoryService(input.HistoryConfig{Reader: reader})
				require.NoError(t, err)
				snapshot, err := input.GetJobHistorySnapshot(context.Background(), history, input.GetJobHistorySnapshotRequest{TenantID: "tenant", JobID: "job"})
				require.NoError(t, err)
				require.Len(t, snapshot.Merges, 2)
				require.Equal(t, "unavailable", snapshot.Merges[1].EvidenceAvailability)
				require.NotEmpty(t, snapshot.Merges[1].Output)
				require.Equal(t, "recorded", snapshot.Merges[0].EvidenceAvailability)
				w := httptest.NewRecorder()
				historyRouter(history).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job", nil))
				require.Equal(t, 200, w.Code)
				var page struct {
					Merges   []input.HistoryMergeEntry
					Coverage string `json:"publication_coverage"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
				require.Len(t, page.Merges, 1)
				require.Equal(t, "partial", page.Coverage)
				if subject == "absent" || subject == "null" {
					require.Nil(t, page.Merges[0].CandidateSubject)
				} else {
					require.NotNil(t, page.Merges[0].CandidateSubject)
					require.Equal(t, output["candidate_subject"], *page.Merges[0].CandidateSubject)
				}
			})
		}
	}
}
