# Test Statements for recipe-core Go Project

## Test Statement Rules
- **MUST** limit test statements to 30 words or less
- **MUST** include all affected filenames in the test statement when covering multiple files
- **MUST** use business/expectation language rather than implementation details  
- **MUST** include both positive and negative test cases for critical functionality
- **MUST** focus on integration points and avoid testing trivial getters/setters

## pkg/cel/cel.go

### Expression Creation and Validation
- Business rules compile successfully with valid CEL syntax [pkg/cel/cel.go]
- Invalid business rule syntax fails compilation with descriptive error [pkg/cel/cel.go] 
- Empty expression defaults to always-true condition for workflows [pkg/cel/cel.go]
- Complex conditional expressions with nested logic compile correctly [pkg/cel/cel.go]

### Expression Evaluation  
- Boolean conditions evaluate correctly for workflow branching decisions [pkg/cel/cel.go]
- String expressions produce expected text output for templates [pkg/cel/cel.go]
- Numeric calculations return accurate results for metrics [pkg/cel/cel.go]
- Expression fails gracefully when accessing undefined variables [pkg/cel/cel.go]
- Type mismatches in expressions produce clear error messages [pkg/cel/cel.go]

### Data Access and Transformation
- Expressions access nested input data for decision making [pkg/cel/cel.go]
- Map data converts to CEL values preserving all types [pkg/cel/cel.go]
- Null values in data handled without causing evaluation errors [pkg/cel/cel.go]
- Large datasets process efficiently without memory issues [pkg/cel/cel.go]
- Circular references in data structures detected and prevented [pkg/cel/cel.go]

### YAML Integration
- Expression configurations persist correctly through YAML serialization [pkg/cel/cel.go]
- Invalid CEL expressions in YAML fail with clear error messages [pkg/cel/cel.go]
- Complex expressions with special characters serialize safely [pkg/cel/cel.go]

### Runtime Safety
- Expressions handle missing data gracefully without crashes [pkg/cel/cel.go]
- Type conversions between CEL and Go preserve data integrity [pkg/cel/cel.go]
- Iterator operations complete without memory leaks [pkg/cel/cel.go]
- Unsupported data types fail fast with clear errors [pkg/cel/cel.go]

## pkg/ops/ops.go

### Operation Registry Management  
- Operations register globally for workflow access [pkg/ops/ops.go]
- Duplicate operation names replace existing registrations [pkg/ops/ops.go]
- Registry lookups find operations by name correctly [pkg/ops/ops.go]
- Missing operations return clear not-found indication [pkg/ops/ops.go]
- Registry clears all operations for test isolation [pkg/ops/ops.go]

### Thread Safety and Performance
- Concurrent operation registrations maintain data integrity [pkg/ops/ops.go]
- Registry handles high-volume lookups without degradation [pkg/ops/ops.go]
- Memory remains stable through registration/clear cycles [pkg/ops/ops.go]
- Singleton instance ensures global consistency [pkg/ops/ops.go]

## pkg/ops/registerable_op.go

### Operation Execution
- Inline operations execute synchronously within workflows [pkg/ops/registerable_op.go]
- Activity operations delegate to external workers correctly [pkg/ops/registerable_op.go]
- Operations with management services integrate properly [pkg/ops/registerable_op.go]
- Invalid input data fails operation with clear errors [pkg/ops/registerable_op.go]
- Operation errors include context for debugging [pkg/ops/registerable_op.go]

### Data Transformation
- JSON-tagged structs decode from input maps correctly [pkg/ops/registerable_op.go]
- Output data converts to maps preserving field names [pkg/ops/registerable_op.go]
- Complex nested structures transform without data loss [pkg/ops/registerable_op.go]
- Type mismatches produce helpful error messages [pkg/ops/registerable_op.go]

### Concurrency and Safety
- Concurrent operation executions maintain isolation [pkg/ops/registerable_op.go]
- Missing handlers fail fast with clear panic messages [pkg/ops/registerable_op.go]
- Nil contexts handled gracefully in operations [pkg/ops/registerable_op.go]

## pkg/recipe/hash.go

### Recipe Fingerprinting
- Identical recipes produce identical hashes for caching [pkg/recipe/hash.go]
- Modified recipes generate different hashes for versioning [pkg/recipe/hash.go]
- Hash computation remains deterministic across runs [pkg/recipe/hash.go]
- Large recipe structures hash efficiently without delays [pkg/recipe/hash.go]
- Circular references fail gracefully with fallback hashing [pkg/recipe/hash.go]
- Special characters in metadata hash correctly [pkg/recipe/hash.go]
- Empty or nil recipes handle without crashes [pkg/recipe/hash.go]

## pkg/recipe/node.go

### Node Type Resolution
- YAML with state key creates state machine node [pkg/recipe/node.go]
- YAML with sequence key creates sequential workflow node [pkg/recipe/node.go]
- YAML with op key creates operation execution node [pkg/recipe/node.go]
- YAML with shared key creates reusable component reference [pkg/recipe/node.go]
- Invalid YAML structure fails with helpful error message [pkg/recipe/node.go]
- Multiple node type keys resolve with correct precedence [pkg/recipe/node.go]

### Node Metadata and Conditions
- Conditional execution via When expressions works correctly [pkg/recipe/node.go]
- Timeout and retry configurations apply to node execution [pkg/recipe/node.go]
- Node descriptions provide runtime documentation [pkg/recipe/node.go]
- Empty metadata fields omit from serialization [pkg/recipe/node.go]

### Schema Generation
- Node schemas include all registered operations dynamically [pkg/recipe/node.go]
- Schema validation catches invalid node configurations [pkg/recipe/node.go]
- Mixed node type lists serialize and deserialize correctly [pkg/recipe/node.go]

## pkg/recipe/op.go

### Operation Configuration
- Known operations load from YAML with correct inputs [pkg/recipe/op.go]
- Unknown operation names fail with clear error [pkg/recipe/op.go]
- Operation inputs validate against expected schema [pkg/recipe/op.go]
- Invalid input types produce helpful validation errors [pkg/recipe/op.go]
- Empty or missing inputs use operation defaults [pkg/recipe/op.go]

### YAML Integration
- Complex input structures unmarshal correctly [pkg/recipe/op.go]
- Concurrent YAML parsing maintains data integrity [pkg/recipe/op.go]
- Malformed YAML fails fast with parse errors [pkg/recipe/op.go]
- Recursion prevention protects against infinite loops [pkg/recipe/op.go]

## pkg/recipe/parser.go

### Recipe Loading
- Valid YAML recipes load successfully from strings [pkg/recipe/parser.go]
- Valid YAML recipes load successfully from readers [pkg/recipe/parser.go]
- State machine recipes parse with correct structure [pkg/recipe/parser.go]
- Sequential workflow recipes parse with correct structure [pkg/recipe/parser.go]
- Single operation recipes parse with correct structure [pkg/recipe/parser.go]

### Error Handling
- Invalid YAML syntax fails with parse error details [pkg/recipe/parser.go]
- Malformed recipe structure fails with type errors [pkg/recipe/parser.go]
- IO errors from readers propagate correctly [pkg/recipe/parser.go]
- Nil or empty inputs handle gracefully [pkg/recipe/parser.go]
- Unicode and special characters preserve correctly [pkg/recipe/parser.go]

### Performance and Safety
- Large recipe files parse without memory issues [pkg/recipe/parser.go]
- Concurrent parsing operations remain thread-safe [pkg/recipe/parser.go]
- Deeply nested structures parse without stack overflow [pkg/recipe/parser.go]
- Multiple YAML documents parse first only [pkg/recipe/parser.go]

## pkg/recipe/recipe.go

### Recipe Type Management
- State machine recipes retrieve metadata correctly [pkg/recipe/recipe.go]
- Sequential workflow recipes retrieve metadata correctly [pkg/recipe/recipe.go]
- Single operation recipes retrieve metadata correctly [pkg/recipe/recipe.go]
- Unknown recipe types fail fast with panic [pkg/recipe/recipe.go]
- Nil recipe implementations fail with clear panic [pkg/recipe/recipe.go]

### Recipe Serialization
- Recipes serialize to YAML preserving all fields [pkg/recipe/recipe.go]
- Recipes deserialize from YAML with type detection [pkg/recipe/recipe.go]
- Missing recipe type in YAML fails with error [pkg/recipe/recipe.go]
- Malformed YAML structure fails during parsing [pkg/recipe/recipe.go]

### Metadata and Schema
- Shared node definitions accessible via metadata [pkg/recipe/recipe.go]
- Input schemas define parameter validation rules [pkg/recipe/recipe.go]
- Optional metadata fields omit when empty [pkg/recipe/recipe.go]
- Complex nested recipes round-trip without data loss [pkg/recipe/recipe.go]

## pkg/recipe/schema.go

### Schema Generation
- Recipe schemas include all registered operations dynamically [pkg/recipe/schema.go]
- Generated schemas validate recipe YAML correctly [pkg/recipe/schema.go]
- Schema includes node type definitions for validation [pkg/recipe/schema.go]
- Empty operation registry handles gracefully [pkg/recipe/schema.go]
- Schema generation errors provide helpful context [pkg/recipe/schema.go]

### Schema Structure
- OneOf schemas allow multiple recipe types [pkg/recipe/schema.go]
- Operation schemas include required input fields [pkg/recipe/schema.go]
- YAML field tags reflect in generated schemas [pkg/recipe/schema.go]
- Deep schema copies preserve all properties [pkg/recipe/schema.go]
- Schema versioning stripped for compatibility [pkg/recipe/schema.go]

## pkg/recipe/sequence.go

### Sequential Workflow Execution
- Sequences execute nodes in defined order [pkg/recipe/sequence.go]
- Input parameters pass correctly to first node [pkg/recipe/sequence.go]
- Output values propagate from last node [pkg/recipe/sequence.go]
- Empty sequences handle without errors [pkg/recipe/sequence.go]
- Invalid nodes in sequence fail validation [pkg/recipe/sequence.go]

### YAML Configuration
- Complete sequences serialize to YAML correctly [pkg/recipe/sequence.go]
- Sequences deserialize from YAML preserving order [pkg/recipe/sequence.go]
- Missing optional fields use defaults [pkg/recipe/sequence.go]
- Malformed YAML fails with parse errors [pkg/recipe/sequence.go]

## pkg/recipe/shared.go

### Duration Configuration
- Human-readable durations parse correctly (1s, 500ms, 2m30s) [pkg/recipe/shared.go]
- Invalid duration strings fail with clear errors [pkg/recipe/shared.go]
- Duration values round-trip through YAML correctly [pkg/recipe/shared.go]
- Zero durations serialize as "0s" consistently [pkg/recipe/shared.go]

### Retry Policy
- Retry configurations serialize with only set fields [pkg/recipe/shared.go]
- Non-retryable error types configure workflow behavior [pkg/recipe/shared.go]
- Empty retry policies omit from YAML output [pkg/recipe/shared.go]

### Data Maps
- Input maps accept heterogeneous parameter types [pkg/recipe/shared.go]
- Output maps capture varied result types [pkg/recipe/shared.go]
- Nested data structures preserve through maps [pkg/recipe/shared.go]
- Nil values in maps handle gracefully [pkg/recipe/shared.go]

## pkg/recipe/state.go

### State Machine Configuration
- State machines define initial state correctly [pkg/recipe/state.go]
- States contain nodes for execution logic [pkg/recipe/state.go]
- Transitions connect states with conditions [pkg/recipe/state.go]
- Error states handle failure scenarios [pkg/recipe/state.go]
- Missing initial state fails validation [pkg/recipe/state.go]

### State Transitions
- Conditional transitions evaluate CEL expressions [pkg/recipe/state.go]
- Unconditional transitions execute without When clause [pkg/recipe/state.go]
- Empty target state fails transition validation [pkg/recipe/state.go]
- Circular state references execute without infinite loops [pkg/recipe/state.go]

### YAML State Machines
- Complete state machines serialize to YAML [pkg/recipe/state.go]
- State machines deserialize preserving all transitions [pkg/recipe/state.go]
- Optional metadata fields omit when empty [pkg/recipe/state.go]
- Complex nested states parse correctly [pkg/recipe/state.go]

## pkg/recipe/types.go

### Recipe Management
- Recipe files track version and modification time [pkg/recipe/types.go]
- Recipe hashes provide content fingerprinting [pkg/recipe/types.go]
- Worker status indicates recipe availability [pkg/recipe/types.go]
- Recipe filters query by status and visibility [pkg/recipe/types.go]

### Job Execution Tracking
- Jobs capture workflow execution details [pkg/recipe/types.go]
- Running jobs show nil end time [pkg/recipe/types.go]
- Completed jobs calculate duration correctly [pkg/recipe/types.go]
- Failed jobs store error messages [pkg/recipe/types.go]
- Job activities track individual operation execution [pkg/recipe/types.go]

### Activity Monitoring
- Activities record execution attempts and retries [pkg/recipe/types.go]
- Activity results capture operation outputs [pkg/recipe/types.go]
- Failed activities preserve error details [pkg/recipe/types.go]
- Running activities show incomplete duration [pkg/recipe/types.go]

### Manifest and Filtering
- Recipe manifests define deployment structure [pkg/recipe/types.go]
- Missing manifest fields fail with errors [pkg/recipe/types.go]
- Job filters support time-based queries [pkg/recipe/types.go]
- Result limits prevent unbounded queries [pkg/recipe/types.go]

## pkg/recipe/validate.go

### Recipe Validation Success
- Valid YAML recipes pass schema validation [pkg/recipe/validate.go]
- Valid JSON recipes pass schema validation [pkg/recipe/validate.go]
- Complex nested workflows validate correctly [pkg/recipe/validate.go]

### Validation Failures
- Invalid YAML syntax fails with parse errors [pkg/recipe/validate.go]
- Missing required fields fail with specific errors [pkg/recipe/validate.go]
- Invalid field types fail with type errors [pkg/recipe/validate.go]
- Unknown properties fail with schema violations [pkg/recipe/validate.go]
- Multiple validation errors report all issues [pkg/recipe/validate.go]

### Schema Integration
- Schema generation failures propagate errors [pkg/recipe/validate.go]
- Malformed schemas fail compilation [pkg/recipe/validate.go]
- Draft2020 schema features validate correctly [pkg/recipe/validate.go]

### Edge Cases and Performance
- Large recipe files validate without timeout [pkg/recipe/validate.go]
- Unicode and special characters validate correctly [pkg/recipe/validate.go]
- Deeply nested structures validate without overflow [pkg/recipe/validate.go]
- Empty recipes handle gracefully [pkg/recipe/validate.go]
- Error messages provide actionable feedback [pkg/recipe/validate.go]

## pkg/recipe/visitor.go, walker.go, base_visitor.go

### Tree Traversal
- Walker visits all nodes in depth-first order [pkg/recipe/walker.go, visitor.go]
- Sequences traverse children nodes in order [pkg/recipe/walker.go, visitor.go]
- State machines traverse all state nodes [pkg/recipe/walker.go, visitor.go]
- Path tracking maintains correct node location [pkg/recipe/walker.go, visitor.go]
- Visitor can skip traversing children when needed [pkg/recipe/visitor.go, base_visitor.go]

### Node Transformation
- Shared nodes resolve to actual definitions [pkg/recipe/visitor.go, walker.go]
- Node replacements preserve metadata fields [pkg/recipe/visitor.go, base_visitor.go]
- Transformed trees maintain structural integrity [pkg/recipe/walker.go, visitor.go]
- Invalid node types fail with clear errors [pkg/recipe/walker.go]

### Shared Node Resolution
- Shared nodes expand from recipe definitions [pkg/recipe/visitor.go, walker.go]
- Missing shared node definitions fail with error [pkg/recipe/visitor.go]
- Circular shared references detect and fail [pkg/recipe/visitor.go]
- Nested shared nodes resolve recursively [pkg/recipe/visitor.go, walker.go]
- Deep copies prevent definition aliasing [pkg/recipe/visitor.go]

### Visitor Patterns
- BaseVisitor provides default pass-through behavior [pkg/recipe/base_visitor.go]
- Custom visitors override specific node types only [pkg/recipe/visitor.go, base_visitor.go]
- Walker sets visitor reference for traversal [pkg/recipe/walker.go, base_visitor.go]
- Embedded BaseVisitor inherits default behavior [pkg/recipe/base_visitor.go, visitor.go]

### Error Handling
- Unknown node types return descriptive errors [pkg/recipe/walker.go]
- Visitor errors propagate with path context [pkg/recipe/walker.go, visitor.go]
- Nil recipe implementations handle gracefully [pkg/recipe/walker.go]
- Malformed node structures fail validation [pkg/recipe/walker.go, visitor.go]