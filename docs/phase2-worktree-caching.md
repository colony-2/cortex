# Phase 2: Worktree Reuse Optimization

## Goal
Improve performance by reusing worktrees across sequential task executions on the same node, avoiding redundant git clone operations.

## Problem
Currently (after Phase 1), each activity invocation:
1. Creates a new temporary worktree with `os.MkdirTemp()`
2. Clones the repository (expensive)
3. Cleans up the worktree after execution

For a job with 3 sequential tasks on the same node, this means 3 full clones of the same repository.

## Solution
Add a worktree cache to `ActivityRegistry` that:
- Maintains a pool of inactive worktrees available for reuse
- Tracks active worktrees currently in use by running activities
- Implements eviction policies (LRU, TTL) to prevent filesystem bloat
- Handles concurrency safely (prevents multiple tasks from using same worktree simultaneously)

---

## Architecture

### Worktree Cache Structure

```go
type WorktreeCache struct {
    mu sync.RWMutex

    // Active worktrees currently in use by running activities
    active map[WorktreeCacheKey]*WorktreeCacheEntry

    // Inactive worktrees available for reuse
    inactive map[WorktreeCacheKey]*WorktreeCacheEntry

    // Configuration
    config WorktreeCacheConfig

    // Base path for all cached worktrees
    basePath string
}

type WorktreeCacheEntry struct {
    path         string
    key          WorktreeCacheKey
    lastUsed     time.Time
    mu           sync.Mutex      // Per-entry lock
    inUse        bool            // Prevent concurrent access
    refCount     int32           // Atomic operations (future: for nested access)

    // Git state for validation
    currentHash  string
    baseRepo     string
}

type WorktreeCacheKey struct {
    BaseRepo string
    CellPath string
    BaseRef  string  // CRITICAL: Different branches need different worktrees
}

type WorktreeCacheConfig struct {
    Enabled  bool
    MaxSize  int           // Maximum number of worktrees to cache
    MaxAge   time.Duration // Evict after idle time
    BasePath string        // Base directory for cached worktrees
}
```

**Why BaseRef is in the cache key:**
- Multiple jobs on different branches would conflict without it
- Git operations assume worktree is on correct branch
- Prevents state corruption between parallel jobs

---

## Implementation Steps

### 1. Create WorktreeCache Data Structure
**File:** `/src/server/recipe-worker/pkg/ops/worktree_cache.go` (new file)

```go
package ops

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/colony-2/colony2/server/git/pkg/gitstate"
)

type WorktreeCache struct {
    mu       sync.RWMutex
    active   map[WorktreeCacheKey]*WorktreeCacheEntry
    inactive map[WorktreeCacheKey]*WorktreeCacheEntry
    config   WorktreeCacheConfig
    basePath string
    stopCh   chan struct{}
}

func NewWorktreeCache(config WorktreeCacheConfig) (*WorktreeCache, error) {
    if config.BasePath == "" {
        config.BasePath = filepath.Join(os.TempDir(), "colony-worktrees")
    }
    if err := os.MkdirAll(config.BasePath, 0755); err != nil {
        return nil, fmt.Errorf("create cache base path: %w", err)
    }

    cache := &WorktreeCache{
        active:   make(map[WorktreeCacheKey]*WorktreeCacheEntry),
        inactive: make(map[WorktreeCacheKey]*WorktreeCacheEntry),
        config:   config,
        basePath: config.BasePath,
        stopCh:   make(chan struct{}),
    }

    // Start background eviction goroutine
    if config.Enabled {
        go cache.evictionLoop()
    }

    return cache, nil
}

// Acquire gets a worktree from cache or creates a new one
func (c *WorktreeCache) Acquire(ctx context.Context, key WorktreeCacheKey, gitCtx *gitstate.GlobalGitTaskContext) (string, func(), error) {
    if !c.config.Enabled {
        // Cache disabled, create temporary worktree
        return c.createTemporary(ctx, key)
    }

    c.mu.Lock()
    entry, exists := c.inactive[key]

    if exists {
        entry.mu.Lock()
        if entry.inUse {
            // Another goroutine got it first, create new instead
            entry.mu.Unlock()
            c.mu.Unlock()
            return c.createNew(ctx, key, gitCtx)
        }
        entry.inUse = true
        entry.mu.Unlock()

        // Move to active cache
        delete(c.inactive, key)
        c.active[key] = entry
        c.mu.Unlock()

        releaseFn := func() {
            c.release(key, entry)
        }

        return entry.path, releaseFn, nil
    }

    c.mu.Unlock()
    return c.createNew(ctx, key, gitCtx)
}

// Release returns a worktree to the inactive pool
func (c *WorktreeCache) release(key WorktreeCacheKey, entry *WorktreeCacheEntry) {
    c.mu.Lock()
    defer c.mu.Unlock()

    entry.mu.Lock()
    entry.inUse = false
    entry.lastUsed = time.Now()
    entry.mu.Unlock()

    delete(c.active, key)
    c.inactive[key] = entry
}

// createNew creates a new worktree and adds it to the cache
func (c *WorktreeCache) createNew(ctx context.Context, key WorktreeCacheKey, gitCtx *gitstate.GlobalGitTaskContext) (string, func(), error) {
    // Generate unique path based on key
    path := c.generatePath(key)

    entry := &WorktreeCacheEntry{
        path:        path,
        key:         key,
        lastUsed:    time.Now(),
        inUse:       true,
        baseRepo:    key.BaseRepo,
        currentHash: gitCtx.PersistHash,
    }

    c.mu.Lock()
    c.active[key] = entry
    c.mu.Unlock()

    releaseFn := func() {
        c.release(key, entry)
    }

    return path, releaseFn, nil
}

// createTemporary creates a non-cached temporary worktree
func (c *WorktreeCache) createTemporary(ctx context.Context, key WorktreeCacheKey) (string, func(), error) {
    path, err := os.MkdirTemp("", "recipe-worktree-*")
    if err != nil {
        return "", nil, fmt.Errorf("create temp worktree: %w", err)
    }

    releaseFn := func() {
        os.RemoveAll(path)
    }

    return path, releaseFn, nil
}

// generatePath creates a deterministic path for a cache key
func (c *WorktreeCache) generatePath(key WorktreeCacheKey) string {
    // Hash the key to create a unique but deterministic directory name
    h := sha256.New()
    h.Write([]byte(key.BaseRepo))
    h.Write([]byte(key.CellPath))
    h.Write([]byte(key.BaseRef))
    hash := hex.EncodeToString(h.Sum(nil))[:16]

    return filepath.Join(c.basePath, hash)
}

// evictionLoop runs periodically to evict old/excess worktrees
func (c *WorktreeCache) evictionLoop() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.evict()
        case <-c.stopCh:
            return
        }
    }
}

// evict removes old or excess worktrees
func (c *WorktreeCache) evict() {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := time.Now()

    // Evict by age (TTL)
    if c.config.MaxAge > 0 {
        for key, entry := range c.inactive {
            entry.mu.Lock()
            if now.Sub(entry.lastUsed) > c.config.MaxAge {
                os.RemoveAll(entry.path)
                delete(c.inactive, key)
            }
            entry.mu.Unlock()
        }
    }

    // Evict by size (LRU)
    if c.config.MaxSize > 0 && len(c.inactive) > c.config.MaxSize {
        // Sort by lastUsed and remove oldest
        type entryWithKey struct {
            key   WorktreeCacheKey
            entry *WorktreeCacheEntry
        }

        entries := make([]entryWithKey, 0, len(c.inactive))
        for k, e := range c.inactive {
            entries = append(entries, entryWithKey{k, e})
        }

        // Sort by lastUsed (oldest first)
        sort.Slice(entries, func(i, j int) bool {
            return entries[i].entry.lastUsed.Before(entries[j].entry.lastUsed)
        })

        // Remove excess
        toRemove := len(c.inactive) - c.config.MaxSize
        for i := 0; i < toRemove; i++ {
            entry := entries[i].entry
            key := entries[i].key
            os.RemoveAll(entry.path)
            delete(c.inactive, key)
        }
    }
}

// Shutdown stops the eviction loop and cleans up all worktrees
func (c *WorktreeCache) Shutdown() {
    close(c.stopCh)

    c.mu.Lock()
    defer c.mu.Unlock()

    // Clean up all inactive worktrees
    for key, entry := range c.inactive {
        os.RemoveAll(entry.path)
        delete(c.inactive, key)
    }

    // Note: Don't clean up active worktrees (still in use)
}
```

---

### 2. Add Cache to ActivityRegistry
**File:** `/src/server/recipe-worker/pkg/ops/activity_registry.go`

```go
type ActivityRegistry struct {
    activities    map[string]ActivityRegistration
    generator     SchemaGenerator
    gitController *gitstate.Controller
    deps          ops.ServiceDependencies2
    worktreeCache *WorktreeCache // NEW
}

func NewActivityRegistry() (*ActivityRegistry, error) {
    // Default config: cache disabled for backward compatibility
    cacheConfig := WorktreeCacheConfig{
        Enabled:  false, // Enable via SetWorktreeCacheConfig
        MaxSize:  100,
        MaxAge:   1 * time.Hour,
        BasePath: "",
    }

    cache, err := NewWorktreeCache(cacheConfig)
    if err != nil {
        return nil, fmt.Errorf("create worktree cache: %w", err)
    }

    a := &ActivityRegistry{
        activities:    make(map[string]ActivityRegistration),
        generator:     NewDefaultSchemaGenerator(),
        gitController: gitstate.NewController(nil),
        deps:          ops.NewServiceDepsBuilder().Build(),
        worktreeCache: cache,
    }
    // ... rest of initialization
    return a, nil
}

// SetWorktreeCacheConfig allows configuring the worktree cache
func (r *ActivityRegistry) SetWorktreeCacheConfig(config WorktreeCacheConfig) error {
    cache, err := NewWorktreeCache(config)
    if err != nil {
        return err
    }

    // Shutdown old cache
    if r.worktreeCache != nil {
        r.worktreeCache.Shutdown()
    }

    r.worktreeCache = cache
    return nil
}
```

---

### 3. Modify withGitWorkspace to Use Cache
**File:** `/src/server/recipe-worker/pkg/ops/activity_registry.go`

```go
func withGitWorkspace(deps ops.ServiceDependencies2, reg ActivityRegistration, controller *gitstate.Controller, cache *WorktreeCache) func(context.Context, ActivityInvocationRequest, []swf.Artifact) (ActivityInvocationOutput, []swf.Artifact, error) {
    if controller == nil {
        controller = gitstate.NewController(nil)
    }
    return func(ctx context.Context, req ActivityInvocationRequest, inputArtifacts []swf.Artifact) (output ActivityInvocationOutput, outputArtifacts []swf.Artifact, err error) {
        var zero ActivityInvocationOutput

        // Build cache key from request
        cacheKey := WorktreeCacheKey{
            BaseRepo: req.GitTaskContext.BaseRepo,
            CellPath: req.GitTaskContext.CellPath,
            BaseRef:  req.GitTaskContext.BaseRef,
        }

        // Acquire worktree from cache (or create new)
        worktreePath, release, err := cache.Acquire(ctx, cacheKey, &req.GitTaskContext)
        if err != nil {
            return zero, nil, fmt.Errorf("acquire worktree: %w", err)
        }
        defer release()

        // Build full GitTaskContext for controller
        fullContext := &gitstate.GitTaskContext{
            GlobalGitTaskContext: &req.GitTaskContext,
            WorktreePath:         worktreePath,
        }

        // ... rest of implementation unchanged ...
    }
}

// Update GetTaskWorkers to pass cache
func (r *ActivityRegistry) GetTaskWorkers(deps ops.ServiceDependencies2) []swf.TaskWorker {
    workers := make([]swf.TaskWorker, 0, len(r.activities))
    for name, registration := range r.activities {
        if registration.Step.DisallowAsTask {
            continue
        }
        wrapped := withGitWorkspace(deps, registration, r.gitController, r.worktreeCache)
        workers = append(workers, &taskWorker{name: name, reg: registration, fn: wrapped})
    }
    return workers
}
```

---

### 4. Add Configuration Support
**File:** `/src/server/recipe-worker/pkg/config/config.go` (or equivalent)

```go
type Config struct {
    // ... existing config fields ...

    WorktreeCache WorktreeCacheSettings `yaml:"worktree_cache"`
}

type WorktreeCacheSettings struct {
    Enabled  bool   `yaml:"enabled"`
    MaxSize  int    `yaml:"max_size"`
    MaxAge   string `yaml:"max_age"` // e.g., "1h", "30m"
    BasePath string `yaml:"base_path"`
}

// Convert to WorktreeCacheConfig
func (s WorktreeCacheSettings) ToConfig() (ops.WorktreeCacheConfig, error) {
    maxAge, err := time.ParseDuration(s.MaxAge)
    if err != nil && s.MaxAge != "" {
        return ops.WorktreeCacheConfig{}, fmt.Errorf("parse max_age: %w", err)
    }

    return ops.WorktreeCacheConfig{
        Enabled:  s.Enabled,
        MaxSize:  s.MaxSize,
        MaxAge:   maxAge,
        BasePath: s.BasePath,
    }, nil
}
```

**Example config.yaml:**
```yaml
worktree_cache:
  enabled: true
  max_size: 100
  max_age: "1h"
  base_path: "/tmp/colony-worktrees"
```

---

### 5. Add Monitoring/Metrics

```go
type WorktreeCacheMetrics struct {
    Hits        int64
    Misses      int64
    Evictions   int64
    ActiveSize  int
    InactiveSize int
}

func (c *WorktreeCache) Metrics() WorktreeCacheMetrics {
    c.mu.RLock()
    defer c.mu.RUnlock()

    return WorktreeCacheMetrics{
        Hits:         atomic.LoadInt64(&c.hits),
        Misses:       atomic.LoadInt64(&c.misses),
        Evictions:    atomic.LoadInt64(&c.evictions),
        ActiveSize:   len(c.active),
        InactiveSize: len(c.inactive),
    }
}

// Add to Acquire method:
func (c *WorktreeCache) Acquire(...) {
    // ...
    if exists {
        atomic.AddInt64(&c.hits, 1)
        // ...
    } else {
        atomic.AddInt64(&c.misses, 1)
        // ...
    }
}

// Add to evict method:
func (c *WorktreeCache) evict() {
    // ... when removing entries ...
    atomic.AddInt64(&c.evictions, 1)
}
```

---

## Testing

### Unit Tests

```go
// Test cache acquisition and release
func TestWorktreeCacheAcquireRelease(t *testing.T) {
    config := WorktreeCacheConfig{
        Enabled:  true,
        MaxSize:  10,
        MaxAge:   1 * time.Hour,
        BasePath: t.TempDir(),
    }
    cache, err := NewWorktreeCache(config)
    require.NoError(t, err)
    defer cache.Shutdown()

    key := WorktreeCacheKey{
        BaseRepo: "test-repo",
        CellPath: "cell",
        BaseRef:  "main",
    }

    // First acquisition - should create new
    path1, release1, err := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
    require.NoError(t, err)
    require.NotEmpty(t, path1)

    // Release
    release1()

    // Second acquisition - should reuse
    path2, release2, err := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
    require.NoError(t, err)
    require.Equal(t, path1, path2) // Same path = cache hit

    release2()
}

// Test concurrent acquisition doesn't conflict
func TestWorktreeCacheConcurrentAcquisition(t *testing.T) {
    config := WorktreeCacheConfig{
        Enabled:  true,
        MaxSize:  10,
        MaxAge:   1 * time.Hour,
        BasePath: t.TempDir(),
    }
    cache, err := NewWorktreeCache(config)
    require.NoError(t, err)
    defer cache.Shutdown()

    key := WorktreeCacheKey{
        BaseRepo: "test-repo",
        CellPath: "cell",
        BaseRef:  "main",
    }

    // Acquire and hold
    path1, release1, err := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
    require.NoError(t, err)

    // Try to acquire same key concurrently
    path2, release2, err := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
    require.NoError(t, err)

    // Should get different paths (no conflict)
    require.NotEqual(t, path1, path2)

    release1()
    release2()
}

// Test eviction by age
func TestWorktreeCacheEvictionByAge(t *testing.T) {
    config := WorktreeCacheConfig{
        Enabled:  true,
        MaxSize:  100,
        MaxAge:   100 * time.Millisecond,
        BasePath: t.TempDir(),
    }
    cache, err := NewWorktreeCache(config)
    require.NoError(t, err)
    defer cache.Shutdown()

    key := WorktreeCacheKey{
        BaseRepo: "test-repo",
        CellPath: "cell",
        BaseRef:  "main",
    }

    path, release, err := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
    require.NoError(t, err)
    release()

    // Wait for eviction
    time.Sleep(200 * time.Millisecond)
    cache.evict()

    // Should be evicted
    require.Equal(t, 0, len(cache.inactive))
}

// Test eviction by size (LRU)
func TestWorktreeCacheEvictionBySize(t *testing.T) {
    config := WorktreeCacheConfig{
        Enabled:  true,
        MaxSize:  2,
        MaxAge:   0,
        BasePath: t.TempDir(),
    }
    cache, err := NewWorktreeCache(config)
    require.NoError(t, err)
    defer cache.Shutdown()

    // Add 3 entries
    for i := 0; i < 3; i++ {
        key := WorktreeCacheKey{
            BaseRepo: fmt.Sprintf("repo-%d", i),
            CellPath: "cell",
            BaseRef:  "main",
        }
        path, release, err := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
        require.NoError(t, err)
        release()
        time.Sleep(10 * time.Millisecond) // Ensure different lastUsed
    }

    // Trigger eviction
    cache.evict()

    // Should only have 2 (newest) entries
    require.Equal(t, 2, len(cache.inactive))
}
```

### Integration Tests

```go
// Test sequential tasks reuse worktree
func TestSequentialTasksReuseWorktree(t *testing.T) {
    // Setup registry with caching enabled
    registry, err := NewActivityRegistry()
    require.NoError(t, err)

    config := WorktreeCacheConfig{
        Enabled:  true,
        MaxSize:  10,
        MaxAge:   10 * time.Minute,
        BasePath: t.TempDir(),
    }
    err = registry.SetWorktreeCacheConfig(config)
    require.NoError(t, err)

    // Execute 3 sequential tasks with same repo/cell/ref
    for i := 0; i < 3; i++ {
        req := ActivityInvocationRequest{
            Input: map[string]interface{}{"step": i},
            GitTaskContext: gitstate.GlobalGitTaskContext{
                BaseRepo: "test-repo",
                CellPath: "cell",
                BaseRef:  "main",
            },
        }

        // Execute task
        // ... (actual execution logic)
    }

    // Verify cache metrics show 2 hits (first is miss, 2nd and 3rd are hits)
    metrics := registry.worktreeCache.Metrics()
    require.Equal(t, int64(2), metrics.Hits)
    require.Equal(t, int64(1), metrics.Misses)
}
```

---

## Performance Benchmarks

```go
func BenchmarkWithCache(b *testing.B) {
    config := WorktreeCacheConfig{
        Enabled:  true,
        MaxSize:  100,
        MaxAge:   1 * time.Hour,
        BasePath: b.TempDir(),
    }
    cache, _ := NewWorktreeCache(config)
    defer cache.Shutdown()

    key := WorktreeCacheKey{
        BaseRepo: "test-repo",
        CellPath: "cell",
        BaseRef:  "main",
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        path, release, _ := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
        // Simulate work
        release()
    }
}

func BenchmarkWithoutCache(b *testing.B) {
    config := WorktreeCacheConfig{
        Enabled: false,
    }
    cache, _ := NewWorktreeCache(config)
    defer cache.Shutdown()

    key := WorktreeCacheKey{
        BaseRepo: "test-repo",
        CellPath: "cell",
        BaseRef:  "main",
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        path, release, _ := cache.Acquire(context.Background(), key, &gitstate.GlobalGitTaskContext{})
        // Simulate work
        release()
    }
}
```

---

## Validation Checklist

- [ ] WorktreeCache correctly handles concurrent access to same worktree key
- [ ] Cache key includes BaseRef to prevent branch conflicts between parallel jobs
- [ ] Sequential tasks on same node/branch reuse worktrees (no re-clone)
- [ ] Cache eviction prevents filesystem bloat (respects max_size and max_age)
- [ ] Monitoring shows cache hit rate > 80% for typical workflows with sequential tasks
- [ ] Performance improvement measurable: 30-50% faster for 3-task chains with same repo/cell
- [ ] No deadlocks or race conditions under high concurrency
- [ ] Graceful degradation if cache is full (creates new worktree instead of blocking)
- [ ] Metrics accurately track hits, misses, evictions
- [ ] Shutdown cleanly removes all inactive worktrees
- [ ] Configuration properly enables/disables caching

---

## Migration Path

1. **Deploy with cache disabled (default)**
   - Phase 1 code uses `os.MkdirTemp()` via cache with `Enabled: false`
   - No behavioral change from Phase 1

2. **Enable on canary workers**
   - Set `worktree_cache.enabled: true` for subset of workers
   - Monitor metrics for cache hit rate and errors

3. **Gradual rollout**
   - Increase percentage of workers with caching enabled
   - Tune `max_size` and `max_age` based on metrics

4. **Full deployment**
   - Enable caching on all workers
   - Monitor performance improvement

---

## Expected Performance Impact

### Without Cache (Phase 1)
- 3 sequential tasks: 3 × (clone time + task time)
- Example: 3 × (5s clone + 2s task) = 21s total

### With Cache (Phase 2)
- 3 sequential tasks: 1 × clone + 3 × task time
- Example: 5s clone + 3 × 2s task = 11s total
- **~48% improvement** for this scenario

### Best Case
- Jobs with many sequential tasks on same repo/cell/branch
- Large repositories (expensive to clone)
- Tasks with short execution time

### Worst Case
- Jobs with tasks on different repos/branches
- Very few sequential tasks
- Cache disabled or frequently evicted
