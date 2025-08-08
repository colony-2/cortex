# server/core

## Directory Purpose

The `server/core` directory defines the foundational domain models and interfaces that form the backbone of the vibethis system. It serves as the shared contract layer between all server modules, ensuring consistent data structures and behavioral interfaces across the entire backend architecture.

## Key Domain Models

### Graph Data Structures
The core module defines the fundamental graph-based data model used throughout vibethis:

- **Cell**: Represents entities in the dependency graph with ID, name, path, type, and dependencies
- **Edge**: Defines directed relationships between cells (source → target)
- **Graph**: Aggregates cells and edges into a complete dependency structure
- **Position**: Tracks visual positioning of cells in the UI (x, y coordinates)

These types are JSON-serializable and form the primary data exchange format between backend and frontend.

## Core Interfaces

### Storage Interface
Defines persistent storage operations contract:
- Position management (save, get, delete cell positions)
- Container ID mapping (associate cells with Docker containers)
- Lifecycle management (close/cleanup)

### GraphBuilder Interface
Defines graph construction contract:
- Build complete dependency graphs from filesystem
- Retrieve individual cells by ID

## Integration Patterns

The core module follows a pure interface pattern:
- **No implementations**: Only type definitions and interfaces
- **Module boundary**: All other modules depend on core, but core depends on nothing
- **Replace directives**: Other modules use local replace directives in go.mod to reference core

### Consumer Modules
- **storage**: Implements the Storage interface with BoltDB and memory backends
- **graph**: Implements the GraphBuilder interface for filesystem analysis
- **api**: Uses core types for HTTP request/response models
- **container**: Uses cell IDs from core for container management

## Design Decisions

1. **Interface Segregation**: Storage and GraphBuilder are separate interfaces to allow independent evolution
2. **Context-First**: All interface methods accept context.Context for cancellation and timeout support
3. **Error Handling**: All operations return explicit errors rather than panics
4. **JSON Tags**: All types include JSON tags for direct API serialization
5. **ID-Based References**: Relationships use string IDs rather than object pointers for flexibility

## Module Configuration

- **Module ID**: `be-core` (moon.yml)
- **Go Module**: `vibethis/core`
- **Type**: Library module (no executable)
- **Go Version**: 1.21

## Important Notes

- This module must remain dependency-free to prevent circular dependencies
- All changes to interfaces are breaking changes for consuming modules
- Types should remain simple and serializable for API compatibility
- The module serves as the "vocabulary" for inter-module communication