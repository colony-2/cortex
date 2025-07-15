# Ono Component Split Implementation Plan

## Overview
This plan outlines the refactoring of the monolithic ono application into five focused, reusable Go libraries. The goal is to maintain all existing functionality while improving modularity and reusability.

## Component Architecture

### 1. embeddedtemporal
**Purpose**: Golang library that provides an embedded Temporal server for in-process execution

**Key Responsibilities**:
- Start/stop Temporal server in-process
- Configure Temporal server options
- Provide client connection management
- Handle server lifecycle management

**Dependencies**:
- Temporal SDK
- No dependencies on other new components

### 2. recipe-core
**Purpose**: Core recipe schema definitions and parsing utilities

**Key Responsibilities**:
- Recipe YAML schema definitions (structs)
- Recipe parsing and validation
- Recipe manifest handling
- Common recipe types and interfaces
- Recipe directory structure conventions

**Dependencies**:
- Standard Go libraries only
- No external component dependencies

### 3. recipe-history
**Purpose**: Recipe-centric view of execution history and activity data

**Key Responsibilities**:
- Query Temporal for workflow/activity history
- Map Temporal data to recipe-centric models
- Read recipe definitions from directories
- Provide recipe execution status and results
- Activity history and outputs

**Dependencies**:
- recipe-core (for schema)
- Temporal SDK (for client operations)

### 4. recipe-worker
**Purpose**: Monitor recipe directories and manage worker processes

**Key Responsibilities**:
- Directory monitoring for recipe changes
- Worker registration and lifecycle
- Recipe-to-worker mapping
- Activity implementation loading
- Workflow execution

**Dependencies**:
- recipe-core (for schema)
- Temporal SDK (for worker implementation)

### 5. ono (CLI)
**Purpose**: Thin CLI wrapper orchestrating all components

**Key Responsibilities**:
- CLI command parsing
- Start embedded temporal via embeddedtemporal
- Initialize recipe-worker for directory monitoring
- Expose recipe-history commands
- Maintain existing CLI interface

**Dependencies**:
- embeddedtemporal
- recipe-core
- recipe-history
- recipe-worker

## Implementation Steps

### Phase 1: Create Component Structure
1. Create new Go modules:
   ```
   server/embeddedtemporal/
   server/recipe-core/
   server/recipe-history/
   server/recipe-worker/
   ```

2. Set up moon.yml for each component with standard Go tasks:
   ```yaml
   # Each component's moon.yml
   $schema: "https://moonrepo.dev/schemas/project.json"
   language: go
   type: library
   tags: ["backend", "go"]
   ```

3. Initialize go.mod for each component

#### Test Migration & Evaluation:
- **Tests to identify**: None (new structure only)
- **Validation**: 
  - Verify moon tasks work: `moon run build`, `moon run test`
  - Ensure go.mod is properly initialized
- **Success criteria**: All components have working build/test infrastructure

### Phase 2: Extract embeddedtemporal
1. Identify Temporal server initialization code in ono
2. Extract to embeddedtemporal package:
   - Server configuration structures
   - Start/stop methods
   - Client creation utilities
3. Create unit tests for server lifecycle
4. Update ono imports to use embeddedtemporal

#### Test Migration & Evaluation:
- **Tests to identify**:
  - Search for tests related to Temporal server initialization
  - Tests for server start/stop lifecycle
  - Tests for client connection creation
  - Configuration tests
- **Tests to migrate**:
  - Move `*_test.go` files testing Temporal server functionality
  - Update package declarations and imports
- **New tests to create**:
  - Unit tests for server lifecycle (start/stop/restart)
  - Configuration validation tests
  - Client connection pool tests
- **Validation**:
  - Run `moon run test` in embeddedtemporal
  - Run `moon run test integration` in ono to ensure nothing broke
  - Verify server starts and stops cleanly
- **Success criteria**:
  - All Temporal server tests pass in new location
  - Ono integration tests still pass
  - No decrease in code coverage

### Phase 3: Extract recipe-core
1. Identify all recipe-related types and schemas
2. Extract to recipe-core:
   - Recipe struct definitions
   - Input/output schemas
   - Activity definitions
   - Workflow definitions
   - YAML parsing logic
   - Validation functions
3. Move recipe parsing tests
4. Update all components to import from recipe-core

#### Test Migration & Evaluation:
- **Tests to identify**:
  - Recipe YAML parsing tests
  - Schema validation tests
  - Recipe structure tests
  - Input/output type tests
  - Search for: `*recipe*_test.go`, `*schema*_test.go`, `*parse*_test.go`
- **Tests to migrate**:
  - All tests in ono that test recipe parsing/validation
  - Tests for recipe manifest handling
  - YAML unmarshaling tests
- **New tests to create**:
  - Comprehensive schema validation tests
  - Edge cases for recipe parsing
  - Invalid recipe structure tests
  - Recipe versioning tests
- **Validation**:
  - Run `moon run test` in recipe-core
  - Verify all recipe types are properly exported
  - Check that ono and other components can import recipe-core
  - Run `moon run test integration` in ono
- **Success criteria**:
  - All recipe parsing/validation tests pass
  - No recipe logic remains in ono
  - Other components successfully use recipe-core types
  - 100% test coverage for parsing logic

### Phase 4: Extract recipe-history
1. Identify history querying code
2. Extract to recipe-history:
   - Temporal client wrappers for history
   - Recipe execution status queries
   - Activity result retrieval
   - Job listing and filtering
3. Create interface between Temporal data and recipe models
4. Move history-related tests
5. Update ono CLI to use recipe-history

#### Test Migration & Evaluation:
- **Tests to identify**:
  - Job history query tests
  - Activity result retrieval tests
  - Workflow status tests
  - Recipe execution listing tests
  - Search for: `*history*_test.go`, `*job*_test.go`, `*status*_test.go`
- **Tests to migrate**:
  - All tests for `ono job describe` functionality
  - Tests for `ono job list` commands
  - Activity history retrieval tests
  - Workflow execution status tests
- **New tests to create**:
  - Mock Temporal client tests
  - History pagination tests
  - Error handling for missing jobs
  - Recipe-to-Temporal mapping tests
- **Validation**:
  - Run `moon run test` in recipe-history
  - Verify all CLI history commands still work
  - Test with actual Temporal instance
  - Run `moon run test integration` in ono
- **Success criteria**:
  - All history query tests pass
  - CLI commands produce identical output
  - Proper error handling for edge cases
  - Clean abstraction over Temporal APIs

### Phase 5: Extract recipe-worker
1. Identify worker management code
2. Extract to recipe-worker:
   - Directory monitoring logic
   - Worker registration
   - Activity implementations
   - Workflow implementations
   - Recipe loading and validation
3. Move worker-related tests
4. Update ono to use recipe-worker

#### Test Migration & Evaluation:
- **Tests to identify**:
  - Worker lifecycle tests
  - Directory monitoring tests
  - Activity implementation tests
  - Workflow execution tests
  - Recipe loading tests
  - Search for: `*worker*_test.go`, `*activity*_test.go`, `*workflow*_test.go`
- **Tests to migrate**:
  - All worker registration tests
  - Directory watcher tests
  - Activity execution tests
  - Workflow orchestration tests
  - Recipe hot-reload tests
- **New tests to create**:
  - Concurrent worker management tests
  - Recipe file change detection tests
  - Worker failure recovery tests
  - Activity timeout tests
  - Memory leak tests for long-running workers
- **Validation**:
  - Run `moon run test` in recipe-worker
  - Test directory monitoring with file changes
  - Verify workers register with Temporal correctly
  - Test activity implementations execute properly
  - Run `moon run test integration` in ono
- **Success criteria**:
  - All worker tests pass
  - Directory monitoring detects changes
  - Workers handle recipe updates gracefully
  - No goroutine leaks
  - Integration tests show full workflow execution

### Phase 6: Refactor ono CLI
1. Remove all extracted logic
2. Update imports to use new libraries
3. Maintain existing CLI interface exactly
4. Ensure all commands delegate to appropriate libraries
5. Verify integration tests still pass

#### Test Migration & Evaluation:
- **Tests to identify**:
  - CLI command tests
  - Integration tests
  - End-to-end workflow tests
  - Command parsing tests
  - Search for: `*cli*_test.go`, `*cmd*_test.go`, `*integration_test.go`
- **Tests to keep in ono**:
  - ALL integration tests (these validate the full stack)
  - CLI command structure tests
  - Command flag parsing tests
  - Help text validation tests
- **Tests that should NOT exist**:
  - Unit tests for business logic (should be in components)
  - Tests for recipe parsing (in recipe-core)
  - Tests for worker management (in recipe-worker)
  - Tests for history queries (in recipe-history)
- **Validation**:
  - Run `moon run test integration` - must pass WITHOUT modification
  - Manually test all CLI commands
  - Verify help text is unchanged
  - Check command output format is identical
  - Performance benchmarks should not degrade
- **Success criteria**:
  - Ono contains ONLY CLI orchestration code
  - All integration tests pass without changes
  - CLI interface is 100% backward compatible
  - Performance is same or better
  - Code coverage remains high

## Testing Strategy

### Unit Test Migration
- Move tests with their related code
- Maintain test coverage levels
- Update import paths
- Ensure no business logic changes

### Integration Test Preservation
1. Keep ono integration tests in ono package
2. These tests validate the full stack works together
3. Should require NO modifications to pass
4. Acts as regression test suite

### Test Execution
```bash
# Each component
cd server/<component>
moon run test

# Integration tests
cd server/ono
moon run test integration
```

### Test Coverage Requirements
- **embeddedtemporal**: >90% coverage (critical infrastructure)
- **recipe-core**: 100% coverage (parsing must be bulletproof)
- **recipe-history**: >85% coverage (focus on query logic)
- **recipe-worker**: >90% coverage (execution reliability)
- **ono**: >80% coverage (CLI orchestration)

### Performance Testing
After each phase, run performance benchmarks:
```bash
# Baseline before refactoring
cd server/ono
go test -bench=. -benchmem > baseline.txt

# After each phase
go test -bench=. -benchmem > phase_N.txt
benchcmp baseline.txt phase_N.txt
```

## Migration Validation Checklist

### For Each Component:
- [ ] Go module initialized
- [ ] moon.yml configured with standard tasks
- [ ] Code extracted with minimal changes
- [ ] Unit tests moved and passing
- [ ] No circular dependencies
- [ ] Clean interfaces between components

### For Ono CLI:
- [ ] All commands work identically
- [ ] Integration tests pass without modification
- [ ] No business logic remains (only orchestration)
- [ ] Help text and CLI interface unchanged

### Overall:
- [ ] No changes to business logic
- [ ] All tests passing
- [ ] Code coverage maintained
- [ ] Clean dependency graph
- [ ] Each component independently usable

## Risk Mitigation

1. **Integration Test Failures**: 
   - Keep detailed mapping of moved code
   - Test after each extraction phase
   - Rollback capability for each phase
   - Create test inventory before starting

2. **Circular Dependencies**:
   - Define clear interfaces upfront
   - Use dependency injection where needed
   - recipe-core has no dependencies
   - Run `go mod graph` to detect cycles

3. **Missing Functionality**:
   - Comprehensive grep for imports
   - Run full test suite after each phase
   - Keep extraction commits atomic
   - Use coverage reports to find gaps

4. **Test Migration Risks**:
   - Document which tests belong to which component
   - Never delete a test without moving it
   - Run coverage comparison before/after
   - Keep integration tests as safety net

### Pre-Migration Test Inventory
Before starting, create a comprehensive test inventory:
```bash
# Generate test inventory
find server/ono -name "*_test.go" -exec grep -l "func Test" {} \; > test_inventory.txt

# For each test file, document:
# - Current location
# - Target component
# - Test purpose
# - Dependencies
```

## Success Criteria

1. All existing ono commands work exactly as before
2. All unit and integration tests pass without modification to test logic
3. Each component can be imported and used independently
4. Clean separation of concerns achieved
5. No performance degradation
6. Improved testability of individual components

## Future Benefits

- **Reusability**: Other applications can use embeddedtemporal or recipe-core
- **Testability**: Each component can be tested in isolation
- **Maintainability**: Clear boundaries and responsibilities
- **Flexibility**: Components can evolve independently
- **Documentation**: Each component has focused documentation