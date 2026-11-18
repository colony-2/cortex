package cortex

import (
	"fmt"
	"net/http"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type inputArtifact struct {
	JobID        string `json:"jobId"`
	TaskOrdinal  int64  `json:"taskOrdinal"`
	Name         string `json:"name"`
	SizeBytes    int64  `json:"sizeBytes"`
	contribution *int
}

func inputArtifacts(job string, chapters []jobdb.Chapter) ([]inputArtifact, error) {
	result := make([]inputArtifact, 0)
	found := false
	appendIndex := 0
	addContribution := func(keys []jobdb.ArtifactKey) {
		index := appendIndex
		appendIndex++
		for _, key := range keys {
			result = append(result, inputArtifact{JobID: key.JobId, TaskOrdinal: key.TaskOrdinal, Name: key.Name, SizeBytes: key.SizeBytes, contribution: &index})
		}
	}
	for _, chapter := range chapters {
		var start *jobdb.JobStartChapter
		switch body := chapter.Body.(type) {
		case jobdb.JobStartChapter:
			start = &body
		case *jobdb.JobStartChapter:
			if body == nil {
				return nil, fmt.Errorf("nil job-start chapter")
			}
			start = body
		case jobdb.AdditionalInputChapter:
			addContribution(body.Artifacts)
			continue
		case *jobdb.AdditionalInputChapter:
			if body == nil {
				return nil, fmt.Errorf("nil additional-input chapter")
			}
			addContribution(body.Artifacts)
			continue
		default:
			continue
		}
		if found {
			return nil, fmt.Errorf("multiple job-start chapters")
		}
		found = true
		for _, artifact := range chapter.Artifacts {
			result = append(result, inputArtifact{JobID: job, TaskOrdinal: chapter.Ordinal, Name: artifact.Name, SizeBytes: artifact.Size})
		}
		if start.Append != nil {
			addContribution(start.AppendArtifacts)
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
