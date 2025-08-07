package statemachine

import (
	"context"
	"fmt"
)

// StateMachineWrapper provides a convenient wrapper for state machine execution
type StateMachineWrapper struct {
	activity *StateMachineActivity
}

// NewStateMachineWrapper creates a new state machine wrapper
func NewStateMachineWrapper(executor ExecutorImplementation) (*StateMachineWrapper, error) {
	activity, err := NewStateMachineActivity(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create state machine activity: %w", err)
	}
	
	return &StateMachineWrapper{
		activity: activity,
	}, nil
}

// Execute runs a state machine with the given configuration
func (w *StateMachineWrapper) Execute(ctx context.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
	return w.activity.Execute(ctx, config, inputs)
}

// ExecuteWithYAML runs a state machine from YAML configuration
func (w *StateMachineWrapper) ExecuteWithYAML(ctx context.Context, yamlConfig string, inputs map[string]interface{}) (map[string]interface{}, error) {
	// This would parse YAML config into StateMachineConfig
	// For now, returning an error as YAML parsing needs to be implemented
	return nil, fmt.Errorf("YAML configuration parsing not yet implemented")
}

// GetActivity returns the underlying state machine activity
func (w *StateMachineWrapper) GetActivity() *StateMachineActivity {
	return w.activity
}