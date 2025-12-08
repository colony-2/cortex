package workflow

import (
	"context"
	"fmt"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
)

type SWFWorkflowControl struct {
	engine   swf.SWFEngine
	registry *worker.Registry
}

func (s *SWFWorkflowControl) ListJobs(ctx context.Context, request swf.ListJobsRequest) (jobs []swf.JobSummary, nextPage string, err error) {
	resp, err := s.engine.ListJobs(ctx, request)
	if err != nil {
		return nil, "", err
	}
	return resp.Jobs, resp.NextPageToken, nil
}

func (s *SWFWorkflowControl) CompleteTask(ctx context.Context, jobId swf.JobId, taskOrdinal int64, data swf.TaskData) error {
	handle, err := s.engine.GetWaitingTask(ctx, jobId)
	if err != nil {
		return err
	}
	if handle.TaskOrdinalToComplete() != taskOrdinal {
		return fmt.Errorf("unexpected task ordinal: %d (actual pending: %d)", taskOrdinal, handle.TaskOrdinalToComplete())
	}

	return handle.Finish(ctx, data)
}

func (s *SWFWorkflowControl) StartJob(ctx context.Context, req workflowctl.StartJob) (swf.JobId, error) {
	file, err := s.registry.GetRecipe(req.RecipeName)
	if err != nil {
		return "", err
	}

	return compiler.StartRecipeJob(ctx, req, s.engine, file.Recipe)
}

func (s *SWFWorkflowControl) Cancel(ctx context.Context, jobId swf.JobId) error {
	return s.engine.CancelJob(ctx, swf.CancelJob{JobId: jobId})
}

var _ workflowctl.WorkflowControl = &SWFWorkflowControl{}
