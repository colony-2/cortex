# Integration Test Issues - Remaining Failures

## Current Status
- **Total Tests**: 43
- **Passing**: 22 (51%)
- **Failing**: 21 (49%)

## Fixed Issues ✅
1. **API Endpoint Mismatch** - Updated from `/api/nodes/*` to `/api/cells/*`
2. **CORS Errors** - Added `--cors-origins http://localhost:5173` to testserver
3. **Missing User Input Endpoints** - Registered InputManagementService in testserver
4. **Navigation Routes** - Updated from `/boxes` to `/cells`
5. **URL Patterns** - Updated from `/box/*` to `/cell/*`
6. **Cell ID Format** - Added "example-" prefix to all cell IDs
7. **React Flow Node Visibility** - Added `state: 'attached'` to waitForSelector calls

## Remaining Issues 🔴

### 1. Node Click Event Not Triggering Navigation
**Affected Tests**: 
- `ui-integration.spec.ts:13` - should show file browser in side panel when clicking node
- `ui-integration.spec.ts:55` - should switch between nodes and update file browser
- `ui-integration.spec.ts:107` - should maintain side panel state when switching tabs

**Issue**: 
- Clicking on `.react-flow__node` elements dispatches `cellSelected` event
- MainView component receives the event but navigation doesn't occur
- URL doesn't update to `/cell/{cellId}` after click
- Side panel doesn't show cell-specific tabs (Files, Changes, etc.)

**Root Cause**: 
- The `cellSelected` event is fired but the navigation logic in MainView may not be executing
- Possible race condition between event handling and React state updates

### 2. Tabs Not Visible When Cell Selected
**Affected Tests**:
- `file-browser.spec.ts:15` - should show files tab when a node is selected
- `side-panel.spec.ts:26` - should show files tab when a node is selected
- `git-changes.spec.ts:19` - should display changes tab with subtabs
- `git-changes.spec.ts:32` - should show summary tab by default

**Issue**:
- When navigating directly to `/cell/example-api`, tabs don't appear
- `.ant-tabs-tab` elements with "Files", "Changes" text not found
- Side panel shows configuration tab instead of cell-specific tabs

**Root Cause**:
- SidePanel component checks for `selectedCell` or `cellId` to show tabs
- Direct URL navigation may not properly set `selectedCell` state
- GraphFlow component's cell selection sync with URL params may be broken

### 3. File Browser Not Loading
**Affected Tests**:
- `file-browser.spec.ts:44` - should display files when node is selected
- `file-browser.spec.ts:71` - should navigate folders in file browser
- `file-browser.spec.ts:100` - should navigate using breadcrumb

**Issue**:
- `/api/cells/example-api/files` endpoint exists but file list doesn't render
- `.ant-list-item` elements not appearing
- Breadcrumb navigation not functional

**Root Cause**:
- GraphBuilder's `GetCell` method may not be properly resolving cell paths
- File browser component may not be receiving correct props

### 4. Container/Devcontainer Tests Failing
**Affected Tests**:
- `env-editor.spec.ts:9` - should show create button when devcontainer.json does not exist
- `env-editor.spec.ts:48` - should show read-only view when file exists

**Issue**:
- Config tab content not loading properly
- Env subtab within Config not accessible
- Container status endpoint mocked but component not rendering

**Root Cause**:
- Config tab structure may have changed
- EnvEditor component may not be properly integrated

### 5. Graph Edges Not Rendering
**Affected Tests**:
- `graph-visualizer.spec.ts:80` - should display edges between cells

**Issue**:
- `.react-flow__edge` elements count is 0
- Nodes render but connections between them don't appear

**Root Cause**:
- Graph data may not include edge information
- React Flow edge rendering configuration issue

### 6. Position Persistence Issues
**Affected Tests**:
- `position-persistence.spec.ts:12` - should save node positions when dragged
- `position-persistence.spec.ts:156` - should maintain relative positions

**Issue**:
- Node dragging doesn't trigger position save
- `/api/positions` POST not being called after drag

**Root Cause**:
- `onNodesChange` handler may not be properly saving positions
- Drag events may not be properly captured

### 7. Git Integration Issues
**Affected Tests**:
- `git-changes.spec.ts:60` - should display git status in summary tab
- `git-changes.spec.ts:97` - should handle commit action
- `git-changes.spec.ts:138` - should display diff in details tab
- `git-changes.spec.ts:173` - should display commit history
- `git-changes.spec.ts:214` - should show empty state when no changes

**Issue**:
- Git tabs not accessible when Changes tab clicked
- Mock responses configured but UI not updating

**Root Cause**:
- Changes tab may not be properly initialized
- GitChanges component may have different props/structure

## Recommended Next Steps

### High Priority
1. **Fix Node Click Handling**
   - Debug `cellSelected` event listener in GraphFlow
   - Verify MainView's `handleCellSelect` is being called
   - Check navigation logic and state updates

2. **Fix Tab Visibility Logic**
   - Debug SidePanel's tab rendering conditions
   - Verify `selectedCell` state propagation
   - Check URL param to state synchronization

### Medium Priority
3. **Fix File Browser Integration**
   - Verify GraphBuilder implementation
   - Check file browser API integration
   - Debug FileBrowser component props

4. **Fix Graph Edge Rendering**
   - Check graph data structure for edges
   - Verify React Flow edge configuration

### Low Priority
5. **Fix Container/Config Tests**
   - Update tests to match current Config tab structure
   - Verify EnvEditor integration

6. **Fix Position Persistence**
   - Debug drag event handlers
   - Verify position save API calls

## Test Execution Command
```bash
moon ui-app:integration
```

## Individual Test Debugging
```bash
# Run specific test with visible browser
npx playwright test tests/[test-file].spec.ts:[line] --headed --timeout=60000
```