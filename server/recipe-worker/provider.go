package worker

import (
	"context"
	"fmt"
	"sync"
)

// ActivityProvider defines the interface for custom activity implementations
// Following Temporal's activity pattern with generic type safety
type ActivityProvider interface {
	// GetType returns the activity type identifier (e.g., "http", "database", "custom_api")
	GetType() string
	
	// Execute runs the activity with the given arguments
	// The args are: config (activity configuration) and inputs (runtime inputs)
	// Returns the activity result or an error
	Execute(ctx context.Context, args ...interface{}) (interface{}, error)
}

// TypedActivityProvider provides a generic interface for strongly-typed activity providers
type TypedActivityProvider[TConfig any, TInput any, TOutput any] interface {
	GetType() string
	ExecuteTyped(ctx context.Context, config TConfig, input TInput) (TOutput, error)
}

// TypedProviderAdapter adapts a TypedActivityProvider to the ActivityProvider interface
type TypedProviderAdapter[TConfig any, TInput any, TOutput any] struct {
	activityType string
	executor     func(ctx context.Context, config TConfig, input TInput) (TOutput, error)
}

// NewTypedProvider creates a new typed provider adapter
func NewTypedProvider[TConfig any, TInput any, TOutput any](
	activityType string,
	executor func(ctx context.Context, config TConfig, input TInput) (TOutput, error),
) ActivityProvider {
	return &TypedProviderAdapter[TConfig, TInput, TOutput]{
		activityType: activityType,
		executor:     executor,
	}
}

func (a *TypedProviderAdapter[TConfig, TInput, TOutput]) GetType() string {
	return a.activityType
}

func (a *TypedProviderAdapter[TConfig, TInput, TOutput]) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("expected 2 arguments (config, input), got %d", len(args))
	}
	
	config, ok := args[0].(TConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected %T", *new(TConfig))
	}
	
	input, ok := args[1].(TInput)
	if !ok {
		return nil, fmt.Errorf("invalid input type: expected %T", *new(TInput))
	}
	
	return a.executor(ctx, config, input)
}

// ProviderRegistry manages registered activity providers
type ProviderRegistry struct {
	providers map[string]ActivityProvider
	mu        sync.RWMutex
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ActivityProvider),
	}
}

// Register adds a new activity provider to the registry
func (r *ProviderRegistry) Register(provider ActivityProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	activityType := provider.GetType()
	if activityType == "" {
		return fmt.Errorf("activity provider must have a non-empty type")
	}
	
	if _, exists := r.providers[activityType]; exists {
		return fmt.Errorf("activity provider for type %q is already registered", activityType)
	}
	
	r.providers[activityType] = provider
	return nil
}

// RegisterTyped registers a typed activity provider
func RegisterTyped[TConfig any, TInput any, TOutput any](
	r *ProviderRegistry,
	activityType string,
	executor func(ctx context.Context, config TConfig, input TInput) (TOutput, error),
) error {
	provider := NewTypedProvider(activityType, executor)
	return r.Register(provider)
}

// Get retrieves an activity provider by type
func (r *ProviderRegistry) Get(activityType string) (ActivityProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	provider, exists := r.providers[activityType]
	if !exists {
		return nil, fmt.Errorf("no provider registered for activity type %q", activityType)
	}
	
	return provider, nil
}

// Has checks if a provider is registered for the given type
func (r *ProviderRegistry) Has(activityType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	_, exists := r.providers[activityType]
	return exists
}

// ListTypes returns all registered activity types
func (r *ProviderRegistry) ListTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	types := make([]string, 0, len(r.providers))
	for activityType := range r.providers {
		types = append(types, activityType)
	}
	return types
}