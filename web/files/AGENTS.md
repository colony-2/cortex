# web/files

## Overview
React component library for file system browsing within Vibethis boxes. Provides hierarchical navigation, file metadata display, and integration with the backend file API for exploring box directory structures.

## Architecture

### Core Components
- **FileBrowser**: Main component providing file exploration interface
- **FileItem Interface**: Type definition for file/folder objects
- **FileBrowserProps**: Component configuration interface

### Key Relationships
- Depends on `@vibethis/shared` for API communication (`fetchFiles`)
- Uses Ant Design components for UI (List, Breadcrumb, Card, Spin, Empty, Tag)
- Integrates with backend via REST API at `/api/cells/{cellId}/files`
- Built as Vite library module for consumption across application

### Data Flow
1. Component receives `cell` object or `cellId` parameter
2. Fetches file data from API using current path
3. Transforms backend response to UI-friendly format
4. Renders interactive file list with navigation

## Key Interfaces

### FileBrowserProps
```typescript
interface FileBrowserProps {
  cell: DependencyCell | null;
  cellId?: string;
}
```

### FileItem
```typescript
interface FileItem {
  id: string;
  name: string;
  type: 'file' | 'folder';
  size: number;
  date: Date;
  ext: string;
  path: string;
  isDir: boolean;
}
```

### DependencyCell (from shared)
```typescript
interface DependencyCell {
  id: string;
  name: string;
  path: string;
  type: string;
  dependencies: string[];
}
```

### API Function
```typescript
fetchFiles(cellId: string, path: string = ''): Promise<{ files: any[], path: string }>
```

### Utility Functions
```typescript
formatFileSize(bytes: number): string
getFileIcon(file: FileItem): JSX.Element
```

## Usage Examples

### Basic Implementation
```typescript
import { FileBrowser } from '@vibethis/files';
import type { DependencyCell } from '@vibethis/shared';

// With cell object
const cell: DependencyCell = {
  id: 'box-123',
  name: 'My Project',
  path: '/path/to/project',
  type: 'project',
  dependencies: []
};

<FileBrowser cell={cell} />
```

### Direct Cell ID Usage
```typescript
// When only cell ID is available
<FileBrowser cellId="box-123" />
```

### Integration in Parent Component
```typescript
function ProjectExplorer({ selectedCell }: { selectedCell: DependencyCell | null }) {
  return (
    <div style={{ height: '100vh' }}>
      <FileBrowser cell={selectedCell} />
    </div>
  );
}
```

### Error Handling Pattern
```typescript
// Component handles errors internally with user-friendly messages
// No external error handling required
<FileBrowser 
  cell={cell} 
  // Component displays loading states and error messages automatically
/>
```

## Configuration

### Build Configuration (vite.config.ts)
```typescript
{
  build: {
    lib: {
      entry: 'src/index.ts',
      name: '@vibethis/files',
      formats: ['es', 'cjs']
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@vibethis/shared']
    }
  }
}
```

### Dependencies
- React 18.3+
- Ant Design 5.26+
- @vibethis/shared (internal package)
- @ant-design/icons for file/folder icons

### API Endpoints
- `GET /api/cells/{cellId}/files?path={path}` - Fetch files for given path
- Returns: `{ files: FileItem[], path: string }`

### Environment Variables
- Uses `import.meta.env.DEV` for API base URL determination
- Development: `http://localhost:8080/api`
- Production: `/api` (relative)