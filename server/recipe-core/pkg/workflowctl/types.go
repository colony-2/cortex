package workflowctl

import (
	"context"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/swf-go/pkg/swf"
)

type WorkflowControl interface {
	StartJob(ctx context.Context, req StartJob) (swf.JobId, error)
	Cancel(ctx context.Context, jobId swf.JobId) error
	ListJobs(ctx context.Context, request swf.ListJobsRequest) (jobs []JobItem, nextPage string, err error)
	CompleteTask(ctx context.Context, jobId swf.JobId, taskOrdinal int64, hash string, data any) error
}

type StartJob struct {
	RecipeName string                      `json:"recipe"`
	Inputs     map[string]interface{}      `json:"inputs,omitempty"`
	JobContext contextual.JobContext       `json:"context,omitempty"`
	GitContext contextual.GitCommitContext `json:"git,omitempty"`
}

type JobItem struct {
	TaskData swf.TaskData
	swf.JobSummary
}
