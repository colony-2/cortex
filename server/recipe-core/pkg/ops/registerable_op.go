// Package types defines interfaces for activities that can be consumed by recipe-worker
package ops

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
	"github.com/fatih/structs"
	"github.com/mitchellh/mapstructure"
	"gorm.io/gorm"
)

// RegisterableOp defines the contract for ops that can be consumed
// by external systems like recipe-worker via YAML definitions
type RegisterableOp interface {
	// ExecuteV2 runs the op as a task with an explicit invocation descriptor.
	ExecuteV2(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (output map[string]interface{}, err error)

	GetMetadata() OpMetadata
	GetName() string

	GetInputStruct() interface{}
	GetInputType() reflect.Type
	GetOutputType() reflect.Type
	GetManagementService() ManagementService

	isOpSpec()
}

type OpDependencies interface {
	Database() (*gorm.DB, bool)
	AddArtifact(swf.Artifact) error
	GetArtifacts() []swf.Artifact
	WorkflowControl() contextual.TaskWorkflowControl
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

// ActivityHandlerV2 defines the signature for activity handlers that accept an invocation descriptor.
type ActivityHandlerV2[In OpInputType, Out OpOutputType] func(deps OpDependencies, ctx context.Context, in In) (Out, error)

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

func (c *opSpecImpl[In, Out]) ExecuteV2(deps OpDependencies, ctx context.Context, resolvedInput map[string]interface{}) (map[string]interface{}, error) {
	var input In
	err := decodeWithJsonTags(resolvedInput, &input)
	if err != nil {
		return nil, fmt.Errorf("error decoding input: %w", err)
	}

	objResult, err := c.activityHandler(deps, ctx, input)
	if err != nil {
		return nil, fmt.Errorf("error executing op: %w", err)
	}

	s := structs.New(objResult)
	s.TagName = "json" // Use JSON tags instead of default "structs" tags
	return s.Map(), nil
}

func decodeWithJsonTags[T any](data map[string]interface{}, input *T) error {
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
	panic("op has no handler to derive input type")
}

func (c *opSpecImpl[In, Out]) GetOutputType() reflect.Type {
	if c.activityHandler != nil {
		return reflect.ValueOf(c.activityHandler).Type().Out(0)
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
