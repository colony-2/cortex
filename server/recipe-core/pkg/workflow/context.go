package workflow

import (
	"log/slog"

	"github.com/colony-2/swf-go/pkg/swf"
)

type Context struct {
	swf.JobContext
	extras map[any]any
}

func (c Context) GetJobId() string {
	return string(c.JobContext.GetJobId())
}

func (c Context) WithValue(k any, v any) Context {
	c.extras[k] = v
	return c
}

func (c Context) Value(k any) any {
	return c.extras[k]
}

func (c Context) GetLogger() *slog.Logger {
	return slog.Default()
}
