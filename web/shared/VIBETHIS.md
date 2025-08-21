# web/shared

## Overview
Shared library module for the vibethis frontend, distributed as `@graph-visualizer/shared`. Provides common types, API client functions, real-time input activity management, and URL state utilities used across web application modules.

## Architecture

### Core Data Layer (`types.ts`, `api.ts`)
- **Type System**: Comprehensive TypeScript interfaces for graph data, file system navigation, and form input structures
- **API Client**: Centralized HTTP client with environment-aware configuration and error handling
- **Graph Operations**: Cell/edge management, position persistence, file browsing

### Real-time Input Management (`inputActivityService.ts`, `InputActivityContext.tsx`)
- **Service Layer**: EventSource-based SSE connection with automatic reconnection and caching
- **React Context**: Provider/hook pattern for reactive state management across components
- **Event System**: Custom EventEmitter for decoupled event handling

### URL State Management (`urlState.ts`)
- **Path Parsing**: Extracts application state from URL paths (`/cell/:cellId/:tab/:subtab`)
- **Navigation Helpers**: Pure functions that return navigation paths for React Router integration

## Key Interfaces

### Core Graph Types
```typescript
interface DependencyCell {
  id: string;
  name: string;
  path: string;
  type: string;
  dependencies: string[];
}

interface RelationshipGraph {
  cells: DependencyCell[];
  edges: DependencyEdge[];
}

interface CellPosition {
  cellId: string;
  x: number;
  y: number;
}
```

### Input Activity Management
```typescript
interface InputForm {
  id: string;
  title: string;
  description?: string;
  fields: InputField[];
}

interface PendingInput {
  workflowId: string;
  cellId: string;
  formTitle: string;
  createdAt: string;
  expiresAt: string;
  status: 'pending' | 'completed' | 'expired';
}
```

### API Functions
```typescript
// Graph operations
fetchGraph(): Promise<RelationshipGraph>
fetchPositions(): Promise<CellPosition[]>
savePositions(positions: CellPosition[]): Promise<void>
fetchFiles(cellId: string, path?: string): Promise<{files: any[], path: string}>

// Input activity operations
inputActivityService.getPendingInputs(cellId?: string): Promise<PendingInput[]>
inputActivityService.getInputDetails(workflowId: string): Promise<InputFormDetails>
inputActivityService.submitResponse(workflowId: string, response: FormResponse): Promise<void>
```

### React Hooks
```typescript
// Context-based input activity management
useInputActivity(): {pendingInputsByCellId: Map<string, PendingInput[]>, isConnected: boolean, connectionError: boolean}
useInputActivityForCell(cellId: string): {pendingInputs: PendingInput[], pendingCount: number, urgency: 'pending' | 'urgent' | 'overdue' | null}
```

## Usage Examples

### Basic API Usage
```typescript
import { fetchGraph, savePositions } from '@graph-visualizer/shared';

// Load graph data
const graph = await fetchGraph();

// Save node positions after layout changes
await savePositions([
  { cellId: 'cell1', x: 100, y: 200 },
  { cellId: 'cell2', x: 300, y: 400 }
]);
```

### Input Activity Integration
```typescript
import { InputActivityProvider, useInputActivityForCell } from '@graph-visualizer/shared';

// App-level setup
function App() {
  return (
    <InputActivityProvider>
      <YourComponents />
    </InputActivityProvider>
  );
}

// Component-level usage
function CellComponent({ cellId }: { cellId: string }) {
  const { pendingInputs, pendingCount, urgency } = useInputActivityForCell(cellId);
  
  return (
    <div>
      {pendingCount > 0 && (
        <Badge count={pendingCount} status={urgency === 'overdue' ? 'error' : 'warning'}>
          <CellIcon />
        </Badge>
      )}
    </div>
  );
}
```

### URL State Management
```typescript
import { getURLState, navigateToPath } from '@graph-visualizer/shared';

// Parse current URL state
const { cellId, tab, subtab } = getURLState();

// Generate navigation paths
const newPath = navigateToPath({ cellId: 'cell1', tab: 'files' });
navigate(newPath); // Use with React Router
```

## Configuration

### Environment Variables
- **Development**: API_BASE = `http://localhost:8080/api`
- **Production**: API_BASE = `/api` (relative to current domain)

### Connection Management
- **SSE Reconnection**: Exponential backoff with 3 max attempts
- **Cache Strategy**: Map-based caching for pending inputs and form details
- **Error Handling**: Graceful degradation when SSE endpoints unavailable