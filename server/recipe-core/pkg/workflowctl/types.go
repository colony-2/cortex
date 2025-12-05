package workflowctl

import (
	"context"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
)

type WorkflowControl interface {
	StartJob(ctx context.Context, req StartJob) (swf.JobId, error)
	Cancel(ctx context.Context, jobId swf.JobId) error
	ListJobs(ctx context.Context, request swf.ListJobsRequest) (jobs []swf.JobSummary, nextPage string, err error)
}

type StartJob struct {
	RecipeName string                      `json:"recipe"`
	Inputs     map[string]interface{}      `json:"inputs,omitempty"`
	JobContext contextual.JobContext       `json:"context,omitempty"`
	GitContext contextual.GitCommitContext `json:"git,omitempty"`
}
