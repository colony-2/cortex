package worker

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"gorm.io/gorm"
)

func newWorkerDependencies(db *gorm.DB) ops.ServiceDependencies2 {
	builder := ops.NewServiceDepsBuilder().
		WithSSEManager(noopSSEManager{}).
		WithDatabase(db)
	return builder.Build()
}

type noopSSEManager struct{}

func (noopSSEManager) Broadcast(event ops.SSEEvent) {}

func (noopSSEManager) Subscribe(clientID string) <-chan ops.SSEEvent {
	ch := make(chan ops.SSEEvent)
	close(ch)
	return ch
}

func (noopSSEManager) Unsubscribe(clientID string) {}
