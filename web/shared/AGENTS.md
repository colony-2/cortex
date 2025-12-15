# web/shared

## Overview
Shared library module for the colony2 frontend, distributed as `@colony2/shared`. Provides common types, API client functions, real-time input activity management, and URL state utilities used across web application modules.

## Architecture

### Core Data Layer (`types.ts`, `api.ts`)
- **Type System**: TypeScript interfaces for graph data and form input structures
- **API Client**: Centralized HTTP client with environment-aware configuration and error handling
- **Graph Operations**: Cell/edge fetching

### Real-time Input Management (`inputActivityService.ts`, `InputActivityContext.tsx`)
- **Service Layer**: EventSource-based SSE connection with automatic reconnection and caching
- **React Context**: Provider/hook pattern for reactive state management across components
- **Event System**: Custom EventEmitter for decoupled event handling

### URL State Management (`urlState.ts`)
- **Path Parsing**: Extracts application state from URL paths (supports optional project prefix: `/project/:projectId/cell/:cellId/:tab/:subtab`)
- **Navigation Helpers**: Pure functions that return navigation paths for React Router integration, including project-aware routes

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
// Project operations
listProjects(): Promise<Project[]>
createProject(input: { name: string; gitRepoPath: string }): Promise<Project>

// Graph operations
fetchGraph(projectId: string): Promise<RelationshipGraph>

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
import { fetchGraph } from '@colony2/shared';

// Load graph data
const graph = await fetchGraph('project-id');
```

### Input Activity Integration
```typescript
import { InputActivityProvider, useInputActivityForCell } from '@colony2/shared';

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
import { getURLState, navigateToPath } from '@colony2/shared';

// Parse current URL state
const { projectId, cellId, tab, subtab } = getURLState();

// Generate navigation paths
const newPath = navigateToPath({ projectId: 'proj1', cellId: 'cell1', tab: 'inputs' });
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
