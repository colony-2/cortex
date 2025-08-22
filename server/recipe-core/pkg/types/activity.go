// Package types defines interfaces for activities that can be consumed by recipe-worker
package types

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/fatih/structs"
	"github.com/mitchellh/mapstructure"
)

// RegisterableOp defines the contract for ops that can be consumed
// by external systems like recipe-worker via YAML definitions
type RegisterableOp interface {
	// Execute runs the operation with the provided configuration and input using string maps which are automatically mapped to struct types.
	Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (output map[string]interface{}, err error)

	GetMetadata() OpMetadata

	GetHandlerType() reflect.Type

	isOpSpec()
}

// OpMetadata describes the activity for registration and documentation
type OpMetadata struct {
	Type           string        // Unique identifier for the activity type
	Name           string        // Human-readable name
	Description    string        // Detailed description
	Version        string        // Semantic version
	DefaultTimeout time.Duration // Default execution timeout
	RetryPolicy    *RetryPolicy  // Default retry configuration

}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaximumAttempts        int32
	InitialInterval        time.Duration
	BackoffCoefficient     float64
	MaximumInterval        time.Duration
	NonRetryableErrorTypes []string
}

type OpExecutor interface {
}

func NewRegisterableOp[Config any, In any, Out any](metadata OpMetadata, handler func(context.Context, Config, In) (Out, error)) RegisterableOp {
	return &opSpecImpl[Config, In, Out]{
		Metadata: metadata,
		Handler:  handler,
	}
}

type opSpecImpl[Config any, In any, Out any] struct {
	Metadata OpMetadata
	Handler  func(context.Context, Config, In) (Out, error)
}

func (c *opSpecImpl[Config, In, Out]) GetMetadata() OpMetadata {
	return c.Metadata
}

func (c *opSpecImpl[Config, In, Out]) Execute(ctx context.Context, configMap map[string]interface{}, inputMap map[string]interface{}) (output map[string]interface{}, err error) {
	var input In
	if err := decodeWithJsonTags(inputMap, &input); err != nil {
		return nil, err
	}
	var config Config
	if err := decodeWithJsonTags(configMap, &config); err != nil {
		return nil, err
	}
	objResult, err := c.Handler(ctx, config, input)
	if err != nil {
		return nil, fmt.Errorf("error executing handler: %w", err)
	}

	s := structs.New(objResult)
	s.TagName = "json" // Use JSON tags instead of default "structs" tags
	return s.Map(), nil
}

func decodeWithJsonTags[T any](data map[string]interface{}, input *T) error {
	config := &mapstructure.DecoderConfig{
		TagName: "json", // Use JSON tags instead of mapstructure tags
		Result:  input,
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	err = decoder.Decode(data)
	if err != nil {
		return err
	}

	return nil
}

func (c *opSpecImpl[Config, In, Out]) GetHandlerType() reflect.Type {
	return reflect.ValueOf(c.Handler).Type()
}

func (c *opSpecImpl[Config, In, Out]) isOpSpec() {}

// confirm opSpecImpl implements OpSpec
var _ RegisterableOp = &opSpecImpl[string, string, string]{}
