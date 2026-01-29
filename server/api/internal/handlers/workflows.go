package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/workflow/pkg/workflow"
	"github.com/gorilla/mux"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (h *Handlers) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	projectID := mux.Vars(r)["projectId"]
	q := r.URL.Query()

	statuses, err := parseWorkflowStatuses(q["status"])
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	var ticketID *string
	if raw := q.Get("ticket_id"); raw != "" {
		ticketID = &raw
	}
	var cellID *string
	if raw := q.Get("cell_id"); raw != "" {
		cellID = &raw
	}

	since, err := parseRFC3339(q.Get("since"))
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	until, err := parseRFC3339(q.Get("until"))
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	limit := 50
	if raw := q.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		limit = value
	}
	offset := 0
	if raw := q.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		offset = value
	}

	results, err := h.workflows.ListWorkflows(r.Context(), workflow.ListWorkflowsRequest{
		ProjectID: projectID,
		Statuses:  statuses,
		TicketID:  ticketID,
		CellID:    cellID,
		Since:     since,
		Until:     until,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}

	response := make([]openapi.WorkflowSummary, 0, len(results))
	for _, item := range results {
		response = append(response, toOpenAPIWorkflowSummary(item))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) handleStartWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	projectID := mux.Vars(r)["projectId"]

	var body openapi.StartWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.RecipeName) == "" || strings.TrimSpace(body.CellId) == "" {
		writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: "recipe_name and cell_id are required"})
		return
	}

	inputs := map[string]interface{}{}
	if body.Inputs != nil {
		inputs = *body.Inputs
	}

	var actorEmail *string
	if body.ActorEmail != nil {
		email := string(*body.ActorEmail)
		actorEmail = &email
	}

	summary, err := h.workflows.StartWorkflow(r.Context(), workflow.StartWorkflowRequest{
		ProjectID:      projectID,
		RecipeName:     body.RecipeName,
		CellID:         body.CellId,
		Inputs:         inputs,
		GitRef:         body.GitRef,
		TicketID:       body.TicketId,
		ActorEmail:     actorEmail,
		IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrInvalidProject),
			errors.Is(err, workflow.ErrInvalidCell),
			errors.Is(err, workflow.ErrRecipeNotFound):
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrEngineUnavailable):
			writeJSON(w, http.StatusBadGateway, openapi.ErrorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: err.Error()})
		}
		return
	}

	if summary == nil {
		writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "workflow start returned empty response"})
		return
	}

	writeJSON(w, http.StatusCreated, toOpenAPIWorkflowSummary(*summary))
}

func (h *Handlers) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	workflowID := vars["workflowId"]

	includeRaw := false
	if raw := r.URL.Query().Get("includeRawJobData"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		includeRaw = parsed
	}

	detail, err := h.workflows.GetWorkflow(r.Context(), workflow.GetWorkflowRequest{
		ProjectID:         projectID,
		WorkflowID:        workflowID,
		IncludeRawJobData: includeRaw,
	})
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrNotFound):
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: "Workflow not found"})
		case errors.Is(err, workflow.ErrWorkflowNotInProject):
			writeJSON(w, http.StatusForbidden, openapi.ErrorResponse{Message: "Workflow does not belong to this project"})
		default:
			writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "Internal server error"})
		}
		return
	}

	writeJSON(w, http.StatusOK, toOpenAPIWorkflowDetail(*detail))
}

func (h *Handlers) handleGetWorkflowArtifact(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}

	// Extract path parameters
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	workflowID := vars["workflowId"]
	chapterNumberStr := vars["chapterNumber"]
	artifactName := vars["artifactName"]

	// Parse chapter number
	chapterNumber, err := strconv.Atoi(chapterNumberStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: "invalid chapter number"})
		return
	}

	// Call workflow service
	artifactData, err := h.workflows.GetWorkflowArtifact(r.Context(), workflow.GetWorkflowArtifactRequest{
		ProjectID:     projectID,
		WorkflowID:    workflowID,
		ChapterNumber: chapterNumber,
		ArtifactName:  artifactName,
	})
	if err != nil {
		// Check for specific error types
		if errors.Is(err, workflow.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "failed to retrieve artifact"})
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifactData.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", artifactData.SizeBytes))

	// Write artifact content
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(artifactData.Content); err != nil {
		// Can't write error response after starting to write the body
		fmt.Fprintf(w, "error writing artifact content: %v", err)
	}
}

func parseRFC3339(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseWorkflowStatuses(values []string) ([]workflow.WorkflowStatus, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]workflow.WorkflowStatus, 0, len(values))
	for _, raw := range values {
		status := workflow.WorkflowStatus(raw)
		switch status {
		case workflow.WorkflowStatusRunning,
			workflow.WorkflowStatusCompleted,
			workflow.WorkflowStatusFailed,
			workflow.WorkflowStatusCanceled,
			workflow.WorkflowStatusTerminated,
			workflow.WorkflowStatusTimedOut,
			workflow.WorkflowStatusUnknown:
			result = append(result, status)
		default:
			return nil, fmt.Errorf("unknown workflow status %q", raw)
		}
	}
	return result, nil
}

func toOpenAPIWorkflowSummary(summary workflow.WorkflowSummary) openapi.WorkflowSummary {
	return openapi.WorkflowSummary{
		Actor:       toOpenAPIWorkflowActor(summary.Actor),
		CellId:      summary.CellID,
		CellName:    summary.CellName,
		CloseTime:   summary.CloseTime,
		CreatedAt:   summary.CreatedAt,
		InputHash:   summary.InputHash,
		RecipeName:  summary.RecipeName,
		RunId:       summary.RunID,
		StartTime:   summary.StartTime,
		SubmittedAt: summary.SubmittedAt,
		Status:      openapi.WorkflowStatus(summary.Status),
		TicketId:    summary.TicketID,
		TicketTitle: summary.TicketTitle,
		WorkflowId:  summary.WorkflowID,
	}
}

func toOpenAPIWorkflowDetail(detail workflow.WorkflowDetail) openapi.WorkflowDetail {
	out := openapi.WorkflowDetail{
		Actor:      toOpenAPIWorkflowActor(detail.Actor),
		CellId:     detail.CellID,
		CellName:   detail.CellName,
		CloseTime:  detail.CloseTime,
		CreatedAt:  detail.CreatedAt,
		GitCommit:  detail.GitCommit,
		GitRef:     detail.GitRef,
		RawJobData: detail.RawJobData,
		RecipeName: detail.RecipeName,
		RunId:      detail.RunID,
		StartTime:  detail.StartTime,
		Status:     openapi.WorkflowStatus(detail.Status),
		TicketId:   detail.TicketID,
		WorkflowId: detail.WorkflowID,
	}

	chapters := make([]openapi.ChapterDetail, 0, len(detail.Chapters))
	for _, chap := range detail.Chapters {
		chapters = append(chapters, toOpenAPIChapterDetail(chap))
	}
	out.Chapters = chapters

	if detail.Ticket != nil {
		ticket := toOpenAPITicket(detail.Ticket)
		out.Ticket = &ticket
	}

	return out
}

func toOpenAPIWorkflowActor(actor workflow.Actor) *openapi.Actor {
	switch actor.Type {
	case workflow.ActorTypeAgent:
		if actor.Agent == nil {
			return &openapi.Actor{Type: openapi.Agent}
		}
		return &openapi.Actor{
			Type: openapi.Agent,
			Agent: &openapi.ActorAgent{
				Cell:           actor.Agent.Cell,
				WorkflowName:   actor.Agent.WorkflowName,
				ExecutionId:    actor.Agent.ExecutionID,
				InvocationHash: actor.Agent.InvocationHash,
			},
		}
	default:
		if actor.User != nil && actor.User.Email != "" {
			return &openapi.Actor{Type: openapi.User, User: &openapi.ActorUser{Email: openapi_types.Email(actor.User.Email)}}
		}
		return &openapi.Actor{Type: openapi.User}
	}
}

func toOpenAPIChapterDetail(detail workflow.ChapterDetail) openapi.ChapterDetail {
	artifacts := make([]openapi.ArtifactReference, 0, len(detail.Artifacts))
	for _, art := range detail.Artifacts {
		artifacts = append(artifacts, openapi.ArtifactReference{
			ArtifactId:   art.ArtifactID,
			ArtifactType: art.ArtifactType,
			CreatedAt:    art.CreatedAt,
			Name:         art.Name,
			SizeBytes:    art.SizeBytes,
			Url:          art.URL,
		})
	}

	return openapi.ChapterDetail{
		Artifacts:     &artifacts,
		ChapterNumber: detail.ChapterNumber,
		ChapterType:   detail.ChapterType,
		EndTime:       detail.EndTime,
		Error:         detail.Error,
		Input:         &detail.Input,
		OpName:        detail.OpName,
		Output:        detail.Output,
		StartTime:     detail.StartTime,
		Status:        openapi.ChapterStatus(detail.Status),
	}
}
