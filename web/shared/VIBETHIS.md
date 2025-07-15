# web/shared

## Purpose
Shared library module for the vibethis frontend, distributed as `@graph-visualizer/shared`. Provides common types, API client functions, and utilities used across web application modules.

## Core Components

### Type Definitions (`types.ts`)
- **DependencyNode**: Core node representation with id, name, path, type, and dependencies array
- **DependencyEdge**: Graph edge with source/target relationship
- **NodePosition**: X/Y coordinates for graph node positioning
- **RelationshipGraph/DependencyGraph**: Complete graph structure with nodes and edges
- **FileItem**: File browser representation with name, path, isDir, size, and type

### API Client (`api.ts`)
Centralized API communication layer:
- **fetchGraph()**: Retrieves complete dependency graph data
- **fetchPositions()**: Gets saved node positions for graph layout
- **savePositions()**: Persists node position updates
- **fetchFiles()**: Browses files within a specific node/box

Features:
- Environment-aware base URL (localhost:8080 in dev, relative /api in prod)
- Comprehensive error handling with detailed messages
- Consistent response validation

### URL State Management (`urlState.ts`)
Path-based routing utilities for maintaining application state in URL:
- Parses paths like `/box/:boxId/:tab/:subtab`
- Provides navigation helpers that return path strings
- Supports box selection, tab navigation, and subtab states
- Designed to work with React Router's navigation hooks

## Build Configuration
- Vite library build targeting ES2020
- Generates both ES and CommonJS modules
- TypeScript declarations included
- External dependencies: react, react-dom, antd, @ant-design/icons

## Integration Patterns
- Import as `@graph-visualizer/shared` in other web modules
- All exports available through main index.ts barrel file
- Stateless utilities - no React components or hooks
- Type-first design for strong typing across modules

## Notes
- Currently minimal test coverage (placeholder test file)
- API client assumes backend running on port 8080 during development
- URL state utilities return paths for navigation, not direct history manipulation