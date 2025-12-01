package task

import (
	"encoding/json"

	"github.com/colony-2/swf-go/pkg/swf"
)

type taskFunc func(Context, any) (any, error)

func AsTask(name string, fn taskFunc) swf.TaskWorker {
	return &worker{
		name: name,
		fn:   fn,
	}
}

type worker struct {
	name string
	fn   taskFunc
}

func (w *worker) Name() string {
	return w.name
}

func (w *worker) Run(context swf.TaskContext, input swf.TaskData) (swf.TaskData, error) {
	d, err := input.GetData()
	if err != nil {
		return nil, err
	}
	out, err := w.fn(Context{TaskContext: context}, d)
	if err != nil {
		return nil, err
	}
	outd, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}

	return &swf.SimpleTaskData{
		Data: outd,
	}, nil

}
