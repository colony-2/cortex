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
	coretasks "github.com/colony-2/colony2/server/recipe-core/pkg/task"
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
		writeError(r, w, err, http.StatusBadRequest)
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
		writeError(r, w, err, http.StatusBadRequest)
		return
	}
	until, err := parseRFC3339(q.Get("until"))
	if err != nil {
		writeError(r, w, err, http.StatusBadRequest)
		return
	}

	limit := 50
	if raw := q.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(r, w, err, http.StatusBadRequest)
			return
		}
		limit = value
	}
	offset := 0
	if raw := q.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(r, w, err, http.StatusBadRequest)
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
		writeError(r, w, err, http.StatusInternalServerError)
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
		writeError(r, w, err, http.StatusBadRequest)
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
			writeError(r, w, err, http.StatusBadRequest)
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

func (h *Handlers) handleGetWorkflowOutcome(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	jobID := vars["jobId"]

	outcome, err := h.workflows.GetWorkflowOutcome(r.Context(), workflow.GetWorkflowOutcomeRequest{
		ProjectID: projectID,
		JobID:     jobID,
	})
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrInvalidProject):
			writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrNotFound):
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrOutcomePending):
			writeJSON(w, http.StatusTooEarly, openapi.ErrorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "Internal server error"})
		}
		return
	}

	writeJSON(w, http.StatusOK, toOpenAPIWorkflowOutcome(*outcome))
}

func (h *Handlers) handleGetJobRunStory(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	jobID := vars["jobId"]

	story, err := h.workflows.GetJobRunStory(r.Context(), workflow.GetJobRunStoryRequest{
		ProjectID: projectID,
		JobID:     jobID,
	})
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrInvalidProject):
			writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrNotFound):
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrJobRunStoryMismatch):
			writeJSON(w, http.StatusConflict, openapi.ErrorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "Internal server error"})
		}
		return
	}

	writeJSON(w, http.StatusOK, toOpenAPIJobRunStory(*story))
}

func (h *Handlers) handleRestartRecipeJob(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	jobID := vars["jobId"]

	type restartRecipeJobBody struct {
		StepOffset   *int64                  `json:"step_offset"`
		ContextPatch *coretasks.ContextPatch `json:"context_patch,omitempty"`
	}
	var body restartRecipeJobBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(r, w, err, http.StatusBadRequest)
		return
	}
	if body.StepOffset == nil || *body.StepOffset < 0 {
		writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: "step_offset is required and must be >= 0"})
		return
	}

	resp, err := h.workflows.RestartRecipeJob(r.Context(), workflow.RestartRecipeJobRequest{
		ProjectID:  projectID,
		JobID:      jobID,
		StepOffset: *body.StepOffset,
		Patch:      body.ContextPatch,
	})
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrInvalidProject):
			writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrNotFound):
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrEngineUnavailable):
			writeJSON(w, http.StatusBadGateway, openapi.ErrorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"job_id": resp.JobID})
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

func (h *Handlers) handleGetJobArtifact(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		http.Error(w, "workflow service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	jobID := vars["jobId"]
	taskOrdinalStr := vars["taskOrdinal"]
	artifactName := vars["artifactName"]

	ordinal, err := strconv.ParseInt(taskOrdinalStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: "invalid task ordinal"})
		return
	}

	data, err := h.workflows.GetArtifactByOrdinal(r.Context(), workflow.GetArtifactByOrdinalRequest{
		ProjectID:    projectID,
		JobID:        jobID,
		TaskOrdinal:  ordinal,
		ArtifactName: artifactName,
	})
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrNotFound):
			writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: err.Error()})
		case errors.Is(err, workflow.ErrInvalidProject):
			writeJSON(w, http.StatusBadRequest, openapi.ErrorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "failed to retrieve artifact"})
		}
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", data.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", data.SizeBytes))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data.Content); err != nil {
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

func toOpenAPIWorkflowOutcome(out workflow.WorkflowOutcome) openapi.WorkflowOutcome {
	artifacts := make([]openapi.ArtifactReference, 0, len(out.Artifacts))
	for _, art := range out.Artifacts {
		artifacts = append(artifacts, openapi.ArtifactReference{
			ArtifactId:   art.ArtifactID,
			ArtifactType: art.ArtifactType,
			CreatedAt:    art.CreatedAt,
			Name:         art.Name,
			SizeBytes:    art.SizeBytes,
			Url:          art.URL,
		})
	}

	var output *map[string]interface{}
	if out.Output != nil {
		output = &out.Output
	}

	return openapi.WorkflowOutcome{
		Artifacts:      artifacts,
		AttemptOrdinal: out.AttemptOrdinal,
		Error:          out.Error,
		JobId:          out.JobID,
		Output:         output,
		Status:         openapi.WorkflowStatus(out.Status),
	}
}

func toOpenAPIJobRunStory(st workflow.JobRunStory) openapi.JobRunStory {
	root := toOpenAPIJobRunStoryNode(st.Root)
	return openapi.JobRunStory{
		FinishedAt:         st.FinishedAt,
		InvocationSequence: st.InvocationSequence,
		JobId:              st.JobID,
		Recipe:             openapi.JobRunStoryRecipe{Id: st.Recipe.ID, Name: st.Recipe.Name, Version: st.Recipe.Version, Source: openapi.JobRunStoryRecipeSource{Kind: st.Recipe.Source.Kind, ArtifactName: st.Recipe.Source.ArtifactName}},
		Root:               root,
		StartedAt:          st.StartedAt,
		Status:             openapi.WorkflowStatus(st.Status),
	}
}

func toOpenAPIJobRunStoryNode(n *workflow.JobRunStoryNode) openapi.JobRunStoryNode {
	// Root is required by the schema; if we couldn't build it, return an empty "unknown" node.
	if n == nil {
		var nilAny *interface{}
		return openapi.JobRunStoryNode{
			ArtifactKeys:       []openapi.ArtifactKey{},
			Attempt:            1,
			Children:           []openapi.JobRunStoryNode{},
			Evaluations:        nil,
			FinishedAt:         nil,
			FromStateId:        nil,
			Id:                 "missing_root",
			Input:              nilAny,
			Invocation:         nil,
			InvokeSeq:          0,
			IsInitial:          nil,
			Kind:               openapi.JobRunStoryNodeKind("recipe"),
			OpId:               nil,
			OpType:             nil,
			Output:             nilAny,
			Path:               []string{"root"},
			PriorAttempts:      []openapi.JobRunStoryNode{},
			RecipeId:           nil,
			RestartFromOrdinal: nil,
			SequenceId:         nil,
			StartedAt:          nil,
			StateId:            nil,
			StateMachineId:     nil,
			Status:             openapi.JobRunStoryNodeStatus("unknown"),
			StepId:             nil,
			StepType:           nil,
			TaskOrdinal:        nil,
			Title:              "missing root",
		}
	}

	keys := make([]openapi.ArtifactKey, 0, len(n.ArtifactKeys))
	for _, k := range n.ArtifactKeys {
		keys = append(keys, openapi.ArtifactKey{
			JobId:       k.JobId,
			TaskOrdinal: k.TaskOrdinal,
			Name:        k.Name,
			SizeBytes:   k.SizeBytes,
		})
	}

	children := make([]openapi.JobRunStoryNode, 0, len(n.Children))
	for _, ch := range n.Children {
		children = append(children, toOpenAPIJobRunStoryNode(ch))
	}

	prior := make([]openapi.JobRunStoryNode, 0, len(n.PriorAttempts))
	for _, pa := range n.PriorAttempts {
		prior = append(prior, toOpenAPIJobRunStoryNode(pa))
	}

	var inPtr *interface{}
	if n.Input != nil {
		tmp := interface{}(n.Input)
		inPtr = &tmp
	}
	var outPtr *interface{}
	if n.Output != nil {
		tmp := interface{}(n.Output)
		outPtr = &tmp
	}

	var invPtr *map[string]interface{}
	if n.Invocation != nil {
		tmp := n.Invocation
		invPtr = &tmp
	}

	var errPtr *openapi.JobRunStoryError
	if n.Error != nil {
		var code *string
		if strings.TrimSpace(n.Error.Code) != "" {
			tmp := n.Error.Code
			code = &tmp
		}
		errPtr = &openapi.JobRunStoryError{Code: code, Message: n.Error.Message}
	}

	var evalsPtr *[]openapi.JobRunStoryTransitionEval
	if len(n.Evaluations) > 0 {
		evs := make([]openapi.JobRunStoryTransitionEval, 0, len(n.Evaluations))
		for _, ev := range n.Evaluations {
			evs = append(evs, openapi.JobRunStoryTransitionEval{
				Expression: ev.Expression,
				Reason:     ev.Reason,
				Result:     ev.Result,
				ToStateId:  ev.ToStateID,
			})
		}
		evalsPtr = &evs
	}

	var decisionPtr *openapi.JobRunStoryTransitionDecision
	if n.Decision != nil {
		decisionPtr = &openapi.JobRunStoryTransitionDecision{
			Kind:      openapi.JobRunStoryTransitionDecisionKind(n.Decision.Kind),
			ToStateId: n.Decision.ToStateID,
		}
	}

	var recipeID *string
	if strings.TrimSpace(n.RecipeID) != "" {
		tmp := n.RecipeID
		recipeID = &tmp
	}
	var sequenceID *string
	if strings.TrimSpace(n.SequenceID) != "" {
		tmp := n.SequenceID
		sequenceID = &tmp
	}
	var opID *string
	if strings.TrimSpace(n.OpID) != "" {
		tmp := n.OpID
		opID = &tmp
	}
	var opType *string
	if strings.TrimSpace(n.OpType) != "" {
		tmp := n.OpType
		opType = &tmp
	}
	var stepID *string
	if strings.TrimSpace(n.StepID) != "" {
		tmp := n.StepID
		stepID = &tmp
	}
	var stepType *string
	if strings.TrimSpace(n.StepType) != "" {
		tmp := n.StepType
		stepType = &tmp
	}
	var smID *string
	if strings.TrimSpace(n.StateMachineID) != "" {
		tmp := n.StateMachineID
		smID = &tmp
	}
	var stateID *string
	if strings.TrimSpace(n.StateID) != "" {
		tmp := n.StateID
		stateID = &tmp
	}
	var fromStateID *string
	if strings.TrimSpace(n.FromStateID) != "" {
		tmp := n.FromStateID
		fromStateID = &tmp
	}

	return openapi.JobRunStoryNode{
		ArtifactKeys:       keys,
		Attempt:            n.Attempt,
		Children:           children,
		Decision:           decisionPtr,
		Error:              errPtr,
		Evaluations:        evalsPtr,
		FinishedAt:         n.FinishedAt,
		FromStateId:        fromStateID,
		Id:                 n.ID,
		Input:              inPtr,
		Invocation:         invPtr,
		InvokeSeq:          n.InvokeSeq,
		IsInitial:          n.IsInitial,
		Kind:               openapi.JobRunStoryNodeKind(n.Kind),
		OpId:               opID,
		OpType:             opType,
		Output:             outPtr,
		Path:               n.Path,
		PriorAttempts:      prior,
		RecipeId:           recipeID,
		RestartFromOrdinal: n.RestartFromOrdinal,
		SequenceId:         sequenceID,
		StartedAt:          n.StartedAt,
		StateId:            stateID,
		StateMachineId:     smID,
		Status:             openapi.JobRunStoryNodeStatus(n.Status),
		StepId:             stepID,
		StepType:           stepType,
		TaskOrdinal:        n.TaskOrdinal,
		Title:              n.Title,
	}
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
