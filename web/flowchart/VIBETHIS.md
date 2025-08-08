# web/flowchart

## Purpose
Interactive graph visualization component for rendering dependency relationships between "cells" as a flowchart. Uses React Flow (xyflow) to provide a draggable, zoomable canvas for visualizing and navigating complex dependency graphs.

## Key Components

### GraphFlow.tsx
Main component that orchestrates the graph visualization:
- Fetches graph data and cell positions from backend API
- Renders cells and edges using React Flow
- Handles cell selection and position updates
- Auto-saves cell positions after drag operations (500ms debounce)
- Listens for global events to refresh graph when relationships change
- Provides loading states and error handling with troubleshooting tips

### ProFlowCell.tsx
Custom cell component for rendering individual cells:
- Displays cell name, type, and relationship count
- Visual feedback for selection state (blue border/shadow)
- Color-coded relationship count badges:
  - Green: 0 relationships
  - Blue: 1-2 relationships
  - Orange: 3-4 relationships
  - Red: 5+ relationships
- Emits global `cellSelected` events on click

## Graph Visualization Features
- **Automatic Layout**: Initial grid-based positioning for new cells
- **Persistent Positions**: Cell positions saved to backend and restored on reload
- **Interactive Canvas**: Pan, zoom, drag cells with minimap and controls
- **Real-time Updates**: Refreshes when relationships change via global events
- **Selection Sync**: Bi-directional selection between graph and other UI components

## Backend Integration
Uses shared API module to communicate with server:
- `GET /api/graph` - Fetches complete graph structure
- `GET /api/positions` - Retrieves saved cell positions
- `POST /api/positions` - Persists cell positions after drag

## UI Patterns
- **Event-Driven Updates**: Uses custom window events for cross-component communication
  - `cellSelected`: Triggered when cell clicked in graph
  - `relationshipsUpdated`: Listened to for refreshing graph data
- **Optimistic UI**: Positions update immediately on drag, saved asynchronously
- **Error Recovery**: Graceful error states with retry options and troubleshooting guidance

## Important Notes
- Cell IDs must match between graph data and position data
- Grid fallback positioning ensures new cells are visible
- Drag-end detection used to minimize position save requests
- Component maintains internal graph reference to avoid unnecessary re-renders
- Selected cell state synchronized with parent components via props/callbacks