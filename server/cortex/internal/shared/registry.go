package shared

import (
	"fmt"
	"strings"

	opsactivity "github.com/divisive-ai/vibethis/server/ops/pkg/activity"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"go.uber.org/zap"
)

// RegistryManager manages activity registry and executor initialization
type RegistryManager struct {
	registry *worker.ActivityRegistry
	executor *executor.StandaloneExecutor
	logger   *zap.Logger
}

// NewRegistryManager creates a new registry manager with all activities registered
func NewRegistryManager(logger *zap.Logger) (*RegistryManager, error) {
	registry := worker.NewActivityRegistry()
	activities := opsactivity.GetAll()
	
	for _, act := range activities {
		if err := registry.RegisterGeneric(act); err != nil {
			if !strings.Contains(err.Error(), "already registered") {
				return nil, fmt.Errorf("failed to register activity: %w", err)
			}
		}
	}
	
	// Create standalone executor if needed
	exec, err := executor.NewStandaloneExecutor(logger)
	if err != nil {
		return nil, err
	}
	
	return &RegistryManager{
		registry: registry,
		executor: exec,
		logger:   logger,
	}, nil
}

// GetRegistry returns the activity registry
func (rm *RegistryManager) GetRegistry() *worker.ActivityRegistry {
	return rm.registry
}

// GetExecutor returns the standalone executor
func (rm *RegistryManager) GetExecutor() *executor.StandaloneExecutor {
	return rm.executor
}