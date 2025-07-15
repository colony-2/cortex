# web/changes

## Purpose
A React component library that provides Git change tracking and visualization functionality for vibethis boxes. This directory contains a reusable component that integrates with the backend API to display git status, diffs, and commit history for individual boxes/nodes in the system.

## Key Features
- **Git Status Display**: Shows added, modified, deleted, and untracked files with visual badges and icons
- **Diff Visualization**: Displays detailed file changes with additions/deletions in a formatted view
- **Commit History**: Shows commit log with author information and timestamps
- **Commit Functionality**: Allows users to commit all changes directly from the UI

## Main Component
### GitChanges Component
- **Location**: `src/GitChanges.tsx`
- **Props**:
  - `node`: The DependencyNode object representing the current box
  - `activeTab`: Controls which tab is active (summary/details/history)
  - `onTabChange`: Callback for tab changes
- **Features**:
  - Three-tab interface: Summary, Details, and History
  - Real-time status fetching based on active tab
  - Commit button with loading states
  - Empty states for non-git directories

## API Integration
The component communicates with the backend through REST endpoints:
- `GET /api/nodes/{nodeId}/git/status` - Fetches git status
- `GET /api/nodes/{nodeId}/git/diff` - Retrieves file diffs
- `GET /api/nodes/{nodeId}/git/history` - Gets commit history
- `POST /api/nodes/{nodeId}/git/commit` - Creates new commits

## UI Patterns
- **Status Visualization**: Color-coded badges and icons for different file states
  - Green (Added/Untracked): FileAddOutlined icon
  - Blue (Modified): EditOutlined icon
  - Red (Deleted): DeleteOutlined icon
- **Diff Display**: Pre-formatted patches with syntax highlighting
- **Loading States**: Spinner components during data fetching
- **Error Handling**: Empty states for non-git repositories or when no changes exist

## Technical Details
- Built as a library component using Vite
- Uses Ant Design (antd) for UI components
- TypeScript for type safety
- Exports both ES and CommonJS modules
- Peer dependencies on React 18.3+

## Integration Notes
- Component expects a valid DependencyNode with an `id` field
- Backend must implement the git-related endpoints
- The component handles transformation of backend responses to frontend data structures
- Currently uses a hardcoded commit message ("Update changes") - consider adding UI for custom messages