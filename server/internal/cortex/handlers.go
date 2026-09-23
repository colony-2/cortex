package cortex

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/story"
	coretask "github.com/colony-2/c2j/pkg/task"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type errorResponse struct {
	Error string `json:"error"`
}

type healthResponse struct {
	OK      bool   `json:"ok"`
	Started string `json:"started"`
}

type submitJobRequest struct {
	Cell   string                 `json:"cell,omitempty"`
	Repo   string                 `json:"repo,omitempty"`
	Recipe string                 `json:"recipe,omitempty"`
	Inputs map[string]interface{} `json:"inputs,omitempty"`
	JobID  string                 `json:"job_id,omitempty"`
}

type submitJobResponse struct {
	TenantID string                   `json:"tenant_id"`
	JobID    string                   `json:"job_id"`
	Target   recipejob.ResolvedTarget `json:"target"`
	Recipe   string                   `json:"recipe"`
}

type restartJobRequest struct {
	StepOffset int64                  `json:"step_offset"`
	Patch      *coretask.ContextPatch `json:"patch,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{OK: true, Started: s.started.Format("2006-01-02T15:04:05Z07:00")})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	tenantID := s.tenantID(r)
	writeJSON(w, http.StatusOK, []Project{{
		ID:       tenantID,
		TenantID: tenantID,
		Name:     "Tenant " + tenantID,
	}})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	tenantID := s.tenantID(r)
	writeJSON(w, http.StatusOK, Project{
		ID:       tenantID,
		TenantID: tenantID,
		Name:     "Tenant " + tenantID,
	})
}

func (s *Server) handleListCells(w http.ResponseWriter, r *http.Request) {
	cells, err := s.cells.Cells(r.Context(), s.tenantID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cells)
}

func (s *Server) handleSelfCell(w http.ResponseWriter, r *http.Request) {
	cell, err := s.cells.SelfCell(r.Context(), s.tenantID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cell)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	tenantID := s.tenantID(r)
	statuses, allStatuses, err := parseStatuses(r.URL.Query()["status"])
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if len(statuses) == 0 && !allStatuses {
		statuses = recipejob.DefaultVisibleStatuses()
	}

	repo, err := s.jobListRepositoryFilter(r, tenantID)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	pageSize := parsePositiveInt(r.URL.Query().Get("pageSize"), 100)
	if pageSize > jobdb.MaxListJobsPageSize {
		pageSize = jobdb.MaxListJobsPageSize
	}

	stores := recipejob.StoresForStatuses(statuses)
	if allStatuses {
		stores = []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}
	}
	resp, err := recipejob.ListRecipeJobs(r.Context(), s.engine, recipejob.ListRecipeJobsRequest{
		TenantID:         tenantID,
		RepositorySource: repo,
		Statuses:         statuses,
		Stores:           stores,
		PageSize:         pageSize,
		PageToken:        r.URL.Query().Get("pageToken"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSubmitJob(w http.ResponseWriter, r *http.Request) {
	var req submitJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	tenantID := s.tenantID(r)
	cell := strings.TrimSpace(req.Cell)
	if cell == "" {
		cell = strings.TrimSpace(req.Repo)
	}
	target, err := recipejob.ResolveTarget(r.Context(), recipejob.ResolveTargetRequest{
		WorkingDir: s.cfg.WorkingDir,
		Cell:       cell,
		Self:       cell == "",
		TenantID:   tenantID,
	})
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	key, err := recipejob.SubmitRecipeJob(r.Context(), recipejob.BuildStartJobRequest{
		TenantID: tenantID,
		JobID:    req.JobID,
		Target:   target,
		Recipe:   req.Recipe,
		Inputs:   req.Inputs,
	}, s.engine)
	if err != nil {
		writeError(w, err)
		return
	}
	job, err := recipejob.GetRecipeJob(r.Context(), s.engine, recipejob.GetRecipeJobRequest{
		TenantID: tenantID,
		JobID:    key.JobId,
	})
	if err == nil {
		writeJSON(w, http.StatusCreated, job)
		return
	}
	writeJSON(w, http.StatusCreated, submitJobResponse{
		TenantID: tenantID,
		JobID:    key.JobId,
		Target:   target,
		Recipe:   req.Recipe,
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, err := recipejob.GetRecipeJob(r.Context(), s.engine, recipejob.GetRecipeJobRequest{
		TenantID: s.tenantID(r),
		JobID:    mux.Vars(r)["jobId"],
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// Verify tenant-scoped existence before replay/control calls. Remote replay can
// return an empty story for a missing job instead of a typed not-found error.
func (s *Server) requireJob(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := recipejob.GetRecipeJob(r.Context(), s.engine, recipejob.GetRecipeJobRequest{
			TenantID: s.tenantID(r),
			JobID:    mux.Vars(r)["jobId"],
		})
		if err != nil {
			writeError(w, err)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleRestartJob(w http.ResponseWriter, r *http.Request) {
	var req restartJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if req.StepOffset < 1 {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("step_offset must be >= 1 to preserve the job start chapter"))
		return
	}
	resp, err := s.story.RestartRecipeJob(r.Context(), story.RestartRecipeJobRequest{
		ProjectID:  s.tenantID(r),
		JobID:      mux.Vars(r)["jobId"],
		StepOffset: req.StepOffset,
		Patch:      req.Patch,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleJobStory(w http.ResponseWriter, r *http.Request) {
	resp, err := s.story.GetJobRunStory(r.Context(), story.GetJobRunStoryRequest{
		ProjectID: s.tenantID(r),
		JobID:     mux.Vars(r)["jobId"],
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleJobOutcome(w http.ResponseWriter, r *http.Request) {
	resp, err := s.story.GetWorkflowOutcome(r.Context(), story.GetWorkflowOutcomeRequest{
		ProjectID: s.tenantID(r),
		JobID:     mux.Vars(r)["jobId"],
	})
	if err != nil {
		if errors.Is(err, story.ErrOutcomePending) || errors.Is(err, jobdb.ErrJobNotComplete) {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleArtifactByOrdinal(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ordinal, err := strconv.ParseInt(vars["taskOrdinal"], 10, 64)
	if err != nil || ordinal < 0 {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("taskOrdinal must be a non-negative integer"))
		return
	}
	resp, err := s.story.GetArtifactByOrdinal(r.Context(), story.GetArtifactByOrdinalRequest{
		ProjectID:    s.tenantID(r),
		JobID:        vars["jobId"],
		TaskOrdinal:  ordinal,
		ArtifactName: vars["artifactName"],
	})
	if err != nil {
		writeError(w, err)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(resp.Filename))
	if contentType == "" {
		contentType = http.DetectContentType(resp.Content)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Content)
}

func (s *Server) tenantID(r *http.Request) string {
	if value := strings.TrimSpace(mux.Vars(r)["projectId"]); value != "" {
		return value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("tenantId")); value != "" {
		return value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("tenant_id")); value != "" {
		return value
	}
	return s.cfg.DefaultTenantID
}

func (s *Server) jobListRepositoryFilter(r *http.Request, tenantID string) (string, error) {
	query := r.URL.Query()
	cell := strings.TrimSpace(query.Get("cell"))
	if cell == "" {
		cell = strings.TrimSpace(query.Get("repo"))
	}
	if cell == "" {
		return "", nil
	}
	target, err := recipejob.ResolveTarget(r.Context(), recipejob.ResolveTargetRequest{
		WorkingDir: s.cfg.WorkingDir,
		Cell:       cell,
		TenantID:   tenantID,
	})
	if err != nil {
		return "", err
	}
	return target.RepositorySource, nil
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseStatuses(values []string) ([]jobdb.JobStatus, bool, error) {
	statuses := make([]jobdb.JobStatus, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.EqualFold(part, "all") {
				return nil, true, nil
			}
			status, ok := knownStatus(part)
			if !ok {
				return nil, false, fmt.Errorf("unknown status %q", part)
			}
			statuses = append(statuses, status)
		}
	}
	return statuses, false, nil
}

func knownStatus(value string) (jobdb.JobStatus, bool) {
	value = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	statuses := map[string]jobdb.JobStatus{
		string(jobdb.JobStatusReady):          jobdb.JobStatusReady,
		string(jobdb.JobStatusExpired):        jobdb.JobStatusExpired,
		string(jobdb.JobStatusPendingJobs):    jobdb.JobStatusPendingJobs,
		string(jobdb.JobStatusAwaitingFuture): jobdb.JobStatusAwaitingFuture,
		string(jobdb.JobStatusActive):         jobdb.JobStatusActive,
		string(jobdb.JobStatusCrashConcern):   jobdb.JobStatusCrashConcern,
		string(jobdb.JobStatusCancelled):      jobdb.JobStatusCancelled,
		string(jobdb.JobStatusCompleted):      jobdb.JobStatusCompleted,
	}
	status, ok := statuses[value]
	return status, ok
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeHTTPError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, recipejob.ErrJobNotFound), errors.Is(err, jobdb.ErrJobNotFound), story.IsNotFound(err):
		writeHTTPError(w, http.StatusNotFound, err)
	case errors.Is(err, jobdb.ErrConflict), jobdb.IsConflict(err):
		writeHTTPError(w, http.StatusConflict, err)
	default:
		writeHTTPError(w, http.StatusInternalServerError, err)
	}
}
