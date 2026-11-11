package cortex

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func historyError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "history_unavailable"
	if upstreamStatus, upstreamCode, ok := jobdb.CellErrorStatus(err); ok {
		writeJSON(w, upstreamStatus, map[string]string{"error": err.Error(), "code": upstreamCode})
		return
	}
	switch {
	case errors.Is(err, input.ErrHistorySnapshotExpired):
		status, code = http.StatusGone, "history_snapshot_expired"
	case errors.Is(err, input.ErrHistoryInvalidCursor):
		status, code = http.StatusBadRequest, "history_invalid_cursor"
	case errors.Is(err, input.ErrHistoryEntryNotFound), errors.Is(err, input.ErrHistoryArtifactNotFound), errors.Is(err, jobdb.ErrJobNotFound):
		status, code = http.StatusNotFound, "history_not_found"
	case errors.Is(err, input.ErrHistoryUnsupported):
		status, code = http.StatusNotImplemented, "history_unsupported"
	case errors.Is(err, input.ErrArtifactUnavailable):
		status, code = http.StatusGone, "history_artifact_unavailable"
	}
	writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
}

func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.history == nil {
		historyError(w, input.ErrHistoryUnsupported)
		return
	}
	size := 50
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		var err error
		size, err = strconv.Atoi(raw)
		if err != nil || size < 1 || size > 500 {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("page_size must be between 1 and 500"))
			return
		}
	}
	page, err := s.history.ListJobInputStory(r.Context(), input.ListJobInputStoryRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], PageSize: size, PageToken: r.URL.Query().Get("page_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	// Use the exact input page observation for publication and ordering data.
	// Older HistoryService implementations remain usable with explicit coverage.
	type conversationPage struct {
		*input.JobInputStoryPage
		Merges              []input.HistoryMergeEntry `json:"merges"`
		Events              []input.HistoryEvent      `json:"events"`
		PublicationCoverage string                    `json:"publication_coverage"`
	}
	result := conversationPage{JobInputStoryPage: page, Merges: []input.HistoryMergeEntry{}, Events: []input.HistoryEvent{}, PublicationCoverage: "recorded"}
	snapshot, err := input.GetJobHistorySnapshot(r.Context(), s.history, input.GetJobHistorySnapshotRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], SnapshotToken: page.SnapshotToken})
	if errors.Is(err, input.ErrHistoryUnsupported) {
		result.PublicationCoverage = "unavailable"
	} else if err != nil {
		historyError(w, err)
		return
	} else {
		result.Events = snapshot.Events
		for _, merge := range snapshot.Merges {
			if merge.EvidenceAvailability == "recorded" && (merge.PublicationDisposition == "published" || merge.PublicationDisposition == "already_present") {
				result.Merges = append(result.Merges, merge)
			} else if merge.EvidenceAvailability != "recorded" {
				result.PublicationCoverage = "partial"
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleConversationEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.history == nil {
		historyError(w, input.ErrHistoryUnsupported)
		return
	}
	entry, err := s.history.GetJobInputStoryEntry(r.Context(), input.GetJobInputStoryEntryRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], EntryID: r.URL.Query().Get("entry_id"), SnapshotToken: r.URL.Query().Get("snapshot_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) handleConversationArtifact(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.history == nil {
		historyError(w, input.ErrHistoryUnsupported)
		return
	}
	// The history resolver validates membership and resolves original producer
	// coordinates through the authorized engine; no arbitrary reference is accepted.
	// Recheck original producer access in the request context even for a cached
	// snapshot. Membership tokens identify bytes; they never grant read authority.
	detail, err := s.history.GetJobInputStoryEntry(r.Context(), input.GetJobInputStoryEntryRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], EntryID: r.URL.Query().Get("entry_id"), SnapshotToken: r.URL.Query().Get("snapshot_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	found := false
	for _, manifest := range [][]input.HistoryArtifactMember{detail.RequestManifest, detail.ResponseManifest} {
		for _, member := range manifest {
			if member.ArtifactID != r.URL.Query().Get("artifact_id") {
				continue
			}
			found = true
			if member.Ref.Kind == artifacts.RefKindExternal {
				historyError(w, input.ErrArtifactUnavailable)
				return
			}
			if key, stored := member.Ref.StoredKey(); stored {
				if s.engine == nil {
					historyError(w, input.ErrHistoryUnsupported)
					return
				}
				_, err = s.engine.GetJobRun(r.Context(), jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: key.JobId}})
				if err != nil {
					historyError(w, err)
					return
				}
			}
		}
	}
	if !found {
		historyError(w, input.ErrHistoryArtifactNotFound)
		return
	}
	artifact, err := s.history.OpenJobInputStoryArtifact(r.Context(), input.OpenJobInputStoryArtifactRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], EntryID: r.URL.Query().Get("entry_id"), ArtifactID: r.URL.Query().Get("artifact_id"), SnapshotToken: r.URL.Query().Get("snapshot_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	defer artifact.Close()
	// Buffer before sending headers so missing/corrupt retained bytes can still
	// produce an explicit error instead of a successful truncated document.
	data, err := io.ReadAll(io.LimitReader(artifact, 32<<20+1))
	if err != nil {
		historyError(w, err)
		return
	}
	if len(data) > 32<<20 {
		writeHTTPError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("historical attachment exceeds 32 MiB download limit"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(artifact.Name))
	w.Write(data)
}
