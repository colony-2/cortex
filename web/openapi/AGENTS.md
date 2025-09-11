# VibeThis OpenAPI Client

## Overview
TypeScript client library auto-generated from VibeThis OpenAPI specification. Provides type-safe API access for React frontend components with support for cell management, file operations, git integration, container orchestration, and graph visualization.

## Architecture

### Code Generation Pipeline
- **Source**: `/api/openapi/vibethis-api.yaml` OpenAPI 3.0 specification  
- **Generator**: openapi-typescript-codegen with axios HTTP client
- **Output**: TypeScript interfaces, service classes, and error handling
- **Build**: tsup bundler producing CJS, ESM, and TypeScript definitions

### Service Layer Architecture
Five core service classes provide typed API methods:
- **ContainerService**: DevContainer lifecycle management (create, start, stop, restart, reset)
- **FilesService**: Cell directory file operations (list, read, write)  
- **GitService**: Version control operations (status, diff, history, commit)
- **GraphService**: Dependency graph retrieval and visualization
- **PositionsService**: UI layout persistence (save/restore cell positions)

### Type System
Generated TypeScript interfaces ensure compile-time safety:
- **Cell**: Core entity with id, name, path, type, and dependencies array
- **Graph**: Contains cells array and edges array for dependency visualization
- **FileInfo**: File metadata with name, size, type, and modification timestamps
- **GitStatus/GitCommit**: Version control state and history representations
- **Position**: UI layout coordinates for graph visualization persistence

## Key Interfaces

### Core Service Methods

```typescript
// Graph operations
GraphService.getGraph(): Promise<Graph>

// File operations
FilesService.listCellFiles(cellId: string, path?: string): Promise<{files: FileInfo[], path: string}>
FilesService.readCellFile(cellId: string, filePath: string): Promise<string>
FilesService.writeCellFile(cellId: string, filePath: string, content: {content: string}): Promise<any>

// Git operations  
GitService.getGitStatus(cellId: string): Promise<GitStatus>
GitService.getGitDiff(cellId: string, staged?: boolean): Promise<string>
GitService.getGitHistory(cellId: string, limit?: number): Promise<GitCommit[]>
GitService.createGitCommit(cellId: string, {message: string, files?: string[]}): Promise<any>

// Container management
ContainerService.getContainerStatus(cellId: string): Promise<{status: ContainerStatus, containerId?: string, hasDevcontainer?: boolean}>
ContainerService.createContainer(cellId: string): Promise<{containerId: string}>
ContainerService.startContainer(cellId: string): Promise<any>
ContainerService.stopContainer(cellId: string): Promise<any>
ContainerService.updateDevcontainer(cellId: string, {content: string}): Promise<any>

// Position persistence
PositionsService.getPositions(): Promise<Position[]>
PositionsService.savePositions(positions: Position[]): Promise<any>
```

### Core Type Definitions

```typescript
interface Cell {
  id: string;
  name: string;
  path: string;
  type: string;
  dependencies: string[];
}

interface Graph {
  cells: Cell[];
  edges: Edge[];
}

interface FileInfo {
  name: string;
  size: number;
  type: string;
  modifiedAt: string;
}

interface GitStatus {
  branch: string;
  ahead: number;
  behind: number;
  files: GitStatusFile[];
}

interface Position {
  cellId: string;
  x: number;
  y: number;
}
```

## Usage Examples

### Basic API Configuration
```typescript
import { OpenAPI, GraphService } from '@vibethis/openapi-client';

// Configure base URL (defaults to http://localhost:8080)
OpenAPI.BASE = process.env.REACT_APP_API_URL || 'http://localhost:8080';

// Fetch complete dependency graph
const graph = await GraphService.getGraph();
```

### File Operations
```typescript
import { FilesService } from '@vibethis/openapi-client';

// List files in cell directory
const {files} = await FilesService.listCellFiles('my-cell-id', 'src/');

// Read file content
const content = await FilesService.readCellFile('my-cell-id', 'package.json');

// Write file content  
await FilesService.writeCellFile('my-cell-id', 'README.md', {
  content: '# My Project\n\nProject description here.'
});
```

### Git Integration
```typescript
import { GitService } from '@vibethis/openapi-client';

// Get current git status
const status = await GitService.getGitStatus('my-cell-id');

// View uncommitted changes
const diff = await GitService.getGitDiff('my-cell-id');

// Create commit with staged files
await GitService.createGitCommit('my-cell-id', {
  message: 'feat: add new component',
  files: ['src/NewComponent.tsx', 'src/index.ts']
});
```

### Container Management
```typescript
import { ContainerService } from '@vibethis/openapi-client';

// Check container status
const {status, hasDevcontainer} = await ContainerService.getContainerStatus('my-cell-id');

// Create and start container if devcontainer.json exists
if (hasDevcontainer && status === 'stopped') {
  await ContainerService.createContainer('my-cell-id');
  await ContainerService.startContainer('my-cell-id');
}
```

### Error Handling
```typescript
import { ApiError } from '@vibethis/openapi-client';

try {
  const files = await FilesService.listCellFiles('invalid-cell-id');
} catch (error) {
  if (error instanceof ApiError) {
    console.error(`API Error ${error.status}: ${error.message}`);
  }
}
```

## Configuration

### Build Scripts
```json
{
  "generate": "openapi-typescript-codegen --input ../../api/openapi/vibethis-api.yaml --output ./src/generated --client axios",
  "build": "tsup src/index.ts --format cjs,esm --dts --clean",
  "clean": "rm -rf src/generated dist"
}
```

### Runtime Configuration
```typescript
import { OpenAPI } from '@vibethis/openapi-client';

// Configure authentication
OpenAPI.TOKEN = 'bearer-token';

// Configure request headers
OpenAPI.HEADERS = {
  'X-Custom-Header': 'value'
};

// Configure credentials
OpenAPI.WITH_CREDENTIALS = true;
OpenAPI.CREDENTIALS = 'include';
```

### Integration with Moon Build System
Depends on `api-openapi:build` task to ensure OpenAPI specification is current before client generation. Generated code in `src/generated/` should never be manually edited.