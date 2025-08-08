# Terminology Standardization Specification

## Executive Summary

This specification outlines the comprehensive renaming of inconsistent terminology throughout the vibethis codebase. The main changes are:
- **nodes/boxes → cells** (unified term for components)
- **recipes/workflows → recipes** (except for Temporal workflows)
- **activities → ops** (except for Temporal activities)

## Current State Analysis

### 1. Component Terminology (cells/nodes/boxes)

**Current Usage:**
- "cells" appears in spec documents (spec-inner-recipe-cell-executor.md) - CORRECT TERM
- "nodes" widely used in UI components, API handlers, and types - NEEDS CHANGE
- "boxes" used in VIBETHIS.md docs, test files, and some activity code - NEEDS CHANGE
- Mixed usage creates confusion about the primary abstraction

**Files and Directories Affected:**
- Frontend components: ProFlowNode.tsx, GraphFlow.tsx, types.ts, api.ts, all test specs
- Backend: handlers.go, core types, storage implementations
- Documentation: Multiple spec files, VIBETHIS.md files
- Package names and import paths

### 2. Orchestration Terminology (recipes/workflows)

**Current Usage:**
- "recipes" used in recipe-worker, recipe-core, recipe-history modules
- "workflows" used for both Temporal workflows AND recipe workflows
- Ambiguous when referring to recipe execution vs Temporal execution

**Files Affected:**
- All recipe-* modules
- Temporal integration files
- Example YAML files

### 3. Action Terminology (activities/ops)

**Current Usage:**
- "activities" used for both Temporal activities AND recipe activities
- Creates confusion between Temporal's activity concept and recipe actions
- Example: research_activity, analyze_activity in recipe YAML files

**Files Affected:**
- server/activity modules
- Recipe YAML examples
- Activity registration and execution code

## Directory Structure Changes Summary

### Directories to Rename
```
server/activity/ → server/ops/
server/recipe-worker/examples/research_project/activities.yaml → ops.yaml
```

### Files to Rename
```
# Frontend
web/flowchart/src/ProFlowNode.tsx → ProFlowCell.tsx

```

### Module/Package Updates
```
# Go modules
github.com/divisive-ai/vibethis/server/activity → github.com/divisive-ai/vibethis/server/ops

# NPM packages (if published separately)
@vibethis/node-types → @vibethis/cell-types
@vibethis/box-utils → @vibethis/cell-utils
```

## Proposed Changes

### Phase 1: Standardize Component Terminology (nodes/boxes → cells)

#### Directory and Package Renames
```
# No directory renames needed for "cells" as it's not currently used in directory names
# But we need to update package references where "node" or "box" appears
```

#### Frontend Changes
```
web/flowchart/src/ProFlowNode.tsx → ProFlowCell.tsx
- Component: ProFlowNode → ProFlowCell
- Props: nodeId → cellId
- Events: nodeSelected → cellSelected
- CSS classes: .graph-node → .graph-cell

web/shared/src/types.ts:
- Interface: Node → Cell
- Field: nodeId → cellId
- Field: nodeName → cellName
- Field: boxId → cellId (where it appears)

web/shared/src/api.ts:
- Endpoints: /api/nodes → /api/cells
- Endpoints: /api/boxes → /api/cells
- Functions: getNode() → getCell()
- Functions: getBox() → getCell()
- Functions: updateNodePosition() → updateCellPosition()
- Functions: updateBoxPosition() → updateCellPosition()

API Request/Response Objects:
- Request body: { nodeId: "123" } → { cellId: "123" }
- Request body: { boxId: "456" } → { cellId: "456" }
- Response: { nodes: [...] } → { cells: [...] }
- Response: { node: {...} } → { cell: {...} }
- Response: { box: {...} } → { cell: {...} }
- Field: node.nodeId → cell.cellId
- Field: box.boxId → cell.cellId
```

#### Backend Changes
```
server/core/pkg/core/types.go:
- Type: Node → Cell
- Type: Box → Cell
- Field: NodeID → CellID
- Field: BoxID → CellID

server/api/internal/handlers/handlers.go:
- Handler: NodeHandler → CellHandler
- Handler: BoxHandler → CellHandler
- Routes: /api/nodes → /api/cells
- Routes: /api/boxes → /api/cells
- Routes: /api/nodes/:nodeId → /api/cells/:cellId
- Routes: /api/boxes/:boxId → /api/cells/:cellId
- Routes: /api/nodes/:nodeId/files → /api/cells/:cellId/files
- Routes: /api/nodes/:nodeId/git → /api/cells/:cellId/git
- Routes: /api/nodes/:nodeId/container → /api/cells/:cellId/container

server/graph/internal/builder/builder.go:
- Function: BuildNode() → BuildCell()
- Function: BuildBox() → BuildCell()
- Field: nodes → cells
- Field: boxes → cells

server/storage/internal/*/storage.go:
- Field: NodePositions → CellPositions
- Field: BoxPositions → CellPositions
```

#### Test Changes
```
web/app/tests/*.spec.ts:
- Replace all "node" references with "cell"
- Replace all "box" references with "cell"
- Update selectors and assertions
```

#### Import Path Updates
```
# Update all import statements that reference "node" or "box" in package names
# Example:
- import { NodeType } from '@vibethis/node-utils'
+ import { CellType } from '@vibethis/cell-utils'
```

### Phase 2: Standardize Orchestration Terminology (workflows → recipes)

**Keep "workflow" only for:**
- Temporal workflow interfaces and types
- go.temporal.io/sdk/workflow imports
- Temporal-specific workflow contexts

**Change to "recipe" for:**
- All recipe execution logic
- YAML workflow definitions
- Recipe orchestration code

#### Example Changes
```
server/recipe-worker/examples/parallel_workflow.yaml → parallel_recipe.yaml
server/recipe-worker/examples/gemini_workflow.yaml → gemini_recipe.yaml

YAML content:
workflows: → recipes:
workflow_name: → recipe_name:
```

### Phase 3: Standardize Action Terminology (activities → ops)

**Keep "activity" only for:**
- Temporal activity interfaces
- go.temporal.io/sdk/activity imports
- RegisterActivity() for Temporal registration

**Change to "ops" for:**
- Recipe action definitions
- YAML activity definitions
- Non-Temporal execution units

#### Directory and Package Renames
```
server/activity/ → server/ops/
- Update all imports from "server/activity" to "server/ops"
- Keep internal Temporal activity wrappers but rename recipe-specific parts

server/recipe-worker/examples/research_project/activities.yaml → ops.yaml

Package renames in go.mod files:
- module github.com/divisive-ai/vibethis/server/activity → server/ops
- Update all import paths accordingly
```

#### Code Changes
```
Content changes in YAML files:
activities: → ops:
  - name: research_activity → research_op
  - name: analyze_activity → analyze_op  
  - name: write_report_activity → write_report_op

server/ops/* (formerly server/activity/*):
- RecipeActivity → RecipeOp
- ActivityConfig → OpConfig
- ExecuteActivity() → ExecuteOp() (for non-Temporal execution)
- ActivityMetadata → OpMetadata (for recipe ops)
- pkg/activity/ → pkg/ops/ (for recipe-specific packages)
- pkg/activity/exports.go → pkg/ops/exports.go 

Keep unchanged:
- Temporal activity registration functions
- workflow.Context activity calls
```

#### Import Updates
```
# Update all Go imports
- import "github.com/divisive-ai/vibethis/server/activity/pkg/input"
+ import "github.com/divisive-ai/vibethis/server/ops/pkg/input"

# Update all recipe references
- type: activity
+ type: op

# Update configuration files
- activity_providers → op_providers
- ACTIVITY_PROVIDERS.md → OP_PROVIDERS.md
```

#### API Changes for Ops
```
# REST endpoints (if exposed)
- /api/activities → /api/ops
- /api/recipes/:id/activities → /api/recipes/:id/ops

# JSON response fields
- { "activities": [...] } → { "ops": [...] }
- { "activityId": "123" } → { "opId": "123" }
- { "activityName": "research" } → { "opName": "research" }
- { "activityStatus": "running" } → { "opStatus": "running" }

# YAML recipe definitions
- activities: → ops:
- activity_type: → op_type:
- activity_config: → op_config:
```

### Phase 4: Documentation Updates

Update all documentation files:
```
VIBETHIS.md:
- Replace all "boxes" with "cells"
- Replace all "nodes" with "cells"  
- Update architecture descriptions to use "cells" consistently

spec-*.md files:
- spec-inner-recipe-cell-executor.md (already correct, keep as is)
- Update all references to nodes → cells
- Update all references to boxes → cells
- Update all non-Temporal workflow → recipe
- Update all non-Temporal activity → op

Documentation file renames:
- server/activity/input-activity-spec-concise.md → server/ops/input-op-spec-concise.md
- server/recipe-worker/ACTIVITY_PROVIDERS.md → server/recipe-worker/OP_PROVIDERS.md
- server/ACTIVITY_WRAPPER_SPEC.md → server/OP_WRAPPER_SPEC.md
```

## Implementation Order

1. **Preparation Phase**
   - Create comprehensive test suite to validate no functional changes
   - Set up automated renaming scripts
   - Create git branch for changes

2. **Backend Core Types** (Priority 1)
   - Rename core domain types (Node → Cell, Box → Cell)
   - Update storage interfaces
   - Update API contracts
   - Update package imports

3. **API Layer** (Priority 2)
   - Update OpenAPI specification (vibethis-api.yaml)
     - All paths: /nodes → /cells, /boxes → /cells
     - All schemas: Node → Cell, Box → Cell
     - All properties: nodeId → cellId, boxId → cellId
     - All descriptions mentioning nodes/boxes → cells
   - Regenerate client/server code
   - Update REST endpoints and all route parameters

4. **Frontend Components** (Priority 3)
   - Rename React component files (ProFlowNode.tsx → ProFlowCell.tsx)
   - Update TypeScript types
   - Update API client calls
   - Update tests

5. **Recipe System** (Priority 4)
   - Rename server/activity directory to server/ops
   - Rename recipe workflows to recipes
   - Rename recipe activities to ops
   - Update YAML parsing
   - Update examples
   - Update all import paths

6. **Documentation** (Priority 5)
   - Update all markdown files
   - Update code comments
   - Update VIBETHIS.md files
   - Rename documentation files containing "activity" to "op"

## Validation Checklist

- [ ] All tests pass after renaming (`moon :test` and `moon :integration`)
- [ ] No functional changes introduced
- [ ] Documentation is consistent
- [ ] Examples work correctly
- [ ] Temporal integration unchanged
- [ ] Build system (moon) updated
- [ ] All REST API endpoints use new terminology
- [ ] All API response objects use new field names


## Excluded from Changes

These should NOT be renamed:
- Temporal SDK imports and types
- go.temporal.io/sdk/workflow references
- go.temporal.io/sdk/activity references
- Docker/container terminology
- Git terminology
- Moon build system references

## Success Criteria

- Zero functional regressions
- Consistent terminology throughout codebase
- Clear distinction between Temporal and recipe concepts
- All API endpoints use cells/recipes/ops terminology
- All JSON/YAML fields use new naming
- No references to old terminology remain (except in Temporal contexts)