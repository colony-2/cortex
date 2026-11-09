package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/colony-2/c2j/pkg/cellref"
	"github.com/colony-2/c2j/pkg/jobref"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/story"
	coretask "github.com/colony-2/c2j/pkg/task"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

// Project the native submission handle without another read for goal identity.
type nativeSubmitter struct{ jobworkflow.Engine }

func (s nativeSubmitter) SubmitJobRef(ctx context.Context, job jobdb.SubmitJob) (jobref.Ref, error) {
	handle, err := s.SubmitJob(ctx, job)
	if err != nil {
		return jobref.Ref{}, err
	}
	return jobref.FromHandle(handle)
}

type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
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

type restartJobRequest struct {
	RequestID string `json:"request_id"`
	Goal      struct {
		Mode string `json:"mode"`
	} `json:"goal"`
	ScopeID        string                 `json:"scope_id,omitempty"`
	ChapterOrdinal *int64                 `json:"chapter_ordinal,omitempty"`
	Patch          *coretask.ContextPatch `json:"patch,omitempty"`
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

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	tenantID := s.tenantID(r)
	includeHidden := false
	if values, ok := r.URL.Query()["includeHidden"]; ok {
		if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("includeHidden must be true or false"))
			return
		}
		includeHidden = values[0] == "true"
	}
	statuses, allStatuses, err := parseStatuses(r.URL.Query()["status"])
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if len(statuses) == 0 && !allStatuses {
		statuses = recipejob.DefaultVisibleStatuses()
	}
	displayStatuses, err := parseDisplayStatuses(r.URL.Query()["displayStatus"])
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if len(displayStatuses) > 0 {
		if len(r.URL.Query()["status"]) > 0 {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("status and displayStatus cannot be combined"))
			return
		}
		// Outcomes and cancellation requests can override the scheduling status.
		statuses, allStatuses = nil, true
	}

	cells, err := jobListCellFilter(r)
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
	resp, err := listJobsByDisplayStatus(r.Context(), s.engine, recipejob.ListRecipeJobsRequest{
		IncludeHidden: includeHidden,
		GoalIDs:       r.URL.Query()["goal"],
		TenantID:      tenantID,
		CellNames:     cells,
		Statuses:      statuses,
		Stores:        stores,
		PageSize:      pageSize,
		PageToken:     r.URL.Query().Get("pageToken"),
	}, displayStatuses)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSubmitJob(w http.ResponseWriter, r *http.Request) {
	var req submitJobRequest
	artifacts, status, err := decodeGoalSubmission(w, r, &req)
	if err != nil {
		writeHTTPError(w, status, err)
		return
	}
	tenantID := s.tenantID(r)
	if err := jobdb.ValidateCellName(req.Cell); err != nil {
		writeError(w, err)
		return
	}
	target, err := recipejob.ResolveRegisteredTarget(r.Context(), recipejob.ResolvedTarget{
		CellName: req.Cell, CellRepo: req.Repo, TenantID: tenantID, DefaultRef: cellref.DefaultRef,
	}, cellref.Registry(s.cells, tenantID))
	if err != nil {
		writeError(w, err)
		return
	}
	key, err := recipejob.SubmitRecipeJob(r.Context(), recipejob.BuildStartJobRequest{
		TenantID:  tenantID,
		JobID:     req.JobID,
		Target:    target,
		Recipe:    req.Recipe,
		Inputs:    req.Inputs,
		Artifacts: artifacts,
	}, nativeSubmitter{s.engine})
	if err != nil {
		writeError(w, err)
		return
	}
	if job, err := recipejob.GetRecipeJob(r.Context(), s.engine, recipejob.GetRecipeJobRequest{TenantID: tenantID, JobID: key.JobID}); err == nil {
		writeJSON(w, http.StatusCreated, job)
		return
	}
	// Submission identity is complete even if a subsequent inspection is unavailable.
	writeJSON(w, http.StatusCreated, map[string]string{
		"goal_id": key.GoalID, "tenant_id": key.TenantID, "job_id": key.JobID,
		"cell_id": key.CellID, "cell_name": key.Cell, "repo": key.CellRepo,
	})
}

// Explicit detail/history reads must include jobs hidden from the inbox. The
// pinned recipejob helper otherwise uses the default visible-only list query.
type archivedJobReader struct{ jobworkflow.Engine }

func (e archivedJobReader) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	req.IncludeHidden = true
	return e.Engine.ListJobs(ctx, req)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, err := recipejob.GetRecipeJob(r.Context(), archivedJobReader{s.engine}, recipejob.GetRecipeJobRequest{
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
	return s.requireJobVisibility(next, false)
}

func (s *Server) requireArchivedJob(next http.HandlerFunc) http.HandlerFunc {
	return s.requireJobVisibility(next, true)
}

func (s *Server) requireJobVisibility(next http.HandlerFunc, includeHidden bool) http.HandlerFunc {
	reader := s.engine
	if includeHidden {
		reader = archivedJobReader{s.engine}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := recipejob.ListRecipeJobs(r.Context(), reader, recipejob.ListRecipeJobsRequest{
			TenantID: s.tenantID(r),
			JobIDs:   []string{mux.Vars(r)["jobId"]},
			Stores:   []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived},
			PageSize: 1,
		})
		if err == nil && len(page.Jobs) == 0 {
			err = recipejob.ErrJobNotFound
		}
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
	if (strings.TrimSpace(req.ScopeID) == "") == (req.ChapterOrdinal == nil) {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("provide exactly one of chapter_ordinal or scope_id"))
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || (req.Goal.Mode != "" && req.Goal.Mode != "continue" && req.Goal.Mode != "new") {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("restart requires request_id and goal mode continue or new"))
		return
	}
	resp, err := s.story.RestartRecipeJob(r.Context(), story.RestartRecipeJobRequest{
		RequestID:      req.RequestID,
		Goal:           req.Goal,
		ProjectID:      s.tenantID(r),
		JobID:          mux.Vars(r)["jobId"],
		ScopeID:        req.ScopeID,
		ChapterOrdinal: req.ChapterOrdinal,
		Patch:          req.Patch,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleRestartContext(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope_id")
	chapter := r.URL.Query().Get("chapter_ordinal")
	if (strings.TrimSpace(scope) == "") == (chapter == "") {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("provide exactly one of chapter_ordinal or scope_id"))
		return
	}
	var ordinal int64
	if chapter != "" {
		var err error
		ordinal, err = strconv.ParseInt(chapter, 10, 64)
		if err != nil || ordinal < 0 {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("chapter_ordinal must be a non-negative integer"))
			return
		}
	}
	resp, err := s.story.GetRestartContext(r.Context(), story.GetRestartContextRequest{
		ProjectID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], ChapterOrdinal: ordinal, ScopeID: scope,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

const defaultCancellationReason = "Cancelled from Cortex"

// Validate the entire bounded body before performing any mutation. A supplied
// reason must be a string; null is intentionally distinct from an absent field.
func cancellationReason(w http.ResponseWriter, r *http.Request) (string, bool, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, err)
		} else {
			writeHTTPError(w, http.StatusBadRequest, err)
		}
		return "", false, false
	}
	var fields map[string]json.RawMessage
	if len(strings.TrimSpace(string(body))) != 0 {
		if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("expected a JSON object"))
			return "", false, false
		}
	}
	raw, supplied := fields["reason"]
	reason := ""
	if supplied {
		if string(raw) == "null" || json.Unmarshal(raw, &reason) != nil {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("reason must be a string"))
			return "", false, false
		}
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 4000 {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("reason must be at most 4,000 Unicode code points"))
		return "", false, false
	}
	if reason == "" {
		reason = defaultCancellationReason
	}
	return reason, supplied, true
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	reason, _, valid := cancellationReason(w, r)
	if !valid {
		return
	}
	err := s.engine.CancelJob(r.Context(), jobdb.CancelJob{
		JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]},
		Reason: reason,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	reason, supplied, valid := cancellationReason(w, r)
	if !valid {
		return
	}
	if s.jobDeletion == nil {
		writeHTTPError(w, http.StatusNotImplemented, jobdb.ErrJobDeletionUnsupported)
		return
	}
	key := jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}
	cancellationAccepted := false
	if supplied {
		run, err := s.engine.GetJobRun(r.Context(), jobdb.GetJobRunRequest{JobKey: key})
		if err != nil {
			writeError(w, err)
			return
		}
		// Archived runtime rows include completed cancellations. Never replace their
		// accepted detail, including on a retry from a different browser session.
		cancellationAccepted = run.Job.ArchivedAt != nil && run.Job.CompletionStatus == "cancelled"
		if run.Job.ArchivedAt == nil {
			listed, err := s.engine.ListJobs(r.Context(), jobdb.ListJobsRequest{TenantIds: []string{key.TenantId}, JobKeys: []jobdb.JobKey{key}, IncludeHidden: true})
			if err != nil {
				writeError(w, err)
				return
			}
			if len(listed.Jobs) != 1 {
				writeError(w, jobdb.ErrJobNotFound)
				return
			}
			cancellationAccepted = cancellationAccepted || listed.Jobs[0].CancelRequested
		}
		if run.Job.ArchivedAt == nil && run.Job.Status != jobdb.JobStatusCompleted && run.Job.Status != jobdb.JobStatusCancelled && !cancellationAccepted {
			if err := s.engine.CancelJob(r.Context(), jobdb.CancelJob{JobKey: key, Reason: reason}); err != nil {
				writeError(w, err)
				return
			}
			// A completion can win the race with CancelJob. Read the persisted
			// state before deciding whether an archive failure is a partial cancel.
			listed, err := s.engine.ListJobs(r.Context(), jobdb.ListJobsRequest{TenantIds: []string{key.TenantId}, JobKeys: []jobdb.JobKey{key}, IncludeHidden: true})
			if err != nil || len(listed.Jobs) != 1 {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Cancellation request accepted; unable to inspect job before archiving", Code: "archive_failed_after_cancel"})
				return
			}
			row := listed.Jobs[0]
			cancellationAccepted = (row.ArchivedAt == nil && row.CancelRequested) || (row.ArchivedAt != nil && row.CompletionStatus == "cancelled")
		}
	}
	_, err := s.jobDeletion.DeleteJobs(r.Context(), jobdb.DeleteJobsRequest{TenantID: key.TenantId, JobIDs: []string{key.JobId}})
	if err != nil {
		if cancellationAccepted {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Cancellation succeeded; archiving failed: " + err.Error(), Code: "archive_failed_after_cancel"})
		} else {
			writeError(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func jobListCellFilter(r *http.Request) ([]string, error) {
	q := r.URL.Query()
	if q.Has("cell") || q.Has("repo") {
		return nil, fmt.Errorf("use cellNames to filter jobs by cell name")
	}
	for _, name := range q["cellNames"] {
		if err := jobdb.ValidateCellName(name); err != nil {
			return nil, err
		}
	}
	return q["cellNames"], nil
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
	if status, code, ok := jobdb.CellErrorStatus(err); ok {
		writeJSON(w, status, errorResponse{Error: err.Error(), Code: code})
		return
	}
	switch {
	case errors.Is(err, jobdb.ErrLeasePermissionDenied):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Permission denied", Code: "lease_permission_denied"})
	case errors.Is(err, story.ErrInvalidRestartBoundary), errors.Is(err, story.ErrInvalidProject):
		writeHTTPError(w, http.StatusBadRequest, err)
	case errors.Is(err, story.ErrRestartContextUnavailable):
		writeHTTPError(w, http.StatusUnprocessableEntity, err)
	case errors.Is(err, jobdb.ErrJobDeletionUnsupported):
		writeHTTPError(w, http.StatusNotImplemented, err)
	case errors.Is(err, recipejob.ErrJobNotFound), errors.Is(err, jobdb.ErrJobNotFound), errors.Is(err, jobdb.ErrGoalNotFound), story.IsNotFound(err):
		writeHTTPError(w, http.StatusNotFound, err)
	case errors.Is(err, jobdb.ErrConflict), errors.Is(err, jobdb.ErrGoalConflict), jobdb.IsConflict(err):
		writeHTTPError(w, http.StatusConflict, err)
	default:
		writeHTTPError(w, http.StatusInternalServerError, err)
	}
}
