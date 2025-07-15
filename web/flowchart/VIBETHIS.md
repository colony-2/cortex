# web/flowchart

## Purpose
Interactive graph visualization component for rendering dependency relationships between "boxes" as a flowchart. Uses React Flow (xyflow) to provide a draggable, zoomable canvas for visualizing and navigating complex dependency graphs.

## Key Components

### GraphFlow.tsx
Main component that orchestrates the graph visualization:
- Fetches graph data and node positions from backend API
- Renders nodes and edges using React Flow
- Handles node selection and position updates
- Auto-saves node positions after drag operations (500ms debounce)
- Listens for global events to refresh graph when relationships change
- Provides loading states and error handling with troubleshooting tips

### ProFlowNode.tsx
Custom node component for rendering individual boxes:
- Displays box name, type, and relationship count
- Visual feedback for selection state (blue border/shadow)
- Color-coded relationship count badges:
  - Green: 0 relationships
  - Blue: 1-2 relationships
  - Orange: 3-4 relationships
  - Red: 5+ relationships
- Emits global `nodeSelected` events on click

## Graph Visualization Features
- **Automatic Layout**: Initial grid-based positioning for new nodes
- **Persistent Positions**: Node positions saved to backend and restored on reload
- **Interactive Canvas**: Pan, zoom, drag nodes with minimap and controls
- **Real-time Updates**: Refreshes when relationships change via global events
- **Selection Sync**: Bi-directional selection between graph and other UI components

## Backend Integration
Uses shared API module to communicate with server:
- `GET /api/graph` - Fetches complete graph structure
- `GET /api/positions` - Retrieves saved node positions
- `POST /api/positions` - Persists node positions after drag

## UI Patterns
- **Event-Driven Updates**: Uses custom window events for cross-component communication
  - `nodeSelected`: Triggered when node clicked in graph
  - `relationshipsUpdated`: Listened to for refreshing graph data
- **Optimistic UI**: Positions update immediately on drag, saved asynchronously
- **Error Recovery**: Graceful error states with retry options and troubleshooting guidance

## Important Notes
- Node IDs must match between graph data and position data
- Grid fallback positioning ensures new nodes are visible
- Drag-end detection used to minimize position save requests
- Component maintains internal graph reference to avoid unnecessary re-renders
- Selected node state synchronized with parent components via props/callbacks