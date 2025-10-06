package ops

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	"github.com/invopop/jsonschema"
	"go.temporal.io/sdk/activity"
)

// ActivityInvocationRequest wraps the invocation metadata and original input payload.
type ActivityInvocationRequest struct {
	Invocation ops.Invocation         `json:"invocation"`
	Input      map[string]interface{} `json:"input"`
}

// ActivityRegistration holds the activity and its generated schemas
type ActivityRegistration struct {
	Activity     ops.RegisterableOp // The generic activity interface
	InputSchema  *jsonschema.Schema
	OutputSchema *jsonschema.Schema
	Metadata     ops.OpMetadata
}

// ActivityRegistry manages all registered activities
type ActivityRegistry struct {
	activities    map[string]ActivityRegistration
	generator     SchemaGenerator
	gitController *gitstate.Controller
	deps          ops.ServiceDependencies2
}

// SchemaGenerator validates struct tags and generates JSON schemas
// This is internal to recipe-worker
type SchemaGenerator interface {
	GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error)
	ValidateStructTags(typ reflect.Type) error
}

// NewActivityRegistry creates a new activity registry
func NewActivityRegistry() (*ActivityRegistry, error) {
	a := &ActivityRegistry{
		activities:    make(map[string]ActivityRegistration),
		generator:     NewDefaultSchemaGenerator(),
		gitController: gitstate.NewController(nil),
		deps:          nil,
	}
	opsList := ops.List()
	for _, op := range opsList {
		if err := a.register(op); err != nil {
			return nil, err
		}
	}
	return a, nil
}

type ActivityRegisterable interface {
	RegisterActivityWithOptions(a interface{}, options activity.RegisterOptions)
}

// SetDependencies makes a dependency container available for invocations produced by this registry.
func (r *ActivityRegistry) SetDependencies(deps ops.ServiceDependencies2) {
	r.deps = deps
}

// Dependencies exposes the current dependency container (may be nil).
func (r *ActivityRegistry) Dependencies() ops.ServiceDependencies2 {
	return r.deps
}

func (r *ActivityRegistry) EnableActivitiesInWorker(worker ActivityRegisterable) {
	for name, registration := range r.activities {
		if !registration.Activity.ExecuteAsActivity() {
			continue
		}
		wrapped := withGitWorkspace(registration, r.gitController, r.deps)
		worker.RegisterActivityWithOptions(wrapped, activity.RegisterOptions{Name: name})
	}
}

func withGitWorkspace(reg ActivityRegistration, controller *gitstate.Controller, deps ops.ServiceDependencies2) func(context.Context, ActivityInvocationRequest) (map[string]interface{}, error) {
	if controller == nil {
		controller = gitstate.NewController(nil)
	}
	return func(ctx context.Context, req ActivityInvocationRequest) (map[string]interface{}, error) {
		input := req.Input
		if input == nil {
			input = map[string]interface{}{}
		}
		gitCtx, err := gitstate.ContextFromRequest(req.Invocation, input)
		if err != nil {
			return nil, err
		}
		if err := controller.PrepareWorkspace(ctx, gitCtx); err != nil {
			return nil, err
		}
		if err := controller.Restore(ctx, gitCtx); err != nil {
			return nil, err
		}
		if req.Invocation.Deps == nil {
			req.Invocation.Deps = deps
		}
		outputs, err := reg.Activity.ExecuteV2(req.Invocation, ctx, input)
		if err != nil {
			return nil, err
		}
		if outputs == nil {
			outputs = make(map[string]interface{})
		}
		if patch, ok := outputs["git_context_patch"]; ok {
			if patchMap, ok := patch.(map[string]interface{}); ok {
				applyGitContextPatch(gitCtx, patchMap)
			}
			delete(outputs, "git_context_patch")
		}
		newHash, updatedCtx, err := controller.Persist(ctx, gitCtx)
		if err != nil {
			return nil, err
		}
		gitstate.InjectPersistResult(outputs, newHash, updatedCtx)
		return outputs, nil
	}
}

func applyGitContextPatch(gitCtx *gitstate.Context, patch map[string]interface{}) {
	if gitCtx == nil || patch == nil {
		return
	}
	for key, raw := range patch {
		str, ok := raw.(string)
		if !ok {
			continue
		}
		str = strings.TrimSpace(str)
		if str == "" {
			continue
		}
		switch strings.ToLower(key) {
		case "base_hash":
			gitCtx.BaseHash = str
		case "persist_hash":
			gitCtx.PersistHash = str
		case "previous_hash":
			gitCtx.PreviousHash = str
		case "blob_store_uri":
			gitCtx.BlobStoreURI = str
		case "thin_pack_path":
			gitCtx.ThinPackPath = str
		case "git_author":
			gitCtx.GitAuthor = str
		}
	}
}

// RegisterGeneric registers any activity without knowing its specific generic types
// This allows dynamic registration of activities from external packages
func (r *ActivityRegistry) register(activity ops.RegisterableOp) error {
	metadata := activity.GetMetadata()
	registration := ActivityRegistration{
		Activity: activity,
		Metadata: metadata,
	}

	// Only generate schemas for activities (inline ops are not activities)
	if activity.ExecuteAsActivity() {
		r.generateSchemasForRegistration(&registration)
	}
	r.activities[metadata.Type] = registration
	return nil
}

// Register accepts any generic RegisterableOp from the activity module
func Register(r *ActivityRegistry, activity ops.RegisterableOp) error {
	metadata := activity.GetMetadata()

	if _, exists := r.activities[metadata.Type]; exists {
		return fmt.Errorf("activity type %s already registered", metadata.Type)
	}

	// Validate all struct fields have json tags before generating schemas
	if err := r.generator.ValidateStructTags(activity.GetInputType()); err != nil {
		return fmt.Errorf("input type validation failed: %w", err)
	}
	if err := r.generator.ValidateStructTags(activity.GetOutputType()); err != nil {
		return fmt.Errorf("output type validation failed: %w", err)
	}

	inputSchema, err := r.generator.GenerateSchema(activity.GetInputType())
	if err != nil {
		return fmt.Errorf("input schema generation failed: %w", err)
	}

	outputSchema, err := r.generator.GenerateSchema(activity.GetOutputType())
	if err != nil {
		return fmt.Errorf("output schema generation failed: %w", err)
	}

	r.activities[metadata.Type] = ActivityRegistration{
		Activity:     activity,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Metadata:     metadata,
	}

	return nil
}

// Get retrieves an activity registration by type
func (r *ActivityRegistry) Get(activityType string) (ActivityRegistration, bool) {
	registration, exists := r.activities[activityType]
	return registration, exists
}

// List returns all registered activity types
func (r *ActivityRegistry) List() []string {
	types := make([]string, 0, len(r.activities))
	for activityType := range r.activities {
		types = append(types, activityType)
	}
	return types
}

// GetAll returns all activity registrations
func (r *ActivityRegistry) GetAll() map[string]ActivityRegistration {
	// Return a copy to prevent external modification
	result := make(map[string]ActivityRegistration)
	for k, v := range r.activities {
		result[k] = v
	}
	return result
}

// UpdateRegistration updates an existing activity registration
// This is used by the schema manager to update schemas after generation
func (r *ActivityRegistry) UpdateRegistration(activityType string, registration ActivityRegistration) {
	r.activities[activityType] = registration
}

// generateSchemasForRegistration generates schemas for an activity using reflection
func (r *ActivityRegistry) generateSchemasForRegistration(registration *ActivityRegistration) {
	// Input is the third parameter
	inputType := registration.Activity.GetInputType()
	if inputType.Kind() != reflect.Interface {
		registration.InputSchema, _ = r.generator.GenerateSchema(inputType)
	}

	// Output is the first return value
	outputType := registration.Activity.GetOutputType()
	if outputType.Kind() != reflect.Interface {
		registration.OutputSchema, _ = r.generator.GenerateSchema(outputType)
	}
}
