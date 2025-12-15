package registry

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/fsnotify/fsnotify"
)

// Registry manages the discovery and tracking of recipes
type Registry struct {
	logger       *slog.Logger
	recipesDir   string
	recipes      map[string]*recipe.RecipeFile // key is recipe name
	mu           sync.RWMutex
	watcher      *fsnotify.Watcher
	ctx          context.Context
	cancel       context.CancelFunc
	hashComputer *recipe.HashComputer
}

// NewRegistry creates a new recipe registry
func NewRegistry(logger *slog.Logger, recipesDir string) (*Registry, error) {
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
		logger:       logger,
		recipesDir:   absDir,
		recipes:      make(map[string]*recipe.RecipeFile),
		watcher:      watcher,
		ctx:          ctx,
		cancel:       cancel,
		hashComputer: recipe.NewHashComputer(),
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
		slog.String("directory", r.recipesDir),
		slog.Int("recipes", len(r.recipes)))

	return nil
}

// Stop stops the recipe registry and file watching
func (r *Registry) Stop() error {
	r.cancel()
	return r.watcher.Close()
}

// GetRecipe returns a recipe by name
func (r *Registry) GetRecipeFile(name string) (*recipe.RecipeFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recipe, exists := r.recipes[name]
	if !exists {
		return nil, fmt.Errorf("recipe %q not found", name)
	}

	return recipe, nil
}

func (r *Registry) GetRecipe(name string) (*recipe.Recipe, error) {
	f, err := r.GetRecipeFile(name)
	if err != nil {
		return nil, err
	}

	return &f.Recipe, nil
}

func (r *Registry) ListRecipes() []*recipe.RecipeFile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*recipe.RecipeFile, len(r.recipes))
	i := 0
	for _, rx := range r.recipes {
		out[i] = rx
		i++
	}
	return out
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
					slog.String("path", path),
					slog.Any("error", err))
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
	for name, _ := range r.recipes {
		if _, exists := discovered[name]; !exists {
			r.logger.Info("Recipe removed", slog.String("name", name))
			delete(r.recipes, name) // Remove from registry
		}
	}

	// Start or restart workers for new/changed recipes
	for name, newRecipe := range discovered {
		oldRecipe, exists := r.recipes[name]

		if !exists {
			// New recipe
			r.logger.Info("New recipe discovered", slog.String("name", name))
		} else if oldRecipe.Hash != newRecipe.Hash {
			// Changed recipe
			r.logger.Info("Recipe changed",
				slog.String("name", name),
				slog.String("oldHash", oldRecipe.Hash),
				slog.String("newHash", newRecipe.Hash))
		}

		r.recipes[name] = newRecipe
	}

	return nil
}

// loadRecipeFile loads a recipe from a single unified YAML file
func (r *Registry) loadRecipeFile(path string) (*recipe.RecipeFile, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

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

	out.Recipe = *rec

	// Compute hash for the recipe
	hashComputer := recipe.NewHashComputer()
	out.Hash = hashComputer.ComputeRecipeHash(rec)

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
				r.logger.Error("Failed to rediscover recipes", slog.Any("error", err))
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
				slog.String("name", event.Name),
				slog.String("op", event.Op.String()))

			debounce()

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.logger.Error("File watcher error", slog.Any("error", err))

		case <-r.ctx.Done():
			return
		}
	}
}
