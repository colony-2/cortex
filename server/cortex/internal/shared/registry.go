package shared

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/ops/pkg/llm"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.uber.org/zap"
)

// RegistryManager manages activity registry and executor initialization
type RegistryManager struct {
	registry *ops.ActivityRegistry
	executor *executor.StandaloneExecutor
	logger   *zap.Logger
}

// NewRegistryManager creates a new registry manager with all activities registered
func NewRegistryManager(logger *zap.Logger) (*RegistryManager, error) {
	// Initialize LLM registry for adapters
	if err := llm.InitializeRegistry(); err != nil {
		logger.Warn("Failed to initialize LLM registry", zap.Error(err))
		// Continue anyway - some activities might still work
	}

	registry, err := ops.NewActivityRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to register Git activities: %w", err)
	}

	// Create standalone executor if needed
	exec, err := executor.NewStandaloneExecutor(registry, logger)
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
func (rm *RegistryManager) GetRegistry() *ops.ActivityRegistry {
	return rm.registry
}

// GetExecutor returns the standalone executor
func (rm *RegistryManager) GetExecutor() *executor.StandaloneExecutor {
	return rm.executor
}
