package worker

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"github.com/vibethis/server/recipe-core"
)

// Registry manages the discovery and tracking of recipes
type Registry struct {
	logger       *zap.Logger
	recipesDir   string
	recipes      map[string]*recipecore.Recipe // key is recipe name
	mu           sync.RWMutex
	watcher      *fsnotify.Watcher
	ctx          context.Context
	cancel       context.CancelFunc
	workerManager *WorkerManager
	hashComputer *recipecore.HashComputer
}

// NewRegistry creates a new recipe registry
func NewRegistry(logger *zap.Logger, recipesDir string, workerManager *WorkerManager) (*Registry, error) {
	absDir, err := filepath.Abs(recipesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve recipes directory: %w", err)
	}

	// Create recipes directory if it doesn't exist
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recipes directory: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	r := &Registry{
		logger:        logger,
		recipesDir:    absDir,
		recipes:       make(map[string]*recipecore.Recipe),
		watcher:       watcher,
		ctx:           ctx,
		cancel:        cancel,
		workerManager: workerManager,
		hashComputer:  recipecore.NewHashComputer(),
	}

	return r, nil
}

// Start begins the recipe discovery and watching process
func (r *Registry) Start() error {
	// Initial discovery
	if err := r.discoverRecipes(); err != nil {
		return fmt.Errorf("initial recipe discovery failed: %w", err)
	}

	// Start watching for changes
	if err := r.watcher.Add(r.recipesDir); err != nil {
		return fmt.Errorf("failed to watch recipes directory: %w", err)
	}

	// Start the file watcher goroutine
	go r.watchForChanges()

	r.logger.Info("Recipe registry started", 
		zap.String("directory", r.recipesDir),
		zap.Int("recipes", len(r.recipes)))

	return nil
}

// Stop stops the recipe registry and file watching
func (r *Registry) Stop() error {
	r.cancel()
	return r.watcher.Close()
}

// GetRecipe returns a recipe by name
func (r *Registry) GetRecipe(name string) (*recipecore.Recipe, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recipe, exists := r.recipes[name]
	if !exists {
		return nil, fmt.Errorf("recipe %q not found", name)
	}

	return recipe, nil
}

// ListRecipes returns all discovered recipes
func (r *Registry) ListRecipes(filter *recipecore.RecipeFilter) ([]*recipecore.Recipe, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var recipes []*recipecore.Recipe
	for _, recipe := range r.recipes {
		// Apply filters
		if filter != nil && filter.Status != nil {
			if recipe.WorkerStatus != *filter.Status {
				continue
			}
		}

		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

// discoverRecipes scans the recipes directory for recipe definitions
func (r *Registry) discoverRecipes() error {
	discovered := make(map[string]*recipecore.Recipe)

	err := filepath.WalkDir(r.recipesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for recipe.yaml in directories
		if d.IsDir() && path != r.recipesDir {
			recipePath := filepath.Join(path, "recipe.yaml")
			if _, err := os.Stat(recipePath); err == nil {
				recipe, err := r.loadMultiFileRecipe(path)
				if err != nil {
					r.logger.Error("Failed to load multi-file recipe", 
						zap.String("path", path), 
						zap.Error(err))
					return nil
				}
				discovered[recipe.Name] = recipe
			}
			return nil
		}

		// Check for single-file recipes (*.yaml files)
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") && d.Name() != "recipe.yaml" {
			recipe, err := r.loadSingleFileRecipe(path)
			if err != nil {
				r.logger.Error("Failed to load single-file recipe", 
					zap.String("path", path), 
					zap.Error(err))
				return nil
			}
			if recipe != nil {
				discovered[recipe.Name] = recipe
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk recipes directory: %w", err)
	}

	// Update registry and manage workers
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop workers for removed recipes
	for name, oldRecipe := range r.recipes {
		if _, exists := discovered[name]; !exists {
			r.logger.Info("Recipe removed", zap.String("name", name))
			if r.workerManager != nil {
				r.workerManager.StopWorker(name)
			}
			oldRecipe.WorkerStatus = recipecore.WorkerStatusStopped
		}
	}

	// Start or restart workers for new/changed recipes
	for name, newRecipe := range discovered {
		oldRecipe, exists := r.recipes[name]
		
		if !exists {
			// New recipe
			r.logger.Info("New recipe discovered", zap.String("name", name))
			newRecipe.WorkerStatus = recipecore.WorkerStatusStarting
			if r.workerManager != nil {
				go r.startWorkerAsync(newRecipe)
			}
		} else if oldRecipe.Hash != newRecipe.Hash {
			// Changed recipe
			r.logger.Info("Recipe changed", 
				zap.String("name", name),
				zap.String("oldHash", oldRecipe.Hash),
				zap.String("newHash", newRecipe.Hash))
			newRecipe.WorkerStatus = recipecore.WorkerStatusStarting
			if r.workerManager != nil {
				r.workerManager.RestartWorker(name, newRecipe)
			}
		} else {
			// No change, preserve worker status
			newRecipe.WorkerStatus = oldRecipe.WorkerStatus
		}
		
		r.recipes[name] = newRecipe
	}

	return nil
}

// loadMultiFileRecipe loads a recipe from a directory with multiple files
func (r *Registry) loadMultiFileRecipe(dir string) (*recipecore.Recipe, error) {
	manifestPath := filepath.Join(dir, "recipe.yaml")
	
	// Load recipe manifest
	var manifest recipecore.RecipeManifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe manifest: %w", err)
	}
	
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse recipe manifest: %w", err)
	}

	recipe := &recipecore.Recipe{
		Name:         manifest.Recipe.Name,
		Version:      manifest.Recipe.Version,
		Description:  manifest.Recipe.Description,
		BasePath:     dir,
		ManifestPath: manifestPath,
	}

	// Load workflow
	if manifest.Recipe.Files.Workflow != "" {
		recipe.WorkflowPath = filepath.Join(dir, manifest.Recipe.Files.Workflow)
		workflow, err := r.loadWorkflowFile(recipe.WorkflowPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load workflow: %w", err)
		}
		recipe.Workflow = workflow
	}

	// Load activities
	if manifest.Recipe.Files.Activities != "" {
		recipe.ActivitiesPath = filepath.Join(dir, manifest.Recipe.Files.Activities)
		activities, err := r.loadActivitiesFile(recipe.ActivitiesPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load activities: %w", err)
		}
		recipe.Activities = activities
	}

	// Load agents
	if manifest.Recipe.Files.Agents != "" {
		recipe.AgentsPath = filepath.Join(dir, manifest.Recipe.Files.Agents)
		agents, err := r.loadAgentsFile(recipe.AgentsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load agents: %w", err)
		}
		recipe.Agents = agents
	}

	// Compute hash and metadata
	recipe.Hash = r.hashComputer.ComputeRecipeHash(recipe)
	info, _ := os.Stat(manifestPath)
	if info != nil {
		recipe.LastModified = info.ModTime()
	}

	return recipe, nil
}

// loadSingleFileRecipe loads a recipe from a single YAML file
func (r *Registry) loadSingleFileRecipe(path string) (*recipecore.Recipe, error) {
	// Parse as project file
	parser := recipecore.NewYamlParser()
	project, err := parser.ParseProject(path)
	if err != nil {
		// Not a valid project file, skip
		return nil, nil
	}

	// Get name and version from manifest or workflow
	name := ""
	version := ""
	description := ""
	
	if project.Manifest != nil {
		name = project.Manifest.Name
		version = project.Manifest.Version
		description = project.Manifest.Description
	} else if project.Workflow != nil {
		name = project.Workflow.Name
		version = project.Workflow.Version
		description = project.Workflow.Description
	}
	
	// Skip if no name found
	if name == "" {
		return nil, nil
	}

	recipe := &recipecore.Recipe{
		Name:         name,
		Version:      version,
		Description:  description,
		BasePath:     filepath.Dir(path),
		ManifestPath: path,
		Project:      project,
		Workflow:     project.Workflow,
		Activities:   project.Activities,
		Agents:       project.Agents,
	}

	// Compute hash and metadata
	recipe.Hash = r.hashComputer.ComputeRecipeHash(recipe)
	info, _ := os.Stat(path)
	if info != nil {
		recipe.LastModified = info.ModTime()
	}

	return recipe, nil
}

// Helper methods to load individual files
func (r *Registry) loadWorkflowFile(path string) (*recipecore.WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var workflow recipecore.WorkflowDefinition
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, err
	}

	return &workflow, nil
}

func (r *Registry) loadActivitiesFile(path string) ([]recipecore.ActivityDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var activities struct {
		Activities []recipecore.ActivityDefinition `yaml:"activities"`
	}
	if err := yaml.Unmarshal(data, &activities); err != nil {
		return nil, err
	}

	return activities.Activities, nil
}

func (r *Registry) loadAgentsFile(path string) (map[string]recipecore.AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var agents struct {
		Agents map[string]recipecore.AgentDefinition `yaml:"agents"`
	}
	if err := yaml.Unmarshal(data, &agents); err != nil {
		return nil, err
	}

	return agents.Agents, nil
}

// watchForChanges monitors the recipes directory for changes
func (r *Registry) watchForChanges() {
	// Debouncing timer to handle rapid changes
	var debounceTimer *time.Timer
	debounce := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
			if err := r.discoverRecipes(); err != nil {
				r.logger.Error("Failed to rediscover recipes", zap.Error(err))
			}
		})
	}

	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			// Filter out temporary files and non-recipe files
			if strings.HasPrefix(filepath.Base(event.Name), ".") ||
				strings.HasSuffix(event.Name, "~") ||
				strings.HasSuffix(event.Name, ".swp") {
				continue
			}

			r.logger.Debug("File system event", 
				zap.String("name", event.Name),
				zap.String("op", event.Op.String()))

			debounce()

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.logger.Error("File watcher error", zap.Error(err))

		case <-r.ctx.Done():
			return
		}
	}
}

// startWorkerAsync starts a worker for a recipe asynchronously
func (r *Registry) startWorkerAsync(recipe *recipecore.Recipe) {
	if err := r.workerManager.StartWorker(recipe); err != nil {
		r.logger.Error("Failed to start worker", 
			zap.String("recipe", recipe.Name),
			zap.Error(err))
		
		r.mu.Lock()
		recipe.WorkerStatus = recipecore.WorkerStatusFailed
		r.mu.Unlock()
	} else {
		r.mu.Lock()
		recipe.WorkerStatus = recipecore.WorkerStatusRunning
		r.mu.Unlock()
	}
}