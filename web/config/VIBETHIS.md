# web/config

## Purpose
This directory provides configuration management UI components for vibethis boxes, specifically focused on devcontainer configuration and management. It serves as a React component library that handles the editing and lifecycle management of development containers.

## Key Components

### EnvEditor
The primary component that provides:
- **Devcontainer Configuration Management**: Read, create, edit, and save `devcontainer.json` files
- **Dual Edit Modes**: 
  - GUI mode: Form-based editing using React JSON Schema Form (RJSF) with Ant Design widgets
  - IDE mode: Monaco Editor for direct JSON editing with schema validation
- **Container Lifecycle Management**: Create, start, restart, stop, and reset development containers
- **Real-time Status Monitoring**: Displays current container status (none, stopped, running)

## Technical Architecture

### Frontend Stack
- React 18 with TypeScript
- Ant Design for UI components
- Monaco Editor for code editing
- React JSON Schema Form (RJSF) for dynamic form generation
- js-yaml for configuration parsing

### API Integration
Interfaces with backend endpoints:
- `GET /api/nodes/{nodeId}/files/.devcontainer/devcontainer.json` - Load configuration
- `PUT /api/nodes/{nodeId}/files/.devcontainer/devcontainer.json` - Save configuration
- `GET /api/nodes/{nodeId}/container/status` - Check container status
- `POST /api/nodes/{nodeId}/container/create` - Create container
- `POST /api/nodes/{nodeId}/container/start` - Start container
- `POST /api/nodes/{nodeId}/container/restart` - Restart container
- `POST /api/nodes/{nodeId}/container/reset` - Reset/remove container

### Key Features
1. **Smart File Handling**: Detects missing devcontainer.json and provides creation UI
2. **Schema Validation**: Uses official devcontainer schema for JSON validation
3. **Error Handling**: Detailed error modals with collapsible technical details
4. **State Management**: Tracks edit mode, original content, and container status
5. **Form Schema**: Predefined JSON schema for common devcontainer properties

## Integration Points
- Consumed by `@vibethis/app` in the SidePanel component
- Uses `@vibethis/shared` for type definitions (DependencyNode)
- Published as `@graph-visualizer/config` npm package

## Build Configuration
- Vite-based library build
- Outputs ES and CommonJS modules
- External dependencies: React, ReactDOM, Ant Design, and shared packages

## Future Considerations
- Current test coverage is minimal (placeholder test only)
- Form schema could be extended for more devcontainer features
- Container logs/output viewing could be added
- Support for multiple container configurations per box