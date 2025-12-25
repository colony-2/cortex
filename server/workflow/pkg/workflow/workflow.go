package workflow

import (
	"context"
	"errors"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/colony2/server/workflow/internal/service"
	"github.com/colony-2/strata-go/pkg/client"
	"github.com/colony-2/swf-go/pkg/swf"
)

type (
	WorkflowStatus       = model.WorkflowStatus
	ChapterStatus        = model.ChapterStatus
	Actor                = model.Actor
	ActorType            = model.ActorType
	ActorUser            = model.ActorUser
	ActorAgent           = model.ActorAgent
	ArtifactReference    = model.ArtifactReference
	ChapterDetail        = model.ChapterDetail
	WorkflowSummary      = model.WorkflowSummary
	WorkflowDetail       = model.WorkflowDetail
	Ticket               = ticket.Ticket
	ListWorkflowsRequest = model.ListWorkflowsRequest
	GetWorkflowRequest   = model.GetWorkflowRequest
)

const (
	WorkflowStatusRunning    = model.WorkflowStatusRunning
	WorkflowStatusCompleted  = model.WorkflowStatusCompleted
	WorkflowStatusFailed     = model.WorkflowStatusFailed
	WorkflowStatusCanceled   = model.WorkflowStatusCanceled
	WorkflowStatusTerminated = model.WorkflowStatusTerminated
	WorkflowStatusTimedOut   = model.WorkflowStatusTimedOut
	WorkflowStatusUnknown    = model.WorkflowStatusUnknown

	ChapterStatusPending   = model.ChapterStatusPending
	ChapterStatusRunning   = model.ChapterStatusRunning
	ChapterStatusCompleted = model.ChapterStatusCompleted
	ChapterStatusFailed    = model.ChapterStatusFailed
	ChapterStatusSkipped   = model.ChapterStatusSkipped

	ActorTypeUser  = model.ActorTypeUser
	ActorTypeAgent = model.ActorTypeAgent
)

var (
	ErrNotFound             = service.ErrNotFound
	ErrWorkflowNotInProject = service.ErrWorkflowNotInProject
)

type Service interface {
	ListWorkflows(ctx context.Context, req ListWorkflowsRequest) ([]WorkflowSummary, error)
	GetWorkflow(ctx context.Context, req GetWorkflowRequest) (*WorkflowDetail, error)
}

type ServiceConfig struct {
	Engine   swf.SWFEngine
	Strata   *client.Client
	Tickets  ticket.Service
	Cells    cell.Service
	Projects project.Service
}

func New(config ServiceConfig) (Service, error) {
	return service.New(service.Config{
		Engine:   config.Engine,
		Strata:   config.Strata,
		Tickets:  config.Tickets,
		Cells:    config.Cells,
		Projects: config.Projects,
	})
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
