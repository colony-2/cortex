# Web Flowchart Module

## Overview
React-based graph visualization component using @xyflow/react to render interactive dependency relationships between "cells". Provides draggable, zoomable flowchart interface with persistent positioning and real-time input activity tracking.

## Architecture

### Core Components
- **GraphFlow**: Main orchestrator component managing graph state, API integration, and ReactFlow rendering
- **ProFlowCell**: Custom ReactFlow node component rendering individual cells with visual states
- **InputBadge**: Notification component for pending input alerts with urgency levels

### Key Interfaces
```typescript
interface GraphFlowProps {
  selectedCellId?: string;
  onCellSelect?: (cell: DependencyCell | null) => void;
}

interface ProFlowCellProps {
  data: {
    title: string;
    name: string;
    type: string;
    dependencies: string[];
    selected?: boolean;
    pendingInputCount?: number;
    inputUrgency?: 'pending' | 'urgent' | 'overdue';
  };
  id: string;
}

interface InputBadgeProps {
  count: number;
  status?: 'pending' | 'urgent' | 'overdue';
  cellId: string;
  size?: 'small' | 'default';
}
```

### API Integration
```typescript
// Core graph data fetching
fetchGraph(): Promise<RelationshipGraph>
fetchPositions(): Promise<CellPosition[]>
savePositions(positions: CellPosition[]): Promise<void>

// Data types
interface RelationshipGraph {
  cells: DependencyCell[];
  edges: DependencyEdge[];
}

interface DependencyCell {
  id: string;
  name: string;
  path: string;
  type: string;
  dependencies: string[];
}
```

## Key Interfaces

### Main Component Usage
```typescript
import { GraphFlow } from '@graph-visualizer/flowchart';

<GraphFlow 
  selectedCellId="cell-123"
  onCellSelect={(cell) => console.log('Selected:', cell)}
/>
```

### Event System
```typescript
// Cell selection events
window.dispatchEvent(new CustomEvent('cellSelected', { detail: { cellId: 'cell-123' } }));

// Graph refresh events  
window.addEventListener('relationshipsUpdated', handleGraphRefresh);
```

### Position Management
```typescript
// Auto-save after drag (500ms debounce)
const positions: CellPosition[] = [
  { cellId: 'cell-1', x: 100, y: 200 },
  { cellId: 'cell-2', x: 300, y: 400 }
];
await savePositions(positions);
```

## Usage Examples

### Basic Integration
```typescript
import { GraphFlow } from '@graph-visualizer/flowchart';
import { useState } from 'react';

function App() {
  const [selectedCell, setSelectedCell] = useState<string>();
  
  return (
    <GraphFlow 
      selectedCellId={selectedCell}
      onCellSelect={(cell) => setSelectedCell(cell?.id)}
    />
  );
}
```

### Custom Cell Styling
```typescript
// Dependency count color mapping
const getDependencyColor = (count: number) => {
  if (count === 0) return 'green';
  if (count <= 2) return 'blue'; 
  if (count <= 4) return 'orange';
  return 'red';
};
```

### Input Activity Monitoring
```typescript
// Input urgency calculation
const calculateUrgency = (inputs: any[]) => {
  const now = Date.now();
  const hasOverdue = inputs.some(i => new Date(i.expiresAt).getTime() <= now);
  const hasUrgent = inputs.some(i => {
    const timeRemaining = new Date(i.expiresAt).getTime() - now;
    return timeRemaining <= 5 * 60 * 1000; // 5 minutes
  });
  
  return hasOverdue ? 'overdue' : hasUrgent ? 'urgent' : 'pending';
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
      name: '@vibethis/flowchart',
      formats: ['es', 'cjs']
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@vibethis/shared', '@xyflow/react']
    }
  }
});
```

### Key Dependencies
- `@xyflow/react`: ^12.3.5 - Core flowchart rendering
- `antd`: ^5.26.1 - UI components (Tag, Badge, Space)
- `dagre`: ^0.8.5 - Graph layout algorithms  
- `@vibethis/shared`: Local module for API/types