# web/app Directory

## Purpose
Main React application orchestrating the vibethis graph visualization interface. This is the primary UI entry point that integrates all visualization and configuration modules.

## Architecture Overview

### Core Application Structure
- **Framework**: React 18 with TypeScript
- **Build Tool**: Vite
- **Router**: React Router v6 for SPA navigation
- **UI Library**: Ant Design (antd)
- **Testing**: Playwright for E2E tests

### Main Components

#### App.tsx
- Root component handling routing configuration
- Default route redirects to `/boxes`
- Nested routing structure:
  - `/boxes` - Main boxes view
  - `/box/:boxId` - Box detail view
  - `/box/:boxId/:tab` - Tab navigation within box
  - `/box/:boxId/:tab/:subtab` - Nested tab navigation

#### MainView.tsx
- Primary layout component using Ant Design Splitter
- Left panel: GraphFlow visualization from `@vibethis/flowchart`
- Right panel: SidePanel with contextual information
- Handles node selection and URL navigation synchronization

#### SidePanel.tsx
- Dynamic panel showing different content based on selection:
  - **When no box selected**: Global configuration tabs
  - **When box selected**: Box-specific tabs (Files, Config, Changes)
- Integrates multiple modules:
  - `@vibethis/files` - FileBrowser component
  - `@vibethis/config` - EnvEditor component
  - `@vibethis/changes` - GitChanges component
- Nested tab structure for configuration options

## State Management
- URL-based state using React Router params
- Component-level state with useState hooks
- Navigation state synchronized with URL for deep linking
- No global state management library (Redux/MobX)

## Module Integration
The app serves as the orchestration layer for visualization modules:
- **@vibethis/flowchart**: Graph visualization component
- **@vibethis/files**: File browser for box contents
- **@vibethis/config**: Environment configuration editor
- **@vibethis/changes**: Git change tracking
- **@vibethis/shared**: Common types and utilities

## Testing Strategy
- E2E tests using Playwright covering:
  - UI integration flows
  - Tab navigation persistence
  - Node selection and file browser updates
  - URL state management
  - Position persistence
  - Git changes visualization

## Development Commands
- `moon run ui-app:serve` - Start development server on port 5173
- `moon run ui-app:e2e` - Run Playwright E2E tests
- Build output goes to `dist/` directory

## Key Features
1. Split-panel interface with resizable panels
2. Deep linking support for all views
3. Contextual side panel adapting to selection
4. Integration with backend API for graph data
5. Responsive navigation with URL synchronization