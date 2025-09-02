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

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// Registry manages the discovery and tracking of recipes
type Registry struct {
	logger        *zap.Logger
	recipesDir    string
	recipes       map[string]*recipe.RecipeFile // key is recipe name
	mu            sync.RWMutex
	watcher       *fsnotify.Watcher
	ctx           context.Context
	cancel        context.CancelFunc
	workerManager WorkerManagerInterface
	hashComputer  *recipe.HashComputer
}

// NewRegistry creates a new recipe registry
func NewRegistry(logger *zap.Logger, recipesDir string, workerManager WorkerManagerInterface) (*Registry, error) {
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
		recipes:       make(map[string]*recipe.RecipeFile),
		watcher:       watcher,
		ctx:           ctx,
		cancel:        cancel,
		workerManager: workerManager,
		hashComputer:  recipe.NewHashComputer(),
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
func (r *Registry) GetRecipe(name string) (*recipe.RecipeFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recipe, exists := r.recipes[name]
	if !exists {
		return nil, fmt.Errorf("recipe %q not found", name)
	}

	return recipe, nil
}

// ListRecipes returns all discovered recipes
func (r *Registry) ListRecipes(filter *recipe.RecipeFilter) ([]*recipe.RecipeFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var files []*recipe.RecipeFile
	for _, file := range r.recipes {
		// Apply filters
		if filter != nil && filter.Status != nil {
			if file.WorkerStatus != *filter.Status {
				continue
			}
		}

		files = append(files, file)
	}

	return files, nil
}

// discoverRecipes scans the recipes directory for recipe definitions
func (r *Registry) discoverRecipes() error {
	discovered := make(map[string]*recipe.RecipeFile)

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

		// Skip directories - unified format uses single files only
		if d.IsDir() {
			return nil
		}

		// Check for unified recipe files (*.yaml files)
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
			recipe, err := r.loadRecipeFile(path)
			if err != nil {
				r.logger.Error("Failed to load unified recipe",
					zap.String("path", path),
					zap.Error(err))
				return nil
			}
			if recipe != nil {
				discovered[recipe.ID] = recipe
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
				_ = r.workerManager.StopWorker(name)
				// TODO: Handle worker stop errors
			}
			oldRecipe.WorkerStatus = recipe.WorkerStatusStopped
			delete(r.recipes, name) // Remove from registry
		}
	}

	// Start or restart workers for new/changed recipes
	for name, newRecipe := range discovered {
		oldRecipe, exists := r.recipes[name]

		if !exists {
			// New recipe
			r.logger.Info("New recipe discovered", zap.String("name", name))
			newRecipe.WorkerStatus = recipe.WorkerStatusStarting
			if r.workerManager != nil {
				go r.startWorkerAsync(newRecipe)
			}
		} else if oldRecipe.Hash != newRecipe.Hash {
			// Changed recipe
			r.logger.Info("Recipe changed",
				zap.String("name", name),
				zap.String("oldHash", oldRecipe.Hash),
				zap.String("newHash", newRecipe.Hash))
			newRecipe.WorkerStatus = recipe.WorkerStatusStarting
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

// loadRecipeFile loads a recipe from a single unified YAML file
func (reg *Registry) loadRecipeFile(path string) (*recipe.RecipeFile, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rec, err := recipe.LoadRecipeFromReader(f)
	if err != nil {
		return nil, err
	}

	out := &recipe.RecipeFile{
		ID:          rec.GetMetdata().ID,
		Version:     rec.GetMetdata().Version,
		Description: rec.GetMetdata().Desc,
		BasePath:    filepath.Dir(path),
	}

	resolver := recipe.NewSharedNodeResolver(rec.GetMetdata().Defs)
	walker := recipe.NewNodeWalker(resolver)
	newRec, err := walker.Walk(*rec)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve shared nodes: %w", err)
	}

	out.Recipe = newRec
	
	// Compute hash for the recipe
	hashComputer := recipe.NewHashComputer()
	out.Hash = hashComputer.ComputeRecipeHash(&newRec)
	
	return out, nil
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
func (r *Registry) startWorkerAsync(rec *recipe.RecipeFile) {
	if err := r.workerManager.StartWorker(rec); err != nil {
		r.logger.Error("Failed to start worker",
			zap.String("recipe", rec.ID),
			zap.Error(err))

		r.mu.Lock()
		rec.WorkerStatus = recipe.WorkerStatusFailed
		r.mu.Unlock()
	} else {
		r.mu.Lock()
		rec.WorkerStatus = recipe.WorkerStatusRunning
		r.mu.Unlock()
	}
}
