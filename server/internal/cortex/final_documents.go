package cortex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func finalManifestID(manifest *story.FinalDocuments) string {
	raw, _ := json.Marshal(manifest)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func finalDocumentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, story.ErrOutcomePending), errors.Is(err, jobdb.ErrJobNotComplete):
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
	case errors.Is(err, story.ErrFinalDocumentsUnsupported):
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": err.Error(), "code": "final_documents_unsupported"})
	case errors.Is(err, story.ErrFinalDocumentUnavailable):
		writeJSON(w, http.StatusGone, map[string]string{"error": err.Error(), "code": "final_document_unavailable"})
	default:
		writeError(w, err)
	}
}

func (s *Server) handleFinalDocuments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	manifest, err := story.GetFinalDocuments(r.Context(), s.story, story.GetWorkflowOutcomeRequest{ProjectID: s.tenantID(r), JobID: mux.Vars(r)["jobId"]})
	if err != nil {
		finalDocumentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		*story.FinalDocuments
		ManifestID string `json:"manifest_id"`
	}{manifest, finalManifestID(manifest)})
}

func (s *Server) handleFinalDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	manifest, err := story.GetFinalDocuments(r.Context(), s.story, story.GetWorkflowOutcomeRequest{ProjectID: s.tenantID(r), JobID: mux.Vars(r)["jobId"]})
	if err != nil {
		finalDocumentError(w, err)
		return
	}
	// A manifest identity binds links to one selected terminal attempt and its
	// exact declarations. It grants no authority; reads recheck producer access.
	if r.URL.Query().Get("manifest_id") != finalManifestID(manifest) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Returned documents changed. Refresh the document list.", "code": "final_manifest_changed"})
		return
	}
	for _, document := range manifest.Documents {
		if document.Alias != r.URL.Query().Get("alias") {
			continue
		}
		if document.Resolution != "stored" || document.Producer == nil {
			finalDocumentError(w, story.ErrFinalDocumentUnavailable)
			return
		}
		key := document.Producer
		if s.engine == nil {
			finalDocumentError(w, story.ErrFinalDocumentsUnsupported)
			return
		}
		if _, err = s.engine.GetJobRun(r.Context(), jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: key.JobId}}); err != nil {
			writeError(w, err)
			return
		}
		// Read the precise member from the just-validated manifest. Reselecting the
		// alias after authorization could accidentally switch attempts during a retry.
		data, err := s.story.GetArtifactByOrdinal(r.Context(), story.GetArtifactByOrdinalRequest{ProjectID: s.tenantID(r), JobID: key.JobId, TaskOrdinal: key.TaskOrdinal, ArtifactName: key.Name})
		if err != nil {
			finalDocumentError(w, err)
			return
		}
		if len(data.Content) > 32<<20 {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("returned document exceeds 32 MiB download limit"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(document.Alias))
		w.Write(data.Content)
		return
	}
	writeHTTPError(w, http.StatusNotFound, fmt.Errorf("returned document is not declared in this manifest"))
}
