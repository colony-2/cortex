package p2

import (
	"fmt"
	"time"

	"github.com/swaggest/jsonschema-go"
)

type InputMap map[string]interface{}

func (InputMap) InlineJSONSchema()         {}
func (input InputMap) Description() string { return "input configuration mapping" }

type OutputMap map[string]interface{}

func (OutputMap) InlineJSONSchema()         {}
func (input OutputMap) Description() string { return "output schema and mapping" }

// Duration wraps time.Duration to provide custom YAML marshaling/unmarshaling
// It serializes to/from human-readable strings like "1s", "500ms", "2m"
type Duration time.Duration

func (d Duration) Exposer() (jsonschema.Schema, error) {
	var schema jsonschema.Schema
	schema.AddType(jsonschema.String)
	schema.WithDescription("duration string")
	return schema, nil
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
