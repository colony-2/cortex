package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/gorilla/mux"
)

type swfJobRunGetter interface {
	GetJobRun(ctx context.Context, req swf.GetJobRunRequest) (swf.GetJobRunResponse, error)
}

func (h *Handlers) handleTestGetJobRun(w http.ResponseWriter, r *http.Request) {
	if h.swfEngine == nil {
		http.Error(w, "swf engine unavailable", http.StatusNotImplemented)
		return
	}

	vars := mux.Vars(r)
	projectID := vars["projectId"]
	jobID := vars["jobId"]

	q := r.URL.Query()
	includeInputs, err := parseOptionalBoolQuery(q, "includeInputs", "include_inputs")
	if err != nil {
		writeError(r, w, err, http.StatusBadRequest)
		return
	}
	includeOutputs, err := parseOptionalBoolQuery(q, "includeOutputs", "include_outputs")
	if err != nil {
		writeError(r, w, err, http.StatusBadRequest)
		return
	}
	includeArtifacts, err := parseOptionalBoolQuery(q, "includeArtifacts", "include_artifacts")
	if err != nil {
		writeError(r, w, err, http.StatusBadRequest)
		return
	}
	includeAttemptInputs, err := parseOptionalBoolQuery(q, "includeAttemptInputs", "include_attempt_inputs")
	if err != nil {
		writeError(r, w, err, http.StatusBadRequest)
		return
	}

	resp, err := h.swfEngine.GetJobRun(r.Context(), swf.GetJobRunRequest{
		JobKey: swf.JobKey{
			TenantId: projectID,
			JobId:    jobID,
		},
		IncludeInputs:        includeInputs,
		IncludeOutputs:       includeOutputs,
		IncludeArtifacts:     includeArtifacts,
		IncludeAttemptInputs: includeAttemptInputs,
	})
	if err != nil {
		switch {
		case errors.Is(err, swf.ErrJobNotFound):
			writeError(r, w, err, http.StatusNotFound)
		default:
			writeError(r, w, err, http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseOptionalBoolQuery(q url.Values, keys ...string) (bool, error) {
	for _, key := range keys {
		if raw := q.Get(key); raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return false, err
			}
			return parsed, nil
		}
	}
	return false, nil
}
