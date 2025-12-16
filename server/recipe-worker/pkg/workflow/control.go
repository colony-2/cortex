package workflow

import (
	"context"
	"fmt"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type RecipeProvider interface {
	GetRecipe(name string) (*recipe.Recipe, error)
}

type SWFWorkflowControl struct {
	Engine   swf.SWFEngine
	Registry RecipeProvider
}

func (s *SWFWorkflowControl) ListJobs(ctx context.Context, request swf.ListJobsRequest) (jobs []workflowctl.JobItem, nextPage string, err error) {
	resp, err := s.Engine.ListJobs(ctx, request)
	if err != nil {
		return nil, "", err
	}

	jobs = make([]workflowctl.JobItem, len(resp.Jobs))
	for i, j := range resp.Jobs {
		jobs[i] = workflowctl.JobItem{
			JobSummary: j,
			TaskData: &taskDataGetter{
				engine:  s.Engine,
				jobID:   j.JobID,
				ordinal: j.TaskWaitInput,
			},
		}
	}

	return jobs, resp.NextPageToken, nil
}

func (s *SWFWorkflowControl) CompleteTask(ctx context.Context, jobId swf.JobId, taskOrdinal int64, hash string, outType any) error {
	handle, err := s.Engine.GetWaitingTask(ctx, jobId)
	if err != nil {
		return err
	}
	if handle.TaskOrdinalToComplete() != taskOrdinal {
		return fmt.Errorf("unexpected task ordinal: %d (actual pending: %d)", taskOrdinal, handle.TaskOrdinalToComplete())
	}

	out := ops.ActivityInvocationOutputRaw{
		GitResult: contextual.GitCommitContext{
			PersistHash: hash,
			ParentHash:  hash,
		},
		Output: outType,
	}

	outData, err := swf.NewTaskData(out)
	if err != nil {
		return err
	}
	return handle.Finish(ctx, outData)
}

func (s *SWFWorkflowControl) StartJob(ctx context.Context, req workflowctl.StartJob) (swf.JobId, error) {
	r, err := s.Registry.GetRecipe(req.RecipeName)
	if err != nil {
		return "", err
	}

	return starter.StartRecipeJob(ctx, req, s.Engine, *r)
}

func (s *SWFWorkflowControl) Cancel(ctx context.Context, jobId swf.JobId) error {
	return s.Engine.CancelJob(ctx, swf.CancelJob{JobId: jobId})
}

type taskDataGetter struct {
	loaded  bool
	engine  swf.SWFEngine
	jobID   swf.JobId
	ordinal *int64
	data    swf.TaskData
}

func (t *taskDataGetter) checkLoad() error {
	if t.loaded {
		return nil
	}
	if t.ordinal == nil {
		return fmt.Errorf("ordinal is required")
	}
	handle, err := t.engine.GetWaitingTask(context.Background(), t.jobID)
	if err != nil {
		return err
	}

	targetCompletion := *t.ordinal + 1
	if handle.TaskOrdinalToComplete() != targetCompletion {
		return fmt.Errorf("unexpected task ordinal: %d (actual pending: %d)", targetCompletion, handle.TaskOrdinalToComplete())
	}

	data, err := handle.Data()
	if err != nil {
		return err
	}
	t.data = data
	t.loaded = true
	return nil
}

func (t *taskDataGetter) GetData() (swf.Data, error) {
	if err := t.checkLoad(); err != nil {
		return nil, err
	}
	return t.data.GetData()
}

func (t *taskDataGetter) GetDataOrPanic() swf.Data {
	d, err := t.GetData()
	if err != nil {
		panic(err)
	}
	return d
}

func (t *taskDataGetter) GetArtifacts() ([]swf.Artifact, error) {
	if err := t.checkLoad(); err != nil {
		return nil, err
	}
	return t.data.GetArtifacts()
}

var _ swf.TaskData = &taskDataGetter{}

var _ workflowctl.WorkflowControl = &SWFWorkflowControl{}
