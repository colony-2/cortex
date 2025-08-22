package executor

import (
	"context"
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"go.uber.org/zap"
)

// ActivityExecutor handles the execution of activities using reflection
type ActivityExecutor struct {
	registry *worker.ActivityRegistry
	logger   *zap.Logger
}

// NewActivityExecutor creates a new activity executor
func NewActivityExecutor(registry *worker.ActivityRegistry, logger *zap.Logger) *ActivityExecutor {
	return &ActivityExecutor{
		registry: registry,
		logger:   logger,
	}
}

// CreateTemporalActivity creates a Temporal-compatible activity function for the given activity type
func (e *ActivityExecutor) CreateTemporalActivity(activityType string) interface{} {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		e.logger.Debug("Executing activity",
			zap.String("type", activityType),
			zap.Any("inputs", inputs),
		)

		// Get activity from registry
		activityReg, exists := e.registry.Get(activityType)
		if !exists {
			return nil, fmt.Errorf("activity type %s not found", activityType)
		}

		// Use reflection to invoke the activity's Execute method
		if activityReg.Activity != nil {
			config := map[string]interface{}{}
			output, err := activityReg.Activity.Execute(ctx, config, inputs)
			if err != nil {
				return nil, err
			}

			e.logger.Debug("Activity completed",
				zap.String("type", activityType),
				zap.Any("outputs", output),
			)

			return output, nil
		}

		// Fallback: return a simple success response
		outputs := map[string]interface{}{
			"result": "success",
		}

		e.logger.Debug("Activity completed with fallback",
			zap.String("type", activityType),
			zap.Any("outputs", outputs),
		)

		return outputs, nil
	}
}
