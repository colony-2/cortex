package workflowctl

import (
	"context"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/swf-go/pkg/swf"
)

type WorkflowControl interface {
	StartJob(ctx context.Context, req StartJob) (swf.JobKey, error)
	Cancel(ctx context.Context, jobKey swf.JobKey) error
	ListJobs(ctx context.Context, request swf.ListJobsRequest) (jobs []JobItem, nextPage string, err error)
	CompleteTask(ctx context.Context, jobKey swf.JobKey, taskOrdinal int64, hash string, data any) error
	GetWaitingTask(ctx context.Context, jobKey swf.JobKey) (TaskHandle, error)
}

type StartJob struct {
	TenantId   string                 `json:"tenantId"`
	RecipeName string                 `json:"recipe"`
	Inputs     map[string]interface{} `json:"inputs,omitempty"`
	JobContext contextual.JobContext  `json:"context,omitempty"`
	GitRef     string                 `json:"git,omitempty"`
}

type JobItem struct {
	TaskData swf.TaskData
	swf.JobSummary
}

type TaskHandle = swf.TaskHandle
