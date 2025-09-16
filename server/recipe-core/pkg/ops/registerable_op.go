// Package types defines interfaces for activities that can be consumed by recipe-worker
package ops

import (
    "context"
    "fmt"
    "net/http"
    "reflect"
    "time"

    "github.com/fatih/structs"
    "github.com/mitchellh/mapstructure"
    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

// RegisterableOp defines the contract for ops that can be consumed
// by external systems like recipe-worker via YAML definitions
type RegisterableOp interface {
	// Execute will be execute this op as a Temporal Activity
	Execute(ctx context.Context, input map[string]interface{}) (output map[string]interface{}, err error)

	// ExecuteInline will execute the op inline within a workflow.
	ExecuteInline(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input map[string]interface{}) (output map[string]interface{}, err error)

	GetMetadata() OpMetadata
	GetName() string
	ExecuteAsActivity() bool // whether this op maps to a Temporal activity or should be executed inline.

	GetInputStruct() interface{}
	GetInputType() reflect.Type
	GetOutputType() reflect.Type
	GetManagementService() ManagementService

	isOpSpec()
}

type HasManagmentService interface {
	GetManagementService() ManagementService
}

// OpMetadata describes the activity for registration and documentation
type OpMetadata struct {
    Type           string        // Unique identifier for the activity type
    Description    string        // Detailed description
    Version        string        // Semantic version
    DefaultTimeout time.Duration // Default execution timeout
}

type OpExecutor interface {
}

func NewInlineOp[In any, Out any](metadata OpMetadata, handler func(workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)) RegisterableOp {
	return &opSpecImpl[In, Out]{
		metadata:      metadata,
		inlineHandler: handler,
	}
}

func NewActivityMappedOp[In any, Out any](metadata OpMetadata, handler func(context.Context, In) (Out, error)) RegisterableOp {
	return &opSpecImpl[In, Out]{
		metadata: metadata,
		handler:  handler,
	}
}

func NewActivityMappedOpWithManagement[In any, Out any](metadata OpMetadata, handler func(context.Context, In) (Out, error), service ManagementService) RegisterableOp {
	return &opSpecImpl[In, Out]{
		metadata:          metadata,
		handler:           handler,
		managementService: service,
	}
}

type opSpecImpl[In any, Out any] struct {
	metadata          OpMetadata
	handler           func(context.Context, In) (Out, error)
	inlineHandler     func(workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)
	managementService ManagementService
}

func (c *opSpecImpl[In, Out]) GetInputStruct() interface{} {
	return reflect.New(c.GetInputType()).Elem().Interface()
}

func (c *opSpecImpl[In, Out]) GetName() string { return c.metadata.Type }

func (c *opSpecImpl[In, Out]) GetManagementService() ManagementService {
	return c.managementService
}

func (c *opSpecImpl[In, Out]) ExecuteAsActivity() bool {
	return c.handler != nil
}

func (c *opSpecImpl[In, Out]) GetMetadata() OpMetadata {
	return c.metadata
}

func (c *opSpecImpl[In, Out]) Execute(ctx context.Context, inputMap map[string]interface{}) (output map[string]interface{}, err error) {
	if c.handler == nil {
		panic("this must be run inline, not as an activity")
	}

	var input In
	if err := decodeWithJsonTags(inputMap, &input); err != nil {
		return nil, err
	}
	objResult, err := c.handler(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("error executing handler: %w", err)
	}

	s := structs.New(objResult)
	s.TagName = "json" // Use JSON tags instead of default "structs" tags
	return s.Map(), nil
}

func (c *opSpecImpl[In, Out]) ExecuteInline(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, inputMap map[string]interface{}) (output map[string]interface{}, err error) {
	if c.inlineHandler == nil {
		panic("this must be run as an activity, not inline")
	}

	var input In
	if err := decodeWithJsonTags(inputMap, &input); err != nil {
		return nil, err
	}
	objResult, err := c.inlineHandler(ctx, timeout, retry, input)
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

func (c *opSpecImpl[In, Out]) GetInputType() reflect.Type {
    if c.handler != nil {
        return reflect.ValueOf(c.handler).Type().In(1)
    } else {
        // Inline handler signature: func(workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)
        // The input type is the 4th parameter (index 3)
        return reflect.ValueOf(c.inlineHandler).Type().In(3)
    }
}

func (c *opSpecImpl[In, Out]) GetOutputType() reflect.Type {
	if c.handler != nil {
		return reflect.ValueOf(c.handler).Type().Out(0)
	} else {
		return reflect.ValueOf(c.inlineHandler).Type().Out(0)
	}
}

func (c *opSpecImpl[In, Out]) isOpSpec() {}

// confirm opSpecImpl implements OpSpec
var _ RegisterableOp = &opSpecImpl[string, string]{}

// ManagementService provides HTTP endpoints for managing input requests
type ManagementService interface {
    // GetRoutes returns HTTP routes this service provides
    GetRoutes() []Route

    // Initialize with injected dependencies
    Initialize(deps ServiceDependencies) error
    Close()
}

type ServiceDependencies interface {
    Get(name string) (interface{}, error)
}

// SSEManager interface for Server-Sent Events
type SSEManager interface {
	Broadcast(event SSEEvent)
	Subscribe(clientID string) <-chan SSEEvent
	Unsubscribe(clientID string)
}

// SSEEvent represents a server-sent event
type SSEEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// Route represents an HTTP route
type Route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

type OpDef interface {
	GetName() string
	GetInputStruct() interface{}
}
