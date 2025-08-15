package worker

import (
	"context"
	"fmt"
	"sync"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	recipeworker "github.com/divisive-ai/vibethis/server/recipe-worker"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	recipeworkflows "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/workflows"
)

// WorkerManager manages the lifecycle of workers for recipes
type WorkerManager struct {
	logger               *zap.Logger
	temporalClient       client.Client
	workers              map[string]worker.Worker // key is recipe name
	mu                   sync.RWMutex
	taskQueue            string // Base task queue name
	compiler             *compiler.Compiler
	activityRegistry     *compiler.ActivityRegistry
	providerRegistry     *recipeworker.ProviderRegistry
	activityTypeRegistry *recipe.ActivityTypeRegistry
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(logger *zap.Logger, temporalClient client.Client) *WorkerManager {
	activityRegistry := compiler.NewActivityRegistry()
	providerRegistry := recipeworker.NewProviderRegistry()
	activityTypeRegistry := recipe.NewActivityTypeRegistry()
	return &WorkerManager{
		logger:               logger,
		temporalClient:       temporalClient,
		workers:              make(map[string]worker.Worker),
		taskQueue:            "ono-recipes", // Base task queue
		activityRegistry:     activityRegistry,
		compiler:             compiler.NewCompiler(activityRegistry),
		providerRegistry:     providerRegistry,
		activityTypeRegistry: activityTypeRegistry,
	}
}

// StartWorker starts a new worker for a recipe
func (m *WorkerManager) StartWorker(recipe *recipe.Recipe) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worker already exists
	if _, exists := m.workers[recipe.Name]; exists {
		return fmt.Errorf("worker already exists for recipe %q", recipe.Name)
	}

	// Create task queue name for this recipe
	taskQueue := fmt.Sprintf("%s-%s", m.taskQueue, recipe.Name)

	// Create worker options
	workerOptions := worker.Options{
		MaxConcurrentWorkflowTaskPollers: 2,
		MaxConcurrentActivityTaskPollers: 2,
	}

	// Create the worker
	w := worker.New(m.temporalClient, taskQueue, workerOptions)

	// Register unified workflow
	if recipe.Recipe != nil {
		// Create dynamic workflow using the compiler as executor
		workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipe.Recipe, m.compiler)
		
		w.RegisterWorkflowWithOptions(
			workflowFunc,
			workflow.RegisterOptions{
				Name: recipe.Name + "-workflow",
			},
		)
	}

	// Register shared activities directly from Node
	if recipe.Recipe != nil && recipe.Recipe.Shared != nil {
		for name, sharedNode := range recipe.Recipe.Shared {
			m.registerSharedNode(w, name, &sharedNode)
		}
	}

	// Start the worker
	err := w.Start()
	if err != nil {
		return fmt.Errorf("failed to start worker for recipe %q: %w", recipe.Name, err)
	}

	m.workers[recipe.Name] = w
	m.logger.Info("Started worker for recipe", 
		zap.String("recipe", recipe.Name),
		zap.String("taskQueue", taskQueue))

	return nil
}

// StopWorker stops a worker for a recipe
func (m *WorkerManager) StopWorker(recipeName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[recipeName]
	if !exists {
		return fmt.Errorf("worker not found for recipe %q", recipeName)
	}

	w.Stop()
	delete(m.workers, recipeName)

	m.logger.Info("Stopped worker for recipe", zap.String("recipe", recipeName))
	return nil
}

// RestartWorker restarts a worker for a recipe
func (m *WorkerManager) RestartWorker(recipeName string, recipe *recipe.Recipe) error {
	// Stop existing worker if it exists
	_ = m.StopWorker(recipeName)

	// Start new worker
	return m.StartWorker(recipe)
}

// StopAll stops all workers
func (m *WorkerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, w := range m.workers {
		w.Stop()
		m.logger.Info("Stopped worker", zap.String("recipe", name))
	}

	m.workers = make(map[string]worker.Worker)
}

// GetWorkerStatus returns the status of a worker
func (m *WorkerManager) GetWorkerStatus(recipeName string) recipe.WorkerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.workers[recipeName]
	if exists {
		// TODO: Check actual worker health
		return recipe.WorkerStatusRunning
	}

	return recipe.WorkerStatusStopped
}

// GetTaskQueueForRecipe returns the task queue name for a recipe
func (m *WorkerManager) GetTaskQueueForRecipe(recipeName string) string {
	return fmt.Sprintf("%s-%s", m.taskQueue, recipeName)
}

// GetActivityTypeRegistry returns the activity type registry for testing
func (m *WorkerManager) GetActivityTypeRegistry() *recipe.ActivityTypeRegistry {
	return m.activityTypeRegistry
}

// RegisterProvider registers a custom activity provider
// Providers must provide schemas for validation
func (m *WorkerManager) RegisterProvider(provider recipeworker.ActivityProvider) error {
	// First register with provider registry
	if err := m.providerRegistry.Register(provider); err != nil {
		return err
	}
	
	// Get schema information
	configSchema, inputSchema, outputSchema := provider.GetSchemas()
	
	// Require at least one schema
	if configSchema == nil && inputSchema == nil && outputSchema == nil {
		return fmt.Errorf("provider %q must provide at least one schema (config, input, or output)", provider.GetType())
	}
	
	options := provider.GetSchemaOptions()
	
	activityTypeDef := &recipe.ActivityTypeDefinition{
		Type:                   provider.GetType(),
		Description:            provider.GetDescription(),
		ConfigSchema:           recipe.JSONSchema(configSchema),
		InputSchema:            recipe.JSONSchema(inputSchema),
		OutputSchema:           recipe.JSONSchema(outputSchema),
		RequiredConfig:         options.RequiredConfig,
		AllowAdditionalConfig:  options.AllowAdditionalConfig,
		AllowAdditionalInputs:  options.AllowAdditionalInputs,
		AllowAdditionalOutputs: options.AllowAdditionalOutputs,
	}
	
	// Register with activity type registry
	if err := m.activityTypeRegistry.RegisterActivityType(activityTypeDef); err != nil {
		// Rollback provider registration
		// Note: ProviderRegistry doesn't have an Unregister method, so we can't rollback
		// In production, you might want to add an Unregister method
		return fmt.Errorf("failed to register activity type: %w", err)
	}
	
	return nil
}

// Legacy step activity registration has been removed

// registerSharedNode registers a shared node directly
func (m *WorkerManager) registerSharedNode(w worker.Worker, name string, node *yamlpkg.Node) {
	// Register activity by name
	m.activityRegistry.RegisterActivity(name)
	// Also register with shared/ prefix for compatibility
	m.activityRegistry.RegisterActivity("shared/" + name)
	
	// Create activity function
	activityFunc := m.createNodeActivity(name, node)
	
	w.RegisterActivityWithOptions(
		activityFunc,
		activity.RegisterOptions{
			Name: name,
		},
	)
	
	// Also register with shared/ prefix
	w.RegisterActivityWithOptions(
		activityFunc,
		activity.RegisterOptions{
			Name: "shared/" + name,
		},
	)
}

// Legacy step activity creation has been removed

// createNodeActivity creates an activity function from a Node
func (m *WorkerManager) createNodeActivity(name string, node *yamlpkg.Node) interface{} {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Get activity type from node operation or inputs
		activityType := node.Op
		if activityType == "" {
			// Check if type is specified in inputs for backward compatibility
			if typeVal, ok := node.Inputs["type"].(string); ok {
				activityType = typeVal
			} else {
				activityType = "function" // Default
			}
		}
		
		// Check if we have a provider for this activity type
		if m.providerRegistry.Has(activityType) {
			provider, err := m.providerRegistry.Get(activityType)
			if err != nil {
				return nil, fmt.Errorf("failed to get provider: %w", err)
			}
			
			// Merge node inputs with provided inputs
			mergedInputs := make(map[string]interface{})
			for k, v := range inputs {
				mergedInputs[k] = v
			}
			for k, v := range node.Inputs {
				mergedInputs[k] = v
			}
			
			// Execute using the provider
			result, err := provider.Execute(ctx, node.Inputs, mergedInputs)
			if err != nil {
				return nil, err
			}
			
			// Convert result to map if needed
			if resultMap, ok := result.(map[string]interface{}); ok {
				return resultMap, nil
			}
			return map[string]interface{}{"result": result}, nil
		}
		
		// Fall back to default implementation (placeholder)
		return map[string]interface{}{
			"status": "completed",
			"name": name,
			"op": node.Op,
		}, nil
	}
}