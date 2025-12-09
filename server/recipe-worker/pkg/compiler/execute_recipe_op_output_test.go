package compiler

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflow"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
)

// Regression guard: a single-op recipe should surface the op's outputs (lastExecution) back to the caller.
func TestExecuteRecipeSingleOpReturnsOutputs(t *testing.T) {
	originalOps := coreops.List()
	t.Cleanup(func() {
		coreops.Clear()
		if len(originalOps) > 0 {
			coreops.Register(originalOps...)
		}
	})
	coreops.Clear()

	type output struct {
		Value bool `json:"value"`
	}

	opType := "single-op-output"
	op, err := coreops.NewOp().
		WithType(opType).
		AddStep(opType, coreops.NewStepWithDeps(func(_ coreops.OpDependencies, _ context.Context, in map[string]interface{}) (output, error) {
			return output{Value: true}, nil
		})).
		Build()
	require.NoError(t, err)
	coreops.Register(op.(coreops.RegisterableOp))

	jobCtx, gitCtx := GenerateTestContext()

	envelope := workerops.ActivityInvocationOutput{
		OpOutput: map[string]interface{}{"value": true},
		GitResult: contextual.GitCommitContext{
			PersistHash: gitCtx.PersistHash,
			ParentHash:  gitCtx.ParentHash,
		},
		NextTask: "",
	}
	taskData := swf.NewTaskDataOrPanic(envelope)

	stub := &stubJobContext{
		out:      taskData,
		jobID:    "stub-job",
		taskType: opType + ":" + opType,
	}

	ctx := workflow.Context{
		JobContext:           stub,
		ServiceDependencies2: coreops.NewServiceDepsBuilder().Build(),
	}

	rec := recipe.Recipe{
		RecipeImpl: &recipe.RecipeOp{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID:     "single-op-recipe",
					Inputs: map[string]interface{}{},
				},
			},
			OpData: recipe.OpData{Op: opType},
		},
	}

	result, err := ExecuteRecipe(ctx, rec, map[string]interface{}{}, jobCtx, gitCtx)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"value": true}, result, "ExecuteRecipe should return op outputs for single-op recipes")
	require.Equal(t, 1, stub.calls)
	require.Equal(t, stub.taskType, stub.lastTaskType)
}

// Minimal JobContext stub to capture DoTask invocations.
type stubJobContext struct {
	jobID        swf.JobId
	out          swf.TaskData
	calls        int
	taskType     string
	lastTaskType string
}

func (s *stubJobContext) GetJobId() swf.JobId              { return s.jobID }
func (s *stubJobContext) Logger() *slog.Logger             { return slog.Default() }
func (s *stubJobContext) AwaitDuration(swf.Duration) error { return nil }
func (s *stubJobContext) SpawnAsync(string, swf.TaskData) (*swf.Future, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubJobContext) DoTask(_ swf.RunPolicy, taskType string, data swf.TaskData) (swf.TaskData, error) {
	s.calls++
	s.lastTaskType = taskType
	return s.out, nil
}
