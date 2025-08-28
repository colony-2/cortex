package recipe

import (
	"fmt"
	"time"

	"github.com/invopop/jsonschema"
)

type InputMap map[string]interface{}

type OutputMap map[string]interface{}

// Duration wraps time.Duration to provide custom YAML marshaling/unmarshaling
// It serializes to/from human-readable strings like "1s", "500ms", "2m"
type Duration time.Duration

func (d Duration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Title:       "Duration",
		Description: "Human friendly duration string",
	}
}

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

type RetryPolicy struct {
	InitialInterval        Duration `yaml:"initial_interval,omitempty"`
	BackoffCoefficient     float64  `yaml:"backoff_coefficient,omitempty"`
	MaximumInterval        Duration `yaml:"maximum_interval,omitempty"`
	MaximumAttempts        int32    `yaml:"maximum_attempts,omitempty"`
	NonRetryableErrorTypes []string `yaml:"non_retryable_error_types,omitempty"`
}
