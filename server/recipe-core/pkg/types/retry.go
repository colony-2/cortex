package types

import (
	"fmt"
	"time"
)

// Duration wraps time.Duration to provide custom YAML marshaling/unmarshaling
// It serializes to/from human-readable strings like "1s", "500ms", "2m"
type Duration time.Duration

// MarshalYAML converts Duration to a YAML string
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// UnmarshalYAML parses a YAML string into a Duration
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	*d = Duration(duration)
	return nil
}

// ToDuration converts to standard time.Duration
func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

// String implements the Stringer interface
func (d Duration) String() string {
	return time.Duration(d).String()
}

// RetryPolicy represents a retry configuration that can be serialized to/from YAML
// This struct mirrors Temporal's RetryPolicy but without any Temporal dependencies
type RetryPolicy struct {
	InitialInterval        Duration `yaml:"initial_interval,omitempty"`
	BackoffCoefficient     float64  `yaml:"backoff_coefficient,omitempty"`
	MaximumInterval        Duration `yaml:"maximum_interval,omitempty"`
	MaximumAttempts        int32    `yaml:"maximum_attempts,omitempty"`
	NonRetryableErrorTypes []string `yaml:"non_retryable_error_types,omitempty"`
}
