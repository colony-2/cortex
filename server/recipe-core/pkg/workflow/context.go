package workflow

import (
	"log/slog"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

type Context struct {
	swf.JobContext
	ops.ServiceDependencies2
}

func (c Context) GetJobId() string {
	return string(c.JobContext.GetJobId())
}

func (c Context) WithValue(k any, v any) Context {
	if c.extras == nil {
		c.extras = make(map[any]any)
	}
	c.extras[k] = v
	return c
}

func (c Context) Value(k any) any {
	if c.extras == nil {
		return nil
	}
	return c.extras[k]
}

func (c Context) GetLogger() *slog.Logger {
	return slog.Default()
}
