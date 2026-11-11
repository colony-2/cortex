package cortex

import (
	"fmt"
	"net/http"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type inputArtifact struct {
	JobID       string `json:"jobId"`
	TaskOrdinal int64  `json:"taskOrdinal"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"sizeBytes"`
}

func inputArtifacts(job string, chapters []jobdb.Chapter) ([]inputArtifact, error) {
	result := make([]inputArtifact, 0)
	found := false
	for _, chapter := range chapters {
		switch body := chapter.Body.(type) {
		case jobdb.JobStartChapter:
		case *jobdb.JobStartChapter:
			if body == nil {
				return nil, fmt.Errorf("nil job-start chapter")
			}
		default:
			continue
		}
		if found {
			return nil, fmt.Errorf("multiple job-start chapters")
		}
		found = true
		for _, artifact := range chapter.Artifacts {
			result = append(result, inputArtifact{job, chapter.Ordinal, artifact.Name, artifact.Size})
		}
	}
	return result, nil
}

func (s *Server) handleInputArtifacts(w http.ResponseWriter, r *http.Request) {
	if s.chapters == nil {
		writeError(w, fmt.Errorf("chapter reading unavailable"))
		return
	}
	job := mux.Vars(r)["jobId"]
	chapters, err := s.chapters.ListChapters(r.Context(), jobdb.ListChaptersRequest{JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: job}})
	if err != nil {
		writeError(w, err)
		return
	}
	artifacts, err := documentInventory(job, chapters)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}
