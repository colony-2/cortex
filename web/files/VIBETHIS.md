# web/files

## Purpose
React component library providing file browsing capabilities for vibethis boxes. Enables users to navigate and explore the filesystem structure within each box directory.

## Core Component
**FileBrowser** - The main component that renders a hierarchical file explorer interface
- Accepts a `DependencyNode` or `boxId` to identify which box's files to display
- Integrates with the backend file API at `/nodes/{nodeId}/files`
- Supports folder navigation with breadcrumb trail
- Displays file metadata (size, type, icons)

## Features
- **Directory Navigation**: Click folders to browse into subdirectories
- **Breadcrumb Trail**: Shows current path with clickable segments for quick navigation
- **File Type Indicators**: Visual distinction between files and folders with appropriate icons
- **File Extensions**: Shows file type tags for quick identification
- **Loading States**: Proper loading indicators during API calls
- **Error Handling**: Graceful error display when file loading fails
- **Empty State**: Clear messaging when directories are empty

## Integration Points
- **API**: Uses `fetchFiles` from `@vibethis/shared` to communicate with backend
- **UI Library**: Built with Ant Design components (List, Breadcrumb, Card, etc.)
- **Build**: Configured as a Vite library for use across the application

## Technical Details
- Transforms backend file data into UI-friendly format
- Maintains current path state for navigation
- Formats file sizes for human readability
- Mouse hover effects for interactive folder selection
- TypeScript interfaces for type safety

## Usage Pattern
```tsx
import { FileBrowser } from '@vibethis/files';

// With node object
<FileBrowser node={selectedNode} />

// With boxId directly
<FileBrowser boxId="my-box-id" />
```

## Notes
- Files are read-only - no editing/deletion capabilities currently
- File clicking is only enabled for directories (navigation)
- Component height fills container with scrollable file list
- No file content preview - only metadata display