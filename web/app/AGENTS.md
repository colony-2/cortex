# VibethisUI Main Application

## Overview
React-based graph visualization application that provides an interactive interface for exploring dependency graphs, managing file systems, and handling workflow inputs. Built with Ant Design components and integrates multiple specialized modules for a comprehensive development workflow experience.

## Architecture

### Core Application Structure
**Framework**: React 18 with TypeScript, Vite build system
**UI Library**: Ant Design (antd) with @ant-design/icons
**Routing**: React Router v6 for SPA navigation and deep linking
**State**: URL-based state management with component-level useState hooks

### Component Hierarchy
```
App (BrowserRouter + InputActivityProvider)
├── MainView (Split layout)
│   ├── GraphFlow (Left panel - from @vibethis/flowchart)
│   └── SidePanel (Right panel - contextual tabs)
│       ├── InputFormsTab (Workflow input management)
│       ├── FileBrowser (from @vibethis/files)
│       ├── EnvEditor (from @vibethis/config)
│       └── GitChanges (from @vibethis/changes)
└── InputFormRenderer (Dynamic form generation)
```

### Module Dependencies
- `@vibethis/shared`: Core types, API layer, and utilities
- `@vibethis/flowchart`: Graph visualization with ReactFlow
- `@vibethis/files`: File browser component
- `@vibethis/config`: Environment configuration editor
- `@vibethis/changes`: Git change tracking and visualization

## Key Interfaces

### Core Types (from @vibethis/shared)
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

interface PendingInput {
  workflowId: string;
  formTitle: string;
  status: 'pending' | 'completed';
  createdAt: string;
  expiresAt: string;
}
```

### Main Component Props
```typescript
// MainView.tsx
interface MainViewProps {
  // Uses URL params: cellId, tab, subtab
  // Manages selectedCell state and navigation
}

// SidePanel.tsx
interface SidePanelProps {
  selectedCell: DependencyCell | null;
}

// InputFormRenderer.tsx
interface InputFormRendererProps {
  form: InputForm;
  context?: FormContext;
  onSubmit: (response: FormResponse) => void;
  onCancel?: () => void;
  loading?: boolean;
}
```

### API Layer
```typescript
// API functions in @vibethis/shared
async function fetchGraph(): Promise<RelationshipGraph>
async function fetchPositions(): Promise<CellPosition[]>
async function savePositions(positions: CellPosition[]): Promise<void>
async function fetchFiles(cellId: string, path?: string): Promise<{files: any[], path: string}>
```

## Usage Examples

### Basic Application Setup
```typescript
// App.tsx - Main routing configuration
function App() {
  return (
    <BrowserRouter>
      <InputActivityProvider>
        <Routes>
          <Route path="/cells" element={<MainView />} />
          <Route path="/cell/:cellId" element={<MainView />} />
          <Route path="/cell/:cellId/:tab" element={<MainView />} />
          <Route path="/cell/:cellId/:tab/:subtab" element={<MainView />} />
        </Routes>
      </InputActivityProvider>
    </BrowserRouter>
  );
}
```

### Cell Selection and Navigation
```typescript
// MainView.tsx - Handle cell selection
const handleCellSelect = useCallback((cell: DependencyCell | null) => {
  setSelectedCell(cell);
  if (cell) {
    const path = navigateToPath({ cellId: cell.id, tab: tab || 'files' });
    navigate(path);
  }
}, [navigate, tab]);
```

### Dynamic Tab Configuration
```typescript
// SidePanel.tsx - Conditional tab rendering
const items = showCellTabs ? [
  { key: 'files', label: 'Files', children: <FileBrowser cell={selectedCell} /> },
  { key: 'inputs', label: 'Inputs', children: <InputFormsTab cell={selectedCell} /> },
  { key: 'config', label: 'Config', children: <CellConfigTabs /> },
  { key: 'changes', label: 'Changes', children: <GitChanges /> }
] : [
  { key: 'config', label: 'Configuration', children: <ConfigurationTabs /> }
];
```

### Form Rendering with Validation
```typescript
// InputFormRenderer.tsx - Dynamic form field generation
const renderField = (field: InputField) => {
  switch (field.type) {
    case 'short_answer':
      return <Input placeholder={field.placeholder} />;
    case 'multiple_choice':
      return (
        <Radio.Group>
          {field.options?.map(option => 
            <Radio key={option} value={option}>{option}</Radio>
          )}
        </Radio.Group>
      );
    // Additional field types: paragraph_text, checkboxes, dropdown, 
    // linear_scale, date, time, file_upload
  }
};
```

## Configuration

### Development Server
```bash
# Start development server
npm run dev  # or moon run ui-app:serve
# Runs on http://localhost:5173
```

### Build Configuration (vite.config.ts)
```typescript
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@vibethis/shared': resolve(__dirname, '../shared/src/index.ts'),
      '@vibethis/flowchart': resolve(__dirname, '../flowchart/src/index.ts'),
      // Other module aliases
    }
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor': ['react', 'react-dom', 'antd'],
          'monaco': ['@monaco-editor/react']
        }
      }
    }
  }
});
```

### API Configuration
```typescript
// Environment-based API configuration
const API_BASE = import.meta.env.DEV 
  ? 'http://localhost:8080/api' 
  : '/api';
```

### Testing Setup
- **E2E Testing**: Playwright with headless mode
- **Unit Testing**: Vitest with jsdom environment  
- **Test Commands**: `npm run test` (unit), `npm run test:e2e` (playwright)
- **Coverage**: Source maps enabled for debugging