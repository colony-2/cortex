package workflowctl

import (
	"context"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/swf-go/pkg/swf"
)

type WorkflowControl interface {
	StartJob(ctx context.Context, req StartJob) (swf.JobKey, error)
	Cancel(ctx context.Context, jobKey swf.JobKey) error
	ListJobs(ctx context.Context, request swf.ListJobsRequest) (jobs []JobItem, nextPage string, err error)
	CompleteTask(ctx context.Context, jobKey swf.JobKey, taskOrdinal int64, hash string, data any) error
	GetWaitingTask(ctx context.Context, jobKey swf.JobKey) (TaskHandle, error)
	GetArtifactLazy(ctx context.Context, tenantId string, key swf.ArtifactKey) swf.Artifact
	JobResult(ctx context.Context, key swf.JobKey) (swf.JobData, error)
}

type StartJob struct {
	TenantId    string                 `json:"tenantId"`
	RecipeName  string                 `json:"recipe"`
	Inputs      map[string]interface{} `json:"inputs,omitempty"`
	Artifacts   []swf.Artifact         `json:"artifacts,omitempty"`
	JobContext  contextual.JobContext  `json:"context,omitempty"`
	GitRef      string                 `json:"git,omitempty"`
	SubmittedAt *time.Time             `json:"submitted_at,omitempty"`
	InputHash   string                 `json:"input_hash,omitempty"`
}

type JobItem struct {
	TaskData swf.TaskData
	swf.JobSummary
}

type TaskHandle = swf.TaskHandle
