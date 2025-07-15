# Storage Module

## Purpose
The storage module provides persistence layer implementations for the vibethis system, specifically managing the storage of box positions and container IDs. It offers pluggable storage backends with a common interface defined by `core.Storage`.

## Storage Implementations

### BoltDB (`internal/bolt`)
- **Primary production storage**: Uses BoltDB, an embedded key-value database
- **File location**: Creates `.vibethis.db` file in specified directory path
- **Features**:
  - ACID transactions
  - Read-only mode support
  - Thread-safe with mutex protection
  - Two buckets: `positions` and `container_ids`
- **Data format**: JSON serialization for positions

### Memory (`internal/memory`)
- **Testing and development storage**: Pure in-memory implementation
- **Features**:
  - Thread-safe with RWMutex
  - Zero persistence (data lost on restart)
  - No external dependencies
  - Primarily for unit tests and local development

## Data Model

### Positions
- **Purpose**: Stores UI positions of boxes/nodes
- **Structure**: `core.Position` with NodeID, X, Y coordinates
- **Operations**: Save, GetAll, Delete by NodeID

### Container IDs
- **Purpose**: Maps node IDs to container IDs
- **Structure**: Simple key-value mapping (nodeID -> containerID)
- **Operations**: Save, Get, Delete by NodeID

## Integration Points

- **Core dependency**: Implements `core.Storage` interface
- **Factory functions**: 
  - `NewBoltStorage(config)` - Creates persistent storage
  - `NewMemoryStorage()` - Creates in-memory storage
- **Configuration**: Via `Config` struct with `DatabasePath` and `ReadOnly` options

## Performance Considerations

- **BoltDB**: File-based with memory-mapped files, suitable for moderate workloads
- **Locking strategy**: RWMutex for concurrent read access
- **Transaction batching**: Single operations wrapped in BoltDB transactions
- **No connection pooling**: Single DB handle per instance

## Usage Notes

- Always call `Close()` to properly shutdown BoltDB
- Read-only mode prevents all write operations (useful for debugging/inspection)
- BoltDB file grows but doesn't shrink automatically
- Memory storage ideal for integration tests requiring clean state