# web/openapi

## Purpose
This directory contains the auto-generated TypeScript client for the VibeThis API. It provides type-safe API access for the React frontend, generated from the OpenAPI specification located at `api/openapi/vibethis-api.yaml`.

## Key Components

### Generated Client Library
- **Package**: `@vibethis/openapi-client` 
- **Generator**: openapi-typescript-codegen with axios client
- **Build Tool**: tsup for bundling (outputs CJS, ESM, and TypeScript definitions)

### Service Classes
Generated service classes provide typed API methods:
- `ContainerService` - Container management operations
- `FilesService` - File operations within node directories
- `GitService` - Git operations for nodes
- `GraphService` - Node dependency graph operations
- `PositionsService` - Position/layout management

### Type Definitions
Generated TypeScript interfaces for API models:
- `Node` - Core node entity with id, name, path, type, and dependencies
- `FileInfo` - File metadata
- `GitStatus`, `GitCommit`, `GitStatusFile` - Git-related types
- `Graph`, `Edge` - Dependency graph structures
- `Position` - Layout positioning data

## Integration Pattern

1. **Code Generation**: Run via moon task `fe-openapi:build` which:
   - Cleans previous generated code
   - Generates new client from OpenAPI spec
   - Builds distribution bundles

2. **API Configuration**: The `OpenAPI` object in `core/OpenAPI.ts` can be configured at runtime:
   ```typescript
   import { OpenAPI } from '@vibethis/openapi-client';
   OpenAPI.BASE = 'http://localhost:8080'; // Default base URL
   ```

3. **Usage in React Components**:
   ```typescript
   import { FilesService, type Node } from '@vibethis/openapi-client';
   
   const files = await FilesService.listNodeFiles(nodeId);
   ```

## Build Dependencies
- Depends on `api-openapi:build` task to ensure OpenAPI spec is up-to-date
- Outputs to both `src/generated/` (source) and `dist/` (bundled)

## Notes
- All generated code is in `src/generated/` and should not be manually edited
- The client uses axios for HTTP requests with built-in error handling
- Supports cancelable promises for request management
- Base URL defaults to `http://localhost:8080` but should be configured for production