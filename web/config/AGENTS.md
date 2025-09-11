# Web Config Module

## Overview
React component library for devcontainer configuration management in the VibThis dependency graph visualization system. Provides CRUD operations for devcontainer.json files and Docker container lifecycle management.

## Architecture
- **EnvEditor**: Primary React component handling devcontainer configuration and container operations
- **Monaco Editor Integration**: JSON editing with schema validation for devcontainer.json
- **API Layer**: RESTful endpoints for file operations and container management
- **State Management**: React hooks managing edit modes, container status, and content synchronization

## Key Interfaces

### EnvEditor Component
```typescript
interface EnvEditorProps {
  cell: DependencyCell;
}

interface DependencyCell {
  id: string;
  name: string;
  path: string;
  type: string;
  dependencies: string[];
}
```

### API Endpoints
```typescript
// Container Management
GET    /api/cells/{cellId}/container/status
POST   /api/cells/{cellId}/container/create
POST   /api/cells/{cellId}/container/start
POST   /api/cells/{cellId}/container/restart
POST   /api/cells/{cellId}/container/reset

// Configuration Management
PUT    /api/cells/{cellId}/container/devcontainer
```

### Component Methods
```typescript
loadContainerStatus(): Promise<void>
createDevcontainerFile(): void
saveDevcontainerFile(): Promise<void>
createContainer(): Promise<void>
startContainer(): Promise<void>
restartContainer(): Promise<void>
resetContainer(): Promise<void>
```

## Usage Examples

### Basic Integration
```tsx
import { EnvEditor } from '@graph-visualizer/config';

function SidePanel({ selectedCell }: { selectedCell: DependencyCell }) {
  return (
    <div>
      <EnvEditor cell={selectedCell} />
    </div>
  );
}
```

### Default Devcontainer Configuration
```json
{
  "name": "Dev Container",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {},
  "customizations": {
    "vscode": {
      "extensions": []
    }
  },
  "forwardPorts": [],
  "postCreateCommand": ""
}
```

### Error Handling Pattern
```typescript
const showError = (title: string, error: any) => {
  Modal.error({
    title: title,
    content: extractErrorMessage(error),
    width: 600
  });
};
```

## Configuration

### Build Configuration (vite.config.ts)
```typescript
export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: 'src/index.ts',
      name: '@vibethis/config',
      formats: ['es', 'cjs']
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@vibethis/shared']
    }
  }
});
```

### Dependencies
- Core: React 18, TypeScript, Ant Design 5.26+
- Editor: Monaco Editor React 4.7+
- Validation: Official devcontainer JSON schema
- Build: Vite 5.4+ with library mode