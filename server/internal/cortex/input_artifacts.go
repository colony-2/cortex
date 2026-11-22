package cortex

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type inputArtifact struct {
	JobID        string `json:"jobId"`
	TaskOrdinal  int64  `json:"taskOrdinal"`
	Name         string `json:"name"`
	SizeBytes    int64  `json:"sizeBytes"`
	contribution *int
	inputOrdinal int64
}

func inputArtifacts(job string, chapters []jobdb.Chapter) ([]inputArtifact, error) {
	result := make([]inputArtifact, 0)
	found := false
	appendIndex := 0
	addContribution := func(ordinal int64, keys []jobdb.ArtifactKey) {
		index := appendIndex
		appendIndex++
		for _, key := range keys {
			result = append(result, inputArtifact{JobID: key.JobId, TaskOrdinal: key.TaskOrdinal, Name: key.Name, SizeBytes: key.SizeBytes, contribution: &index, inputOrdinal: ordinal})
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
			addContribution(chapter.Ordinal, body.Artifacts)
			continue
		case *jobdb.AdditionalInputChapter:
			if body == nil {
				return nil, fmt.Errorf("nil additional-input chapter")
			}
			addContribution(chapter.Ordinal, body.Artifacts)
			continue
		default:
			continue
		}
		if found {
			return nil, fmt.Errorf("multiple job-start chapters")
		}
		found = true
		for _, artifact := range chapter.Artifacts {
			result = append(result, inputArtifact{JobID: job, TaskOrdinal: chapter.Ordinal, Name: artifact.Name, SizeBytes: artifact.Size, inputOrdinal: chapter.Ordinal})
		}
		if start.Append != nil {
			addContribution(chapter.Ordinal, start.AppendArtifacts)
		}
	}
	return result, nil
}

var errInputOrdinal = errors.New("ordinal must be a single non-negative integer")

func (s *Server) readInputInventory(r *http.Request) ([]inputDocument, error) {
	if s.chapters == nil {
		return nil, fmt.Errorf("chapter reading unavailable")
	}
	key := jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}
	if values, supplied := r.URL.Query()["ordinal"]; supplied {
		if len(values) != 1 {
			return nil, errInputOrdinal
		}
		ordinal, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || ordinal < 0 {
			return nil, errInputOrdinal
		}
		reader, ok := s.chapters.(chapterLookup)
		if !ok {
			return nil, fmt.Errorf("exact chapter reading unavailable")
		}
		chapter, err := reader.GetChapter(r.Context(), jobdb.ChapterRef{JobKey: key, Ordinal: ordinal})
		if err != nil {
			return nil, err
		}
		return documentInventory(key.JobId, []jobdb.Chapter{chapter})
	}
	// Group contributions form a contiguous prefix. A normal submission needs
	// only chapter zero, even if it has thousands of execution chapters.
	if reader, ok := s.chapters.(chapterLookup); ok {
		chapters := []jobdb.Chapter{}
		started := false
		for ord := int64(0); ; ord++ {
			ch, err := reader.GetChapter(r.Context(), jobdb.ChapterRef{JobKey: key, Ordinal: ord})
			if errors.Is(err, jobdb.ErrChapterNotFound) {
				break
			}
			if err != nil {
				return nil, err
			}
			switch body := ch.Body.(type) {
			case jobdb.JobStartChapter:
				chapters = append(chapters, ch)
				started = true
				if body.Append != nil {
					continue
				}
			case *jobdb.JobStartChapter:
				chapters = append(chapters, ch)
				started = true
				if body != nil && body.Append != nil {
					continue
				}
			case jobdb.AdditionalInputChapter, *jobdb.AdditionalInputChapter:
				chapters = append(chapters, ch)
				continue
			default:
				if !started {
					continue
				} // A retained job can have a pre-start chapter.
			}
			break
		}
		return documentInventory(key.JobId, chapters)
	}
	chapters, err := s.chapters.ListChapters(r.Context(), jobdb.ListChaptersRequest{JobKey: key})
	if err != nil {
		return nil, err
	}
	return documentInventory(key.JobId, chapters)
}

func (s *Server) handleInputArtifacts(w http.ResponseWriter, r *http.Request) {
	artifacts, err := s.readInputInventory(r)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}
