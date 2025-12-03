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

func (c Context) GetLogger() *slog.Logger {
	return slog.Default()
}
