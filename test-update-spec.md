# Integration Test Update Specification

## Executive Summary

The integration tests for the UI app have not been maintained and are failing due to several architectural changes:
1. API endpoint naming change from `/api/nodes/*` to `/api/cells/*`
2. Missing `/api/user-inputs/stream` endpoint
3. UI route changes from `/boxes` to root path `/`
4. Component structural changes

## Test Case Analysis and Proposed Updates

### 1. env-editor.spec.ts (2 tests)
**Current Tests:**
- `should show create button when devcontainer.json does not exist`
- `should show read-only view when file exists and not in edit mode`

**Issue:** Using `/api/nodes/*/container/status` instead of `/api/cells/*/container/status`

**Spirit Valid:** YES - Container configuration is still a core feature

**Proposed Update:**
- Update all API routes from `/api/nodes/*` to `/api/cells/*`
- Verify the container status endpoint response structure matches current implementation
- Keep test logic as-is since devcontainer management is still supported

### 2. file-browser.spec.ts (5 tests)
**Current Tests:**
- `should display root directory contents`
- `should navigate into subdirectories`
- `should navigate back using breadcrumbs`
- `should show empty state for empty directories`
- `should handle navigation errors gracefully`

**Issue:** Using `/api/nodes/*/files` instead of `/api/cells/*/files`

**Spirit Valid:** YES - File browsing is a core feature per VIBETHIS.md

**Proposed Update:**
- Update all API routes from `/api/nodes/*` to `/api/cells/*`
- Update initial navigation from `/boxes` to `/`
- Verify file browser component selectors are still valid

### 3. git-changes.spec.ts (6 tests)
**Current Tests:**
- `should handle non-git repository gracefully`
- `should display git status in summary tab`
- `should handle commit action`
- `should display diff in details tab`
- `should display commit history in history tab`
- `should show empty state when no changes`

**Issue:** Using `/api/nodes/*/git/*` instead of `/api/cells/*/git/*`

**Spirit Valid:** YES - Git integration is a core feature per VIBETHIS.md

**Proposed Update:**
- Update all API routes from `/api/nodes/*` to `/api/cells/*`
- Keep all test logic intact as git functionality remains unchanged

### 4. graph-visualizer.spec.ts (5 tests)
**Current Tests:**
- `should load and display the graph`
- `should display cell information correctly`
- `should have working zoom controls`
- `should display edges between cells`
- `should allow panning the graph`

**Issue:** Initial route is `/boxes` instead of `/`

**Spirit Valid:** YES - Graph visualization is the primary UI feature

**Proposed Update:**
- Change initial navigation from `/boxes` to `/`
- Verify React Flow selectors are still valid
- Ensure graph endpoint `/api/graph` is functioning correctly

### 5. position-persistence.spec.ts (3 tests)
**Current Tests:**
- `should save node positions when dragged`
- `should restore saved node positions on reload`
- `should maintain relative positions when multiple nodes are moved`

**Issue:** Initial route is `/boxes` instead of `/`

**Spirit Valid:** YES - Position persistence is essential for user experience

**Proposed Update:**
- Change initial navigation from `/boxes` to `/`
- Verify `/api/positions` endpoints work correctly
- Update node/cell terminology if needed in assertions

### 6. side-panel.spec.ts (7 tests)
**Current Tests:**
- `should show configuration tab when no node is selected`
- `should show files tab when a node is selected`
- `should show Config tab with Claude Code subtab when a node is selected`
- `should display files when node is selected`
- `should navigate folders in file browser`
- `should navigate using breadcrumb`
- `should maintain side panel width`

**Issue:** Using `/api/nodes/*/files` and incorrect initial route

**Spirit Valid:** YES - Side panel is a primary UI component

**Proposed Update:**
- Update API routes from `/api/nodes/*` to `/api/cells/*`
- Change initial navigation from `/boxes` to `/`
- Verify Claude Code subtab still exists or update to current config structure

### 7. tab-restoration.spec.ts (6 tests)
**Current Tests:**
- `should restore Files tab when reloading /box/<id>/files`
- `should restore Config tab when reloading /box/<id>/config`
- `should restore Config tab with Env subtab when reloading /box/<id>/config/env`
- `should restore Changes tab when reloading /box/<id>/changes`
- `should restore Changes tab with History subtab when reloading /box/<id>/changes/history`
- `should restore Changes tab with Details subtab when reloading /box/<id>/changes/details`

**Issue:** 
- Using `/api/nodes/*/git/*` endpoints
- URL structure uses `/box/<id>/*` instead of current routing

**Spirit Valid:** PARTIAL - Tab restoration is important but URL structure has changed

**Proposed Update:**
- Investigate current URL routing structure
- Update from `/box/<id>/*` to current URL pattern (likely `/cells/<id>/*` or similar)
- Update API routes from `/api/nodes/*` to `/api/cells/*`
- If URL-based routing no longer exists, consider converting to state persistence tests

### 8. ui-integration.spec.ts (3 tests)
**Current Tests:**
- `should show file browser in side panel when clicking node`
- `should switch between nodes and update file browser`
- `should maintain side panel state when switching tabs`

**Issue:** 
- Using `/api/nodes/*/files`
- Initial route is `/boxes`

**Spirit Valid:** YES - Core UI interaction flows

**Proposed Update:**
- Update API routes from `/api/nodes/*` to `/api/cells/*`
- Change initial navigation from `/boxes` to `/`
- Verify component interaction patterns are still valid

### 9. url-state.spec.ts (4 tests)
**Current Tests:**
- `should update URL when selecting a cell`
- `should update URL when changing tabs`
- `should update URL when changing subtabs in Changes`
- `should restore state from URL on page load`

**Issue:** 
- Initial route and URL patterns
- API endpoint names

**Spirit Valid:** YES - URL state management is mentioned in VIBETHIS.md as a key pattern

**Proposed Update:**
- Investigate current URL state patterns
- Update URL assertions to match current routing
- Update API routes from `/api/nodes/*` to `/api/cells/*`

## Missing Functionality - User Input Stream

### Root Cause of `/api/user-inputs/stream` 404 Error:
The `InputManagementService` that provides the `/api/user-inputs/stream` endpoint exists in `server/ops/pkg/input/management.go` but is only registered in the cortex server setup (`server/cortex/internal/setup/setup.go`), NOT in the testserver used for integration tests.

### Fix Required:
The testserver (`server/api/cmd/testserver/main.go`) needs to register the InputManagementService routes. This involves:
1. Import the input package: `"github.com/divisive-ai/vibethis/server/ops/pkg/input"`
2. Create and initialize the InputManagementService
3. Add its routes as ExtensionRoutes to the web.Dependencies

### Implementation for testserver:
```go
// In server/api/cmd/testserver/main.go, add to runServer function:

// Create input management service
inputService := input.NewInputManagementService()
inputService.Initialize(input.ServiceDependencies{})

// Convert input service routes to extension routes
var extensionRoutes []web.ExtensionRoute
for _, route := range inputService.GetRoutes() {
    path := route.Path
    if strings.HasPrefix(path, "/api") {
        path = strings.TrimPrefix(path, "/api")
    }
    extensionRoutes = append(extensionRoutes, web.ExtensionRoute{
        Method:  route.Method,
        Path:    path,
        Handler: route.Handler,
    })
}

// Add to Dependencies struct:
deps := web.Dependencies{
    Storage:         store,
    Graph:           graphBuilder,
    Files:           fileBrowser,
    Git:             gitRepo,
    Container:       containerManager,
    ExtensionRoutes: extensionRoutes, // Add this line
}
```

### Test Coverage for User Input Stream:
Once the endpoint is properly registered, existing tests that expect this endpoint will work. Additional test cases to consider:
1. **SSE Connection Test**
   - Test initial connection to `/api/user-inputs/stream`
   - Verify SSE event format
   - Test reconnection on disconnect

2. **User Input Flow Test**
   - Test pending inputs via `/api/user-inputs/pending`
   - Test input response submission
   - Test real-time updates via SSE

2. **Recipe/Workflow Tests** (based on VIBETHIS.md Ono features)
   - Test recipe visualization if exposed in UI
   - Test workflow status monitoring

3. **Container WebSocket Terminal** (mentioned in VIBETHIS.md)
   - Test terminal attachment if implemented
   - Test input/output streaming

## Implementation Priority

### Critical (Blocking All Tests):
1. **Fix testserver to register InputManagementService** - This will resolve the 404 errors for `/api/user-inputs/stream`

### High Priority (Core Functionality):
2. Update all API endpoints from `/api/nodes/*` to `/api/cells/*`
3. Update initial navigation from `/boxes` to `/`
4. Fix graph-visualizer.spec.ts (primary feature)
5. Fix file-browser.spec.ts (essential feature)
6. Fix git-changes.spec.ts (version control)

### Medium Priority (Important UX):
7. Fix position-persistence.spec.ts
8. Fix side-panel.spec.ts
9. Fix ui-integration.spec.ts

### Low Priority (May Need Redesign):
10. Fix url-state.spec.ts (investigate current patterns first)
11. Fix tab-restoration.spec.ts (depends on URL routing)
12. Fix env-editor.spec.ts (container features)

## Technical Debt Items

1. **Standardize terminology**: Decide on "nodes" vs "cells" vs "boxes"
2. **Update test data**: Ensure example repository structure matches current expectations
3. **Add API response validation**: Tests should verify response schemas
4. **Improve test isolation**: Reduce dependency on specific UI selectors
5. **Consider unified server binary**: Evaluate if testserver should be replaced with cortex server for testing

## Next Steps

1. **Fix testserver to include InputManagementService registration** (resolves 404 errors)
2. Run a quick search-and-replace for API endpoint updates (`/api/nodes/*` → `/api/cells/*`)
3. Update navigation routes (`/boxes` → `/`)
4. Run tests individually to identify component-specific issues
5. Update selectors based on current React component structure
6. Add new tests for any features added since tests were last maintained