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
)

// RegisterableOp defines the contract for ops that can be consumed
// by external systems like recipe-worker via YAML definitions.
// Multi-step aware: execution happens per TaskStep in TaskChain.
type RegisterableOp interface {
	GetMetadata() OpMetadata
	GetName() string
	TaskChain() []TaskStep
	GetManagementService() ManagementService
	isOpSpec()
}

type HasManagmentService interface {
	GetManagementService() ManagementService
}

// OpMetadata describes the activity for registration and documentation.
// Task-level DisallowAsTask is handled on TaskStep.
type OpMetadata struct {
	Type           string        // Unique identifier for the activity type
	Description    string        // Detailed description
	Version        string        // Semantic version
	DefaultTimeout time.Duration // Default execution timeout
}

type OpExecutor interface {
}

// TaskStep describes an executable step within an op.
type TaskStep struct {
	Name           string
	InputType      reflect.Type
	OutputType     reflect.Type
	DisallowAsTask bool
	NextStepTask   string
	Invoke         func(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error)
}

// Step is an internal interface used by the builder.
type Step interface {
	isStep()
	getName() string
	getInputType() reflect.Type
	getOutputType() reflect.Type
	isDisallowed() bool
	invoke(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error)
}

type stepImpl struct {
	name           string
	inputType      reflect.Type
	outputType     reflect.Type
	disallowAsTask bool
	invokeFn       func(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error)
}

func (s *stepImpl) isStep()                     {}
func (s *stepImpl) getName() string             { return s.name }
func (s *stepImpl) getInputType() reflect.Type  { return s.inputType }
func (s *stepImpl) getOutputType() reflect.Type { return s.outputType }
func (s *stepImpl) isDisallowed() bool          { return s.disallowAsTask }
func (s *stepImpl) invoke(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error) {
	return s.invokeFn(deps, ctx, resolvedInput)
}

// NewStep constructs a step with signature func(ctx context.Context, in In) (Out, error).
func NewStep[In any, Out any](fn func(ctx context.Context, in In) (Out, error)) Step {
	inputType := reflect.TypeOf((*In)(nil)).Elem()
	outputType := reflect.TypeOf((*Out)(nil)).Elem()
	invoker := func(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error) {
		var input In
		if err := decodeWithJsonTags(resolvedInput, &input); err != nil {
			return nil, fmt.Errorf("error decoding input: %w", err)
		}
		objResult, err := fn(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("error executing step: %w", err)
		}
		s := structs.New(objResult)
		s.TagName = "json"
		return s.Map(), nil
	}
	return &stepImpl{
		inputType: inputType, outputType: outputType, invokeFn: invoker,
	}
}

// NewStepWithDeps constructs a step with signature func(deps OpDependencies, ctx context.Context, in In) (Out, error).
func NewStepWithDeps[In any, Out any](fn ActivityHandlerV2[In, Out]) Step {
	inputType := reflect.TypeOf((*In)(nil)).Elem()
	outputType := reflect.TypeOf((*Out)(nil)).Elem()
	invoker := func(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error) {
		var input In
		if err := decodeWithJsonTags(resolvedInput, &input); err != nil {
			return nil, fmt.Errorf("error decoding input: %w", err)
		}
		objResult, err := fn(deps, ctx, input)
		if err != nil {
			return nil, fmt.Errorf("error executing step: %w", err)
		}
		s := structs.New(objResult)
		s.TagName = "json"
		return s.Map(), nil
	}
	return &stepImpl{
		inputType: inputType, outputType: outputType, invokeFn: invoker,
	}
}

// StepDisallow marks a step as disallowed for direct task exposure.
func StepDisallow(step Step) Step {
	if si, ok := step.(*stepImpl); ok {
		si.disallowAsTask = true
	}
	return step
}

// ActivityHandlerV2 defines the signature for activity handlers that accept an invocation descriptor.
type ActivityHandlerV2[In OpInputType, Out OpOutputType] func(deps OpDependencies, ctx context.Context, in In) (Out, error)

// OpBuilder builds a multi-step RegisterableOp.
type OpBuilder interface {
	WithType(t string) OpBuilder
	WithDescription(desc string) OpBuilder
	WithVersion(ver string) OpBuilder
	WithDefaultTimeout(d time.Duration) OpBuilder
	AddStep(name string, step Step) OpBuilder
	WithManagementService(svc ManagementService) OpBuilder
	Build() (RegisterableOp, error)
}

func NewOp() OpBuilder {
	return &opBuilder{}
}

type opBuilder struct {
	metadata OpMetadata
	steps    []Step
	mgmt     ManagementService
}

func (b *opBuilder) WithType(t string) OpBuilder {
	b.metadata.Type = t
	return b
}

func (b *opBuilder) WithDescription(desc string) OpBuilder {
	b.metadata.Description = desc
	return b
}

func (b *opBuilder) WithVersion(ver string) OpBuilder {
	b.metadata.Version = ver
	return b
}

func (b *opBuilder) WithDefaultTimeout(d time.Duration) OpBuilder {
	b.metadata.DefaultTimeout = d
	return b
}

func (b *opBuilder) AddStep(name string, step Step) OpBuilder {
	if si, ok := step.(*stepImpl); ok && si.name == "" {
		si.name = name
	}
	b.steps = append(b.steps, step)
	return b
}

func (b *opBuilder) WithManagementService(svc ManagementService) OpBuilder {
	b.mgmt = svc
	return b
}

func (b *opBuilder) Build() (RegisterableOp, error) {
	if b.metadata.Type == "" {
		return nil, fmt.Errorf("op type is required")
	}
	if len(b.steps) == 0 {
		return nil, fmt.Errorf("at least one step is required")
	}

	steps := make([]TaskStep, 0, len(b.steps))
	for i, st := range b.steps {
		inputT := st.getInputType()
		outputT := st.getOutputType()

		if i > 0 {
			prevOut := steps[i-1].OutputType
			if prevOut == nil || inputT == nil || !prevOut.AssignableTo(inputT) && prevOut != inputT {
				return nil, fmt.Errorf("step %d input type %v does not match previous output %v", i, inputT, prevOut)
			}
		}

		steps = append(steps, TaskStep{
			Name:           st.getName(),
			InputType:      inputT,
			OutputType:     outputT,
			DisallowAsTask: st.isDisallowed(),
			Invoke:         st.invoke,
		})
	}
	for i := range steps {
		if i < len(steps)-1 {
			steps[i].NextStepTask = fmt.Sprintf("%s:%s", b.metadata.Type, steps[i+1].Name)
		}
	}

	return &opChainImpl{
		metadata:          b.metadata,
		steps:             steps,
		managementService: b.mgmt,
	}, nil
}

type opChainImpl struct {
	metadata          OpMetadata
	steps             []TaskStep
	managementService ManagementService
}

func (o *opChainImpl) GetMetadata() OpMetadata { return o.metadata }
func (o *opChainImpl) GetName() string         { return o.metadata.Type }
func (o *opChainImpl) TaskChain() []TaskStep   { return o.steps }
func (o *opChainImpl) GetManagementService() ManagementService {
	return o.managementService
}
func (o *opChainImpl) isOpSpec() {}

// Compatibility helpers: legacy constructors delegate to the builder with a single step.
func NewActivityMappedOpV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out]) RegisterableOp {
	op, err := NewOp().
		WithType(metadata.Type).
		WithDescription(metadata.Description).
		WithVersion(metadata.Version).
		WithDefaultTimeout(metadata.DefaultTimeout).
		AddStep(metadata.Type, NewStepWithDeps(handler)).
		Build()
	if err != nil {
		panic(err)
	}
	return op
}

func NewActivityMappedOpWithManagementV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out], service ManagementService) RegisterableOp {
	op, err := NewOp().
		WithType(metadata.Type).
		WithDescription(metadata.Description).
		WithVersion(metadata.Version).
		WithDefaultTimeout(metadata.DefaultTimeout).
		AddStep(metadata.Type, NewStepWithDeps(handler)).
		WithManagementService(service).
		Build()
	if err != nil {
		panic(err)
	}
	return op
}

func NewActivityMappedOpWithProviderV2[In any, Out any](metadata OpMetadata, handler ActivityHandlerV2[In, Out], getInputStruct func() interface{}) RegisterableOp {
	// provider support is dropped in the new model; fallback to handler-based decoding.
	op, err := NewOp().
		WithType(metadata.Type).
		WithDescription(metadata.Description).
		WithVersion(metadata.Version).
		WithDefaultTimeout(metadata.DefaultTimeout).
		AddStep(metadata.Type, NewStepWithDeps(handler)).
		Build()
	if err != nil {
		panic(err)
	}
	return op
}

func decodeWithJsonTags[T any](data map[string]interface{}, input *T) error {
	coerced, ok := any(input).(*map[string]interface{})
	if ok {
		for k, v := range data {
			(*coerced)[k] = v
		}
	}

	config := &mapstructure.DecoderConfig{
		TagName:     "json", // Use JSON tags instead of mapstructure tags
		Result:      input,
		ErrorUnused: true,
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

// ManagementService provides HTTP endpoints for managing input requests
type ManagementService interface {
	// GetRoutes returns HTTP routes this service provides
	GetRoutes() []Route

	// Initialize with injected dependencies
	Initialize(deps ServiceDependencies2) error
	Close()
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
