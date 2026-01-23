package compiler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type validationJobContext struct {
	inner      swf.JobContext
	gitContext contextual.GitCommitContext
}

func (v *validationJobContext) AwaitJobs(jobIds ...string) error {
	return v.AwaitJobs(jobIds...)
}

func wrapValidationContext(ctx workflow.Context, commitContext contextual.GitCommitContext) workflow.Context {
	if _, ok := ctx.JobContext.(*validationJobContext); ok {
		return ctx
	}
	return workflow.Context{
		JobContext:           &validationJobContext{inner: ctx.JobContext, gitContext: commitContext},
		ServiceDependencies2: ctx.ServiceDependencies2,
	}
}

func (v *validationJobContext) GetJobKey() swf.JobKey {
	if v.inner == nil {
		return swf.JobKey{}
	}
	return v.inner.GetJobKey()
}

func (v *validationJobContext) Logger() *slog.Logger {
	if v.inner == nil {
		return slog.Default()
	}
	return v.inner.Logger()
}

func (v *validationJobContext) AwaitDuration(waitFor swf.Duration) error {
	if v.inner == nil {
		return nil
	}
	return v.inner.AwaitDuration(waitFor)
}

func (v *validationJobContext) SpawnAsync(jobType string, data swf.TaskData) (*swf.Future, error) {
	if v.inner == nil {
		return nil, fmt.Errorf("spawn async not supported in validation")
	}
	return v.inner.SpawnAsync(jobType, data)
}

func (v *validationJobContext) DoTask(_ swf.RunPolicy, taskType string, data swf.TaskData) (swf.TaskData, error) {
	parts := strings.SplitN(taskType, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid task type: %s", taskType)
	}

	opName := parts[0]
	stepName := parts[1]
	op, exists := ops.Get(opName)
	if !exists {
		return nil, fmt.Errorf("operation %s not found", opName)
	}
	chain := op.TaskChain()
	stepIndex := -1
	var stepOutputType reflect.Type
	for i, step := range chain {
		if step.Name == stepName {
			stepIndex = i
			stepOutputType = step.OutputType
			break
		}
	}
	if stepIndex == -1 {
		return nil, fmt.Errorf("task step %s not found", stepName)
	}

	zeroOutput, err := zeroOutputFromType(stepOutputType)
	if err != nil {
		return nil, err
	}

	var invocation workerops.ActivityInvocationRequest
	payload, err := data.GetData()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payload, &invocation); err != nil {
		return nil, err
	}

	gitResult := v.gitContext
	if gitResult.ParentHash == "" {
		gitResult.ParentHash = invocation.GitTaskContext.ParentHash
	}
	if gitResult.PersistHash == "" {
		gitResult.PersistHash = invocation.GitTaskContext.PersistHash
	}

	nextTask := ""
	if stepIndex < len(chain)-1 {
		nextTask = fmt.Sprintf("%s:%s", opName, chain[stepIndex+1].Name)
	}

	envelope := workerops.ActivityInvocationOutput{
		OpOutput:  zeroOutput,
		GitResult: gitResult,
		NextTask:  nextTask,
	}

	return swf.NewTaskData(envelope)
}
