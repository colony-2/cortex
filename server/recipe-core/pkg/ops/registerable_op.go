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
	// ExecuteV2 runs the op as a Temporal Activity with an explicit invocation descriptor.
	ExecuteV2(inv Invocation, ctx context.Context, input map[string]interface{}) (output map[string]interface{}, err error)

	// ExecuteInlineV2 runs the op inline with an explicit invocation descriptor.
	ExecuteInlineV2(inv Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input map[string]interface{}) (output map[string]interface{}, err error)

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

// InlineHandlerV2 defines the signature for inline handlers that accept an invocation descriptor.
type InlineHandlerV2[In any, Out any] func(inv Invocation, wctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in In) (Out, error)

// ActivityHandlerV2 defines the signature for activity handlers that accept an invocation descriptor.
type ActivityHandlerV2[In any, Out any] func(inv Invocation, actx context.Context, in In) (Out, error)

func NewInlineOpV2[In any, Out any](metadata OpMetadata, handler InlineHandlerV2[In, Out]) RegisterableOp {
	return newInlineOpV2(metadata, handler, nil)
}

func NewInlineOpWithManagementV2[In any, Out any](metadata OpMetadata, handler InlineHandlerV2[In, Out], service ManagementService) RegisterableOp {
	return newInlineOpV2(metadata, handler, service)
}

func newInlineOpV2[In any, Out any](metadata OpMetadata, handler InlineHandlerV2[In, Out], service ManagementService) RegisterableOp {
	return &opSpecImpl[In, Out]{
		metadata:          metadata,
		inlineHandler:     handler,
		managementService: service,
	}
}

func NewActivityMappedOpV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out]) RegisterableOp {
	return newActivityMappedOpV2(metadata, handler, nil, nil)
}

func NewActivityMappedOpWithManagementV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out], service ManagementService) RegisterableOp {
	return newActivityMappedOpV2(metadata, handler, service, nil)
}

func NewActivityMappedOpWithProviderV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out], getInputStruct func() interface{}) RegisterableOp {
	return newActivityMappedOpV2(metadata, handler, nil, getInputStruct)
}

func newActivityMappedOpV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out], service ManagementService, provider func() interface{}) RegisterableOp {
	return &opSpecImpl[In, Out]{
		metadata:          metadata,
		activityHandler:   handler,
		managementService: service,
		inputProvider:     provider,
	}
}

type opSpecImpl[In any, Out any] struct {
	metadata          OpMetadata
	activityHandler   ActivityHandlerV2[In, Out]
	inlineHandler     InlineHandlerV2[In, Out]
	managementService ManagementService
	inputProvider     func() interface{}
}

func (c *opSpecImpl[In, Out]) GetInputStruct() interface{} {
	if c.inputProvider != nil {
		return c.inputProvider()
	}
	return reflect.New(c.GetInputType()).Elem().Interface()
}

func (c *opSpecImpl[In, Out]) GetName() string { return c.metadata.Type }

func (c *opSpecImpl[In, Out]) GetManagementService() ManagementService {
	return c.managementService
}

func (c *opSpecImpl[In, Out]) ExecuteAsActivity() bool { return c.activityHandler != nil }

func (c *opSpecImpl[In, Out]) GetMetadata() OpMetadata {
	return c.metadata
}

func (c *opSpecImpl[In, Out]) ExecuteV2(inv Invocation, ctx context.Context, inputMap map[string]interface{}) (map[string]interface{}, error) {
	if c.activityHandler == nil {
		panic("this must be run inline, not as an activity")
	}
	var input In
	if err := decodeWithJsonTags(inputMap, &input); err != nil {
		return nil, err
	}
	objResult, err := c.activityHandler(inv, ctx, input)
	if err != nil {
		return nil, fmt.Errorf("error executing handler: %w", err)
	}

	s := structs.New(objResult)
	s.TagName = "json" // Use JSON tags instead of default "structs" tags
	return s.Map(), nil
}

func (c *opSpecImpl[In, Out]) ExecuteInlineV2(inv Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, inputMap map[string]interface{}) (map[string]interface{}, error) {
	if c.inlineHandler == nil {
		panic("this must be run as an activity, not inline")
	}
	var input In
	if err := decodeWithJsonTags(inputMap, &input); err != nil {
		return nil, err
	}
	objResult, err := c.inlineHandler(inv, ctx, timeout, retry, input)
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
	if c.inputProvider != nil {
		t := reflect.TypeOf(c.inputProvider())
		if t.Kind() == reflect.Ptr {
			return t.Elem()
		}
		return t
	}
	if c.activityHandler != nil {
		return reflect.ValueOf(c.activityHandler).Type().In(2)
	}
	if c.inlineHandler != nil {
		// Inline handler signature: func(inv Invocation, workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)
		// The input type is the 5th parameter (index 4)
		return reflect.ValueOf(c.inlineHandler).Type().In(4)
	}
	panic("op has no handler to derive input type")
}

func (c *opSpecImpl[In, Out]) GetOutputType() reflect.Type {
	if c.activityHandler != nil {
		return reflect.ValueOf(c.activityHandler).Type().Out(0)
	}
	if c.inlineHandler != nil {
		return reflect.ValueOf(c.inlineHandler).Type().Out(0)
	}
	panic("op has no handler to derive output type")
}

func (c *opSpecImpl[In, Out]) isOpSpec() {}

// confirm opSpecImpl implements OpSpec
var _ RegisterableOp = &opSpecImpl[string, string]{}

// ManagementService provides HTTP endpoints for managing input requests
type ManagementService interface {
	// GetRoutes returns HTTP routes this service provides
	GetRoutes() []Route

	// Initialize with injected dependencies
	Initialize(deps ServiceDependencies2) error
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
