package story

import (
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	coreworkflow "github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/swf-go/pkg/swf"
)

type replayResult struct {
	exec *recordingExecutor
	out  map[string]interface{}
	err  error
}

func runRecipeReplay(t *testing.T, rec *recipe.Recipe, inputs map[string]interface{}, execCtx contextual.JobContext, gitCtx contextual.GitCommitContext, jobStatus swf.JobStatus, tasks []swf.TaskRun, opts ...compiler.ExecutionOptions) replayResult {
	t.Helper()

	if inputs == nil {
		inputs = map[string]interface{}{}
	}

	attempts := []swf.JobAttempt{{Attempt: 1, Tasks: tasks}}
	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", jobStatus, attempts, nil)
	tree := newTreeBuilder()
	recJobCtx := newRecordingJobContext(jobCtx, "job", tree)
	exec := newRecordingExecutor(compiler.DefaultRecipeExecutor{}, tree, recJobCtx)

	wCtx := coreworkflow.Context{
		JobContext:           recJobCtx,
		ServiceDependencies2: coreops.NewServiceDepsBuilder().Build(),
	}

	out, _, err := compiler.ExecuteRecipeWithExecutor(exec, wCtx, *rec, inputs, execCtx, gitCtx, opts...)
	return replayResult{exec: exec, out: out, err: err}
}
