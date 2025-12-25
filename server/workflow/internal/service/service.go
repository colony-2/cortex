package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/strata-go/pkg/client"
	"github.com/colony-2/strata-go/pkg/client/pagination"
	"github.com/colony-2/strata-go/pkg/client/story"
	"github.com/colony-2/swf-go/pkg/swf"
)

var (
	ErrNotFound             = errors.New("workflow: not found")
	ErrWorkflowNotInProject = errors.New("workflow: not in project")
)

type Config struct {
	Engine   swf.SWFEngine
	Strata   *client.Client
	Tickets  ticket.Service
	Cells    cell.Service
	Projects project.Service
}

type Service struct {
	engine   swf.SWFEngine
	strata   *client.Client
	tickets  ticket.Service
	cells    cell.Service
	projects project.Service
}

func New(cfg Config) (*Service, error) {
	if cfg.Engine == nil {
		return nil, errors.New("workflow service: engine is required")
	}
	return &Service{
		engine:   cfg.Engine,
		strata:   cfg.Strata,
		tickets:  cfg.Tickets,
		cells:    cfg.Cells,
		projects: cfg.Projects,
	}, nil
}

type chapterEnvelope struct {
	Meta        chapterMeta     `json:"meta"`
	PayloadKind string          `json:"payload_kind"`
	Payload     json.RawMessage `json:"payload"`
}

type chapterMeta struct {
	Ordinal   int64               `json:"ordinal"`
	TaskType  string              `json:"task_type"`
	CreatedAt time.Time           `json:"created_at"`
	InputRef  *swf.InputReference `json:"input_ref,omitempty"`
	Attempt   int                 `json:"attempt,omitempty"`
}

func (s *Service) ListWorkflows(ctx context.Context, req model.ListWorkflowsRequest) ([]model.WorkflowSummary, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	target := limit + offset
	pageSize := target
	if pageSize <= 0 {
		pageSize = 1
	}
	if pageSize > swf.MaxListJobsPageSize {
		pageSize = swf.MaxListJobsPageSize
	}

	summaries := make([]model.WorkflowSummary, 0, target)
	pageToken := ""
	for {
		resp, err := s.engine.ListJobs(ctx, swf.ListJobsRequest{
			TenantIds:     []string{req.ProjectID},
			Stores:        []swf.JobStore{swf.JobStoreActive, swf.JobStoreArchived},
			CreatedAfter:  req.Since,
			CreatedBefore: req.Until,
			PageSize:      pageSize,
			PageToken:     pageToken,
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Jobs) == 0 {
			break
		}
		fmt.Println(resp.Jobs)
		for _, job := range resp.Jobs {
			summary, ok, err := s.buildSummary(ctx, req.ProjectID, job)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			if !statusMatches(req.Statuses, summary.Status) {
				continue
			}
			if req.TicketID != nil && (summary.TicketID == nil || *summary.TicketID != *req.TicketID) {
				continue
			}
			if req.CellID != nil && (summary.CellID == nil || *summary.CellID != *req.CellID) {
				continue
			}
			summaries = append(summaries, summary)
			if len(summaries) >= target {
				break
			}
		}
		if len(summaries) >= target || resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if offset >= len(summaries) {
		return []model.WorkflowSummary{}, nil
	}
	end := offset + limit
	if end > len(summaries) {
		end = len(summaries)
	}
	return summaries[offset:end], nil
}

func (s *Service) GetWorkflow(ctx context.Context, req model.GetWorkflowRequest) (*model.WorkflowDetail, error) {
	if req.WorkflowID == "" {
		return nil, ErrNotFound
	}
	jobKey := swf.JobKey{TenantId: req.ProjectID, JobId: req.WorkflowID}

	resp, err := s.engine.ListJobs(ctx, swf.ListJobsRequest{
		TenantIds: []string{req.ProjectID},
		JobKeys:   []swf.JobKey{jobKey},
		Stores:    []swf.JobStore{swf.JobStoreActive, swf.JobStoreArchived},
		PageSize:  1,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Jobs) == 0 {
		return nil, ErrNotFound
	}

	job := resp.Jobs[0]
	if job.JobKey.TenantId != req.ProjectID {
		return nil, ErrWorkflowNotInProject
	}

	startJob, _ := s.loadStartJob(ctx, job.JobKey)
	recipeName := ""
	if startJob != nil {
		recipeName = startJob.RecipeName
	}

	status := mapWorkflowStatus(job.Status)
	createdAt := job.CreatedAt
	startTime := &createdAt
	closeTime := job.ArchivedAt

	detail := model.WorkflowDetail{
		WorkflowID: job.JobKey.JobId,
		RunID:      job.JobKey.JobId,
		Status:     status,
		RecipeName: recipeName,
		StartTime:  startTime,
		CloseTime:  closeTime,
		Actor:      actorFromStartJob(startJob),
		CreatedAt:  createdAt,
	}

	if startJob != nil {
		if startJob.JobContext.Actor.TicketID != "" {
			ticketID := startJob.JobContext.Actor.TicketID
			detail.TicketID = &ticketID
			if s.tickets != nil {
				if ticketDetail, err := s.loadTicket(ctx, ticketID); err == nil {
					detail.Ticket = ticketDetail
				}
			}
		}
		if startJob.JobContext.Workflow.CellName != "" {
			cellName := startJob.JobContext.Workflow.CellName
			detail.CellName = &cellName
			if s.cells != nil {
				if cellID, err := s.findCellID(ctx, req.ProjectID, cellName); err == nil {
					detail.CellID = cellID
				}
			}
		}
		if startJob.GitRef != "" {
			gitRef := startJob.GitRef
			detail.GitRef = &gitRef
		}
	}

	if req.IncludeRawJobData {
		if raw := parseRawJobPayload(job.Payload); raw != nil {
			detail.RawJobData = raw
		}
	}

	chapters, err := s.loadChapters(ctx, job.JobKey)
	if err != nil {
		return nil, err
	}
	detail.Chapters = chapters

	return &detail, nil
}

func (s *Service) buildSummary(ctx context.Context, projectID string, job swf.JobSummary) (model.WorkflowSummary, bool, error) {
	startJob, _ := s.loadStartJob(ctx, job.JobKey)
	if startJob == nil {
		return model.WorkflowSummary{}, false, nil
	}

	status := mapWorkflowStatus(job.Status)
	createdAt := job.CreatedAt
	startTime := &createdAt
	closeTime := job.ArchivedAt

	summary := model.WorkflowSummary{
		WorkflowID: job.JobKey.JobId,
		RunID:      job.JobKey.JobId,
		Status:     status,
		RecipeName: startJob.RecipeName,
		StartTime:  startTime,
		CloseTime:  closeTime,
		Actor:      actorFromStartJob(startJob),
		CreatedAt:  createdAt,
	}

	if startJob.JobContext.Actor.TicketID != "" {
		ticketID := startJob.JobContext.Actor.TicketID
		summary.TicketID = &ticketID
		if s.tickets != nil {
			if ticketDetail, err := s.loadTicket(ctx, ticketID); err == nil && ticketDetail != nil {
				title := ticketDetail.Title
				summary.TicketTitle = &title
			}
		}
	}

	if startJob.JobContext.Workflow.CellName != "" {
		cellName := startJob.JobContext.Workflow.CellName
		summary.CellName = &cellName
		if s.cells != nil {
			if cellID, err := s.findCellID(ctx, projectID, cellName); err == nil {
				summary.CellID = cellID
			}
		}
	}

	return summary, true, nil
}

func (s *Service) loadStartJob(ctx context.Context, jobKey swf.JobKey) (*workflowctl.StartJob, error) {
	if s.strata == nil {
		return nil, nil
	}
	chap, err := s.strata.Chapter(ctx, jobKey.ToStoryKey(), 0)
	if err != nil {
		return nil, err
	}
	var env chapterEnvelope
	if err := json.Unmarshal(chap.Body(), &env); err != nil {
		return nil, err
	}
	if len(env.Payload) == 0 {
		return nil, nil
	}
	var start workflowctl.StartJob
	if err := json.Unmarshal(env.Payload, &start); err != nil {
		return nil, err
	}
	return &start, nil
}

func (s *Service) loadChapters(ctx context.Context, jobKey swf.JobKey) ([]model.ChapterDetail, error) {
	if s.strata == nil {
		return []model.ChapterDetail{}, nil
	}
	storyHandle, err := s.strata.Story(ctx, jobKey.ToStoryKey())
	if err != nil {
		return nil, err
	}
	iter, err := storyHandle.Chapters(ctx, story.ChaptersOptions{PageSize: 100, Direction: story.DirectionForward})
	if err != nil {
		return nil, err
	}

	chapters := []model.ChapterDetail{}
	for iter.HasNext() {
		chap, err := iter.Next(ctx)
		if errors.Is(err, pagination.ErrNoMoreItems) {
			break
		}
		if err != nil {
			return nil, err
		}
		detail, err := chapterToDetail(chap)
		if err != nil {
			return nil, err
		}
		chapters = append(chapters, detail)
	}
	return chapters, nil
}

func chapterToDetail(chap story.Chapter) (model.ChapterDetail, error) {
	var env chapterEnvelope
	if err := json.Unmarshal(chap.Body(), &env); err != nil {
		return model.ChapterDetail{}, err
	}
	status := mapChapterStatus(env.PayloadKind)
	startTime := env.Meta.CreatedAt

	input := map[string]interface{}{}
	var output *map[string]interface{}
	var errMsg *string

	payloadMap := map[string]interface{}{}
	if len(env.Payload) > 0 && json.Unmarshal(env.Payload, &payloadMap) == nil {
		if status == model.ChapterStatusFailed {
			if msg := extractErrorMessage(payloadMap); msg != "" {
				errMsg = &msg
			}
		} else if chap.Ordinal() == 0 {
			input = payloadMap
		} else {
			output = &payloadMap
		}
	}

	artifacts := make([]model.ArtifactReference, 0)
	for _, art := range chap.Artifacts() {
		size := art.SizeBytes()
		artID := art.ID()
		artType := art.ContentType()
		name := art.Name()
		createdAt := env.Meta.CreatedAt
		artifacts = append(artifacts, model.ArtifactReference{
			ArtifactID:   artID,
			ArtifactType: artType,
			Name:         name,
			SizeBytes:    &size,
			CreatedAt:    createdAt,
		})
	}

	chapterType := env.Meta.TaskType
	if chapterType == "" {
		chapterType = "workflow"
	}

	return model.ChapterDetail{
		ChapterNumber: int(chap.Ordinal()),
		ChapterType:   chapterType,
		Status:        status,
		StartTime:     &startTime,
		Input:         input,
		Output:        output,
		Error:         errMsg,
		Artifacts:     artifacts,
	}, nil
}

func mapWorkflowStatus(status swf.JobStatus) model.WorkflowStatus {
	switch status {
	case swf.JobStatusActive, swf.JobStatusPendingJobs, swf.JobStatusAwaitingFuture, swf.JobStatusReady:
		return model.WorkflowStatusRunning
	case swf.JobStatusCompleted:
		return model.WorkflowStatusCompleted
	case swf.JobStatusCancelled:
		return model.WorkflowStatusCanceled
	case swf.JobStatusExpired:
		return model.WorkflowStatusTimedOut
	case swf.JobStatusCrashConcern:
		return model.WorkflowStatusFailed
	default:
		return model.WorkflowStatusUnknown
	}
}

func mapChapterStatus(payloadKind string) model.ChapterStatus {
	switch payloadKind {
	case "App", "AppChildJob":
		return model.ChapterStatusCompleted
	case "AppError", "SystemError", "Timeout":
		return model.ChapterStatusFailed
	case "":
		return model.ChapterStatusPending
	default:
		return model.ChapterStatusCompleted
	}
}

func extractErrorMessage(payload map[string]interface{}) string {
	if msg, ok := payload["message"].(string); ok {
		return msg
	}
	return ""
}

func parseRawJobPayload(payload json.RawMessage) *map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil
	}
	return &m
}

func statusMatches(filters []model.WorkflowStatus, status model.WorkflowStatus) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f == status {
			return true
		}
	}
	return false
}

func actorFromStartJob(startJob *workflowctl.StartJob) model.Actor {
	actor := model.Actor{Type: model.ActorTypeUser}
	if startJob == nil {
		return actor
	}
	act := startJob.JobContext.Actor
	if act.ActorEmail != "" {
		actor.User = &model.ActorUser{Email: act.ActorEmail}
	}
	return actor
}

func (s *Service) findCellID(ctx context.Context, projectID string, cellName string) (*string, error) {
	if s.cells == nil {
		return nil, fmt.Errorf("cells service unavailable")
	}
	iter, err := s.cells.ListCells(ctx, cell.SearchFilter{
		ProjectIDs: []project.ID{project.ID(projectID)},
		Names:      []string{cellName},
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close(ctx)
	c, err := iter.Next(ctx)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("cell not found")
	}
	id := string(c.ID)
	return &id, nil
}

func (s *Service) loadTicket(ctx context.Context, ticketID string) (*ticket.Ticket, error) {
	if s.tickets == nil {
		return nil, fmt.Errorf("ticket service unavailable")
	}
	at := time.Now().UTC()
	item, err := s.tickets.GetTicketAt(ctx, ticket.ID(ticketID), at)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}
