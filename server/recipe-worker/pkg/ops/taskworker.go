package ops

import (
	"context"
	"encoding/json"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type taskWorker struct {
	name string
	reg  ActivityRegistration
	doer *opExecutor
}

func (t *taskWorker) Name() string {
	return t.name
}

func (t *taskWorker) Run(ctx swf.TaskContext, input swf.TaskData) (swf.TaskData, error) {
	d, err := input.GetData()
	if err != nil {
		return nil, err
	}
	air := ActivityInvocationRequest{}
	if err := json.Unmarshal(d, &air); err != nil {
		return nil, err
	}
	inArt, err := input.GetArtifacts()
	if err != nil {
		return nil, err
	}
	out, outArt, err := t.doer.do(context.Background(), &ops.TaskBasedJobTool{TaskContext: ctx}, air, inArt)
	if err != nil {
		return nil, err
	}
	return swf.NewTaskData(out, outArt...)
}

var _ swf.TaskWorker = &taskWorker{}
