# GitChanges Component Library

## Overview
The web/changes module provides a React component for Git change tracking and visualization within the vibethis platform. It displays Git status, diffs, and commit history for dependency cells with integrated commit functionality.

## Architecture
The module consists of a single main component with supporting types and utilities:

### Core Components
- **GitChanges**: Main React component (`src/GitChanges.tsx`)
- **Type Definitions**: Local interfaces for Git operations
- **API Integration**: REST endpoint communication layer

### Data Flow
```
DependencyCell -> GitChanges -> API Endpoints -> Git Backend
                            ↓
                    Tab-based UI (Summary/Details/History)
```

### Component Structure
- Tab-based interface with three views
- State management for Git data (status, diff, history)
- Async data loading with loading states
- Error handling for non-Git repositories

## Key Interfaces

### GitChangesProps
```typescript
interface GitChangesProps {
  cell: DependencyCell | null;
  activeTab?: string;
  onTabChange?: (key: string) => void;
}
```

### GitStatus
```typescript
interface GitStatus {
  added: string[];
  modified: string[];
  deleted: string[];
  untracked: string[];
  totalCount: number;
}
```

### GitCommit
```typescript
interface GitCommit {
  hash: string;
  author: string;
  email: string;
  message: string;
  timestamp: string;
}
```

### GitFileDiff
```typescript
interface GitFileDiff {
  path: string;
  status: string;
  additions: number;
  deletions: number;
  patch: string;
}
```

### Main Functions
- `fetchStatus()`: Retrieves Git status from API
- `fetchDiff()`: Gets file diffs from API
- `fetchHistory()`: Loads commit history
- `handleCommit()`: Creates new commits
- `parseDiff(diffText: string)`: Parses Git diff text to structured format

## Usage Examples

### Basic Implementation
```typescript
import { GitChanges } from '@graph-visualizer/changes';

function MyComponent() {
  const [activeTab, setActiveTab] = useState('summary');
  const [selectedCell, setSelectedCell] = useState<DependencyCell | null>(null);

  return (
    <GitChanges
      cell={selectedCell}
      activeTab={activeTab}
      onTabChange={setActiveTab}
    />
  );
}
```

### API Endpoint Requirements
```typescript
// Backend must implement these endpoints:
GET /api/cells/{cellId}/git/status    // Returns file status array
GET /api/cells/{cellId}/git/diff      // Returns plain text diff
GET /api/cells/{cellId}/git/history   // Returns commit array
POST /api/cells/{cellId}/git/commit   // Creates commit
```

### Status Response Format
```typescript
// Expected API response for /git/status
{
  "files": [
    { "path": "file.ts", "status": "M" },
    { "path": "new.ts", "status": "A" }
  ]
}
```

### Integration with Ant Design
```typescript
// Component uses these Ant Design elements:
import { Tabs, Button, Badge, Space, Typography, Empty, Spin, List, Tag, message } from 'antd';
import { SyncOutlined, FileAddOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons';
```

## Configuration

### Build Configuration (vite.config.ts)
- Library build targeting ES and CommonJS formats
- External dependencies: react, react-dom, antd, @ant-design/icons
- Test exclusions for proper test isolation

### Dependencies
```json
{
  "dependencies": {
    "@graph-visualizer/shared": "file:../shared",
    "antd": "^5.26.1",
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  }
}
```

### Peer Dependencies
- React 18.3+ required
- Ant Design icons as dev dependency

### File Status Mapping
- `A`: Added files (green, FileAddOutlined)
- `M`: Modified files (blue, EditOutlined)  
- `D`: Deleted files (red, DeleteOutlined)
- `?`: Untracked files (yellow, FileAddOutlined)