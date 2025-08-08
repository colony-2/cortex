# VIBETHIS Project Overview

VibeThis is a comprehensive system for building, managing, and orchestrating modular components called "cells". Each cell is an independent unit that can have dependencies, be version controlled, and run in isolated development containers. This document provides a guide to all subprojects in the vibethis ecosystem.

## System Architecture

The system consists of:
- **Go Backend Server**: RESTful API serving graph data, file operations, Git integration, and container management
- **React Frontend**: Interactive UI for visualizing dependencies and managing cells
- **Recipe Orchestration**: YAML-based recipe system with LLM integration (Ono)
- **I/O Interception**: Process monitoring and control via LD_PRELOAD shims (rwshim)

## Core Components

### Backend Services (Go)

#### server/core
**Purpose**: Foundational domain models and interfaces  
**Key Features**:
- Defines core types: Cell, Edge, Graph, Position
- Storage and GraphBuilder interfaces
- Pure interface pattern with no implementations
- Module boundary for all backend services

#### server/api
**Purpose**: HTTP REST API server  
**Key Features**:
- Graph management endpoints
- File operations per cell
- Git operations per cell  
- Container lifecycle management
- Single Page Application (SPA) hosting

#### server/storage
**Purpose**: Persistence layer implementations  
**Key Features**:
- BoltDB for production (file-based)
- Memory storage for testing
- Stores cell positions and container IDs
- Thread-safe implementations

#### server/graph
**Purpose**: Dependency graph construction  
**Key Features**:
- Integrates with Moon build system
- Discovers cells and relationships
- Builds complete dependency graphs
- Supports symlink resolution

#### server/files
**Purpose**: Secure file system operations  
**Key Features**:
- Directory browsing within cell boundaries
- Path validation and security
- File type detection
- Configurable size limits

#### server/git
**Purpose**: Git version control integration  
**Key Features**:
- Read-only Git operations
- Status tracking and diff generation
- Commit history and file staging
- No automatic repository initialization

#### server/container
**Purpose**: Development container orchestration  
**Key Features**:
- Full container lifecycle management
- Devcontainer.json parsing and support
- Multi-platform Docker integration
- WebSocket terminal attachment

#### server/openapi
**Purpose**: OpenAPI code generation  
**Key Features**:
- Generates Go types from OpenAPI spec
- Type-safe HTTP client
- Embedded spec validation

### Frontend Components (React/TypeScript)

#### web/app
**Purpose**: Main React application  
**Key Features**:
- Routes and orchestrates all UI components
- Split-panel interface with graph and details
- URL-based state management
- Deep linking support

#### web/flowchart
**Purpose**: Interactive dependency graph visualization  
**Key Features**:
- React Flow-based graph rendering
- Draggable cells with position persistence
- Visual relationship indicators
- Real-time updates via events

#### web/files
**Purpose**: File browser component  
**Key Features**:
- Hierarchical file exploration
- Breadcrumb navigation
- File type indicators
- Integration with backend file API

#### web/config
**Purpose**: Configuration management UI  
**Key Features**:
- Devcontainer.json editing (GUI and Monaco)
- Container lifecycle controls
- JSON Schema validation
- Real-time status monitoring

#### web/changes
**Purpose**: Git change tracking component  
**Key Features**:
- Git status visualization
- Diff display with syntax highlighting
- Commit history view
- Direct commit functionality

#### web/shared
**Purpose**: Shared utilities and types  
**Key Features**:
- Common TypeScript types
- API client functions
- URL state management utilities

#### web/openapi
**Purpose**: TypeScript API client generation  
**Key Features**:
- Auto-generated from OpenAPI spec
- Type-safe service classes
- Axios-based HTTP client

### Workflow Orchestration (Ono)

#### server/ono
**Purpose**: YAML-based workflow orchestration with LLM integration  
**Key Features**:
- Recipe-based workflow definitions
- Native LLM support (OpenAI, Anthropic, Gemini)
- Structured output schemas
- Built on Temporal for reliability
- Automatic retries and error handling

#### server/recipe-core
**Purpose**: Core recipe data structures and parsing  
**Key Features**:
- Recipe schema definitions
- YAML parsing for recipes, ops, agents
- Multi-file and single-file recipe support
- Content hashing for change detection

#### server/recipe-worker
**Purpose**: Runtime engine for recipe execution  
**Key Features**:
- Dynamic recipe discovery and execution
- Live-reloading of recipe changes
- Temporal worker management
- Op execution dispatch

#### server/recipe-history
**Purpose**: Recipe execution history adapter  
**Key Features**:
- Transforms Temporal history to recipe model
- Job listing and filtering
- Op execution reconstruction
- Clean API abstraction

#### server/embeddedtemporal
**Purpose**: Self-contained Temporal server  
**Key Features**:
- Embedded Temporal with SQLite
- Zero-configuration startup
- Dynamic port allocation
- Automatic schema management

### Process Monitoring (rwshim)

#### rwshim/clib
**Purpose**: C-based I/O interception library  
**Key Features**:
- LD_PRELOAD/DYLD_INSERT_LIBRARIES support
- Read/write system call interception
- Unix socket communication protocol
- Policy-based I/O control

#### rwshim/rwshimgo
**Purpose**: Go library for process I/O control  
**Key Features**:
- High-level API for monitor creation
- Policy builder pattern
- Process launching with shim injection
- Cross-platform support (Linux/macOS)

### API Specifications

#### api/openapi
**Purpose**: OpenAPI 3.0.3 REST API specification  
**Key Features**:
- Complete API documentation
- Request/response schemas
- Five main endpoint categories
- Source of truth for API contract

## Key Design Patterns

1. **Module Independence**: Each module has clear boundaries with interface-based communication
2. **Code Generation**: OpenAPI drives both backend and frontend type safety
3. **Event-Driven Updates**: Frontend components communicate via custom window events
4. **URL State Management**: Deep linking and navigation state in URLs
5. **Dynamic Discovery**: Recipes and cells discovered from filesystem
6. **Security by Design**: Path validation and boundary enforcement

## Development Workflow

1. **Moon Build System**: Orchestrates builds across all modules
2. **Local Development**: Frontend on port 5173, backend on port 8080
3. **Testing**: Unit tests, integration tests, and E2E tests with Playwright
4. **Production Build**: Single Go binary with embedded React frontend

## Integration Points

- Backend modules use local Go module replace directives
- Frontend modules published as npm packages
- API contract shared via OpenAPI specification
- Git repositories serve as cell containers
- Docker provides isolated execution environments

## Future Placeholders

- **server/tempcrew**: Intended for temporary worker/crew functionality
- **server/cortex**: Currently minimal, likely for AI/ML integration

This modular architecture enables flexible development, clear separation of concerns, and easy extension of functionality while maintaining a cohesive system for managing and orchestrating cells.