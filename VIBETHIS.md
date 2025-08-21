# VibeThis - Dependency Graph Orchestration Platform

## Overview
VibeThis is a monorepo platform for managing and visualizing project dependencies, executing recipes through workflow orchestration, and providing containerized development environments. The system integrates Moon build system for dependency analysis, Temporal for workflow execution, Docker for containerization, and provides comprehensive web interfaces for visualization and control.

## Architecture

### Core Layers
```
┌─────────────────────────────────────────────────────────────┐
│                     Web Frontend Layer                       │
│  ui-app, ui-changes, ui-config, ui-files, ui-flowchart      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      API Gateway Layer                       │
│               be-api, api-openapi, fe-openapi               │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Core Services Layer                       │
│  be-graph, be-files, be-git, be-container, be-storage      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                 Workflow Orchestration Layer                 │
│  recipe-core, recipe-worker, recipe-history, be-activity    │
│              embeddedtemporal, nucleus                       │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Infrastructure Layer                      │
│           cortex, llm, rwshim-clib, rwshim-go              │
└─────────────────────────────────────────────────────────────┘
```

## Projects Summary

### API Layer
- **api/openapi**: OpenAPI 3.0.3 specification defining REST endpoints for graph, file, git, and container operations
- **server/api**: HTTP REST API server with handlers for graph visualization, file management, git operations, and container lifecycle
- **web/openapi**: TypeScript client library generated from OpenAPI spec for type-safe React frontend integration
- **server/openapi**: Go code generation from OpenAPI specifications using oapi-codegen

### Core Services
- **server/core**: Foundational domain types and interfaces (Cell, Edge, Graph, Position, Storage, GraphBuilder)
- **server/graph**: Moon build system integration for dependency discovery and project analysis
- **server/files**: Secure file system operations with type detection and validation for node directories
- **server/git**: Comprehensive Git integration with repository operations and workflow activities
- **server/container**: Docker container orchestration with DevContainer specification support
- **server/storage**: Persistent (BoltDB) and in-memory storage implementations for positions and container mappings

### Workflow Orchestration
- **server/recipe-core**: Unified recipe definitions with YAML parsing, validation, and activity type registry
- **server/recipe-worker**: Temporal-based workflow execution engine with hot-reloading and dynamic recipe discovery
- **server/recipe-history**: Abstraction layer between Temporal workflow history and recipe-core data model
- **server/ops**: Workflow activities for LLM inference, command execution, user input, and recipe invocation
- **server/embeddedtemporal**: Self-contained Temporal server with SQLite backend for development and testing
- **server/nucleus**: CLI wrapper for recipe-worker providing managed workflow execution

### Web Frontend
- **web/app**: Main React application with graph visualization, dynamic tabs, and workflow input management
- **web/flowchart**: Interactive React Flow-based dependency graph visualization with position persistence
- **web/changes**: Git change visualization with status tracking, diff viewing, and commit management
- **web/config**: DevContainer configuration editor with Monaco integration and container lifecycle control
- **web/files**: File browser component with hierarchical navigation and metadata display
- **web/shared**: Common TypeScript library with types, API client, SSE-based input service, and React contexts

### Infrastructure
- **server/cortex**: CLI tool for recipe management with schema generation, validation, and execution commands
- **server/llm**: Multi-provider LLM integration supporting OpenAI, Anthropic, and Gemini with unified interface
- **rwshim/clib**: C-based LD_PRELOAD library for read/write system call interception
- **rwshim/rwshimgo**: Go library for I/O interception control via Unix socket monitoring

### Placeholder Projects
- **server/rucc**: Placeholder module (minimal implementation pending)

## Key Technologies

### Backend
- **Go 1.24**: Primary backend language
- **Temporal.io**: Workflow orchestration engine
- **BoltDB**: Embedded key-value database
- **Docker SDK**: Container management
- **Moon**: Build system and dependency analysis

### Frontend
- **React 18**: UI framework
- **TypeScript**: Type-safe JavaScript
- **Ant Design**: Component library
- **React Flow**: Graph visualization
- **Monaco Editor**: Code editing
- **Vite**: Build tooling

### Infrastructure
- **Unix Sockets**: IPC communication
- **Server-Sent Events**: Real-time updates
- **OpenAPI 3.0**: API specification
- **CEL**: Expression evaluation

## Configuration

### Environment Variables
- `OPENAI_API_KEY`: OpenAI API access
- `ANTHROPIC_API_KEY`: Anthropic Claude access
- `GEMINI_API_KEY`: Google Gemini access
- `VIBETHIS_API_BASE_URL`: API server endpoint

### Build System
```bash
# List all projects
moon query projects

# Build all projects  
moon run :build

# Run tests
moon test
```

### Development
```bash
# Start API server
cd server/api && go run cmd/main.go

# Start web frontend
cd web/app && npm run dev

# Run Cortex CLI
cd server/cortex && go run cmd/cortex/main.go
```

## Project Documentation

Each project contains a `VIBETHIS.md` file with:
1. Overview - Concise functionality description
2. Architecture - Key components and relationships
3. Key Interfaces - Main APIs with signatures
4. Usage Examples - Practical code examples
5. Configuration - Settings and requirements

Refer to individual project directories for detailed documentation.