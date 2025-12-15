package workflow

import (
	"log/slog"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type Context struct {
	swf.JobContext
	ops.ServiceDependencies2
}

func (c Context) GetJobId() string {
	return string(c.JobContext.GetJobId())
}

func (c Context) GetLogger() *slog.Logger {
	return slog.Default()
}
