package cortex

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func (s *Server) handleClearCrashConcern(w http.ResponseWriter, r *http.Request) {
	key := jobdb.JobKey{TenantId: mux.Vars(r)["projectId"], JobId: mux.Vars(r)["jobId"]}
	if strings.TrimSpace(key.TenantId) == "" || strings.TrimSpace(key.JobId) == "" || key.JobId == "self" || r.URL.RawQuery != "" {
		writeHTTPError(w, 400, errors.New("provide an exact tenant and job without query parameters"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1025))
	if err != nil || len(body) > 1024 {
		writeHTTPError(w, 400, errors.New("expected an empty JSON object or no body"))
		return
	}
	if len(bytes.TrimSpace(body)) != 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil || fields == nil || len(fields) != 0 {
			writeHTTPError(w, 400, errors.New("expected an empty JSON object"))
			return
		}
	}
	if r.Context().Err() != nil {
		writeError(w, r.Context().Err())
		return
	}
	// Use the same visible-only recipe boundary as other story mutations, and
	// verify identity before forwarding an operator action.
	page, err := recipejob.ListRecipeJobs(r.Context(), s.engine, recipejob.ListRecipeJobsRequest{
		TenantID: key.TenantId, JobIDs: []string{key.JobId}, Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}, PageSize: 1,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	visible := false
	for _, job := range page.Jobs {
		if job.TenantID == key.TenantId && job.JobID == key.JobId {
			visible = true
		}
	}
	if !visible {
		writeError(w, recipejob.ErrJobNotFound)
		return
	}

	if s.crashConcern == nil {
		writeJSON(w, 501, errorResponse{Error: "This runtime cannot clear crash concern", Code: "unsupported_crash_concern"})
		return
	}
	if r.Context().Err() != nil {
		writeError(w, r.Context().Err())
		return
	}
	err = s.crashConcern.ClearCrashConcern(r.Context(), jobdb.ClearCrashConcernRequest{JobKey: key, WorkerID: "cortex", Reason: "Crash concern cleared from Cortex"})
	if errors.Is(err, jobdb.ErrInvalidCrashConcernRequest) {
		writeHTTPError(w, 400, err)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
