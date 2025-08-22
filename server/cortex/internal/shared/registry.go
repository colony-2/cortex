package shared

import (
	"fmt"
	gitactivity "github.com/divisive-ai/vibethis/server/git/pkg/activity"
	"github.com/divisive-ai/vibethis/server/ops/pkg/llm"
	opsactivity "github.com/divisive-ai/vibethis/server/ops/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/sleepop"
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
	// Initialize LLM registry for adapters
	if err := llm.InitializeRegistry(); err != nil {
		logger.Warn("Failed to initialize LLM registry", zap.Error(err))
		// Continue anyway - some activities might still work
	}

	registry := worker.NewActivityRegistry()
	err := registry.RegisterAll(
		sleepop.NewSleepActivity(),
		commandop.NewCommandExecutionActivity(),
		append(gitactivity.GetAll(), opsactivity.GetAll()...))
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
func (rm *RegistryManager) GetRegistry() *worker.ActivityRegistry {
	return rm.registry
}

// GetExecutor returns the standalone executor
func (rm *RegistryManager) GetExecutor() *executor.StandaloneExecutor {
	return rm.executor
}
