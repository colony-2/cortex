# Storage Module

## Overview

The storage module provides persistent and in-memory storage implementations for the vibethis system. It manages cell positions and container ID mappings with support for both BoltDB-backed persistence and memory-based storage for testing.

## Architecture

The module implements a factory pattern with two concrete storage backends:

- **BoltDB Storage** (`internal/bolt`): Persistent storage using embedded BoltDB with thread-safe operations and read-only mode support
- **Memory Storage** (`internal/memory`): In-memory storage using Go maps with thread-safe operations for testing

Both implementations satisfy the `core.Storage` interface and provide identical APIs for position and container ID management.

## Key Interfaces

### Storage Interface
```go
type Storage interface {
    // Position operations
    SavePosition(ctx context.Context, pos Position) error
    GetPositions(ctx context.Context) ([]Position, error)  
    DeletePosition(ctx context.Context, cellID string) error
    
    // Container ID operations
    SaveContainerID(ctx context.Context, cellID, containerID string) error
    GetContainerID(ctx context.Context, cellID string) (string, error)
    DeleteContainerID(ctx context.Context, cellID string) error
    
    Close() error
}
```

### Configuration
```go
type Config struct {
    DatabasePath string // Path to database file (BoltDB only)
    ReadOnly     bool   // Read-only mode flag
}
```

### Position Type
```go
type Position struct {
    CellID string  `json:"cellId"`
    X      float64 `json:"x"`
    Y      float64 `json:"y"`
}
```

### Factory Functions
```go
func NewBoltStorage(config Config) (core.Storage, error)
func NewMemoryStorage() core.Storage
```

## Usage Examples

### BoltDB Storage
```go
import "github.com/divisive-ai/vibethis/server/storage"

// Create persistent storage
config := storage.Config{
    DatabasePath: "/path/to/data",
    ReadOnly:     false,
}
store, err := storage.NewBoltStorage(config)
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Save position
pos := core.Position{CellID: "cell1", X: 100.5, Y: 200.5}
err = store.SavePosition(ctx, pos)

// Get all positions
positions, err := store.GetPositions(ctx)

// Save container mapping
err = store.SaveContainerID(ctx, "cell1", "container123")

// Retrieve container ID
containerID, err := store.GetContainerID(ctx, "cell1")
```

### Memory Storage
```go
// Create in-memory storage for testing
store := storage.NewMemoryStorage()

// Same API as BoltDB storage
pos := core.Position{CellID: "test", X: 50.0, Y: 75.0}
err := store.SavePosition(ctx, pos)
```

### Read-Only Mode
```go
// Open in read-only mode
config := storage.Config{
    DatabasePath: "/path/to/data",
    ReadOnly:     true,
}
store, err := storage.NewBoltStorage(config)

// Read operations work normally
positions, err := store.GetPositions(ctx)

// Write operations return errors
err = store.SavePosition(ctx, pos) // Returns "storage is read-only" error
```

## Configuration

### BoltDB Configuration
- `DatabasePath`: Directory where `.vibethis.db` file will be created
- `ReadOnly`: Set to `true` to prevent write operations
- Database uses two buckets: `positions` and `container_ids`
- File permissions: 0600 (owner read/write only)

### Dependencies
- `github.com/boltdb/bolt`: Embedded key-value database
- `github.com/divisive-ai/vibethis/server/core`: Core types and interfaces

### Error Handling
- Thread-safe operations with mutex protection
- Proper resource cleanup with Close() method
- Detailed error messages with context
- Read-only mode validation for write operations