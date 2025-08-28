# Test Statements for recipe-core Go Project

## pkg/cel/cel.go

### CELExpr Struct Tests
- Test JSONSchema returns string type schema with correct description [pkg/cel/cel.go]
- Test InlineJSONSchema method executes without errors [pkg/cel/cel.go]
- Test String method returns original expression string [pkg/cel/cel.go]
- Test JSONSchemaExtend sets type to string and clears properties [pkg/cel/cel.go]
- Test AlwaysTrue returns true for empty string expression [pkg/cel/cel.go]
- Test AlwaysTrue returns true for case-insensitive "true" expression [pkg/cel/cel.go]
- Test AlwaysTrue returns false for non-empty, non-true expressions [pkg/cel/cel.go]

### NewCELExpr Constructor Tests
- Test NewCELExpr creates valid CELExpr with simple boolean expression [pkg/cel/cel.go]
- Test NewCELExpr creates valid CELExpr with complex expression [pkg/cel/cel.go]
- Test NewCELExpr returns error for invalid CEL syntax [pkg/cel/cel.go]
- Test NewCELExpr returns error for empty expression compilation [pkg/cel/cel.go]

### AsBool Method Tests
- Test AsBool returns true for AlwaysTrue expressions without evaluation [pkg/cel/cel.go]
- Test AsBool evaluates boolean expression and returns correct result [pkg/cel/cel.go]
- Test AsBool returns error when expression evaluates to non-boolean [pkg/cel/cel.go]
- Test AsBool returns error when expression evaluation fails [pkg/cel/cel.go]
- Test AsBool works with complex boolean expressions using inputs [pkg/cel/cel.go]

### Evaluate Method Tests
- Test Evaluate panics when program is nil [pkg/cel/cel.go]
- Test Evaluate returns correct value for string expressions [pkg/cel/cel.go]
- Test Evaluate returns correct value for numeric expressions [pkg/cel/cel.go]
- Test Evaluate returns correct value for boolean expressions [pkg/cel/cel.go]
- Test Evaluate handles map input access via inputs parameter [pkg/cel/cel.go]
- Test Evaluate returns error for invalid expression evaluation [pkg/cel/cel.go]

### YAML Marshaling Tests
- Test MarshalYAML returns expression string unchanged [pkg/cel/cel.go]
- Test UnmarshalYAML correctly parses valid expression string [pkg/cel/cel.go]
- Test UnmarshalYAML returns error for invalid YAML format [pkg/cel/cel.go]
- Test UnmarshalYAML returns error for invalid CEL expression [pkg/cel/cel.go]

### compile Function Tests
- Test compile creates valid program for simple expressions [pkg/cel/cel.go]
- Test compile creates valid program with inputs variable access [pkg/cel/cel.go]
- Test compile returns error for invalid expression syntax [pkg/cel/cel.go]
- Test compile returns error when CEL environment creation fails [pkg/cel/cel.go]
- Test compile returns error when expression parsing fails [pkg/cel/cel.go]
- Test compile returns error when expression checking fails [pkg/cel/cel.go]
- Test compile returns error when program creation fails [pkg/cel/cel.go]

### DynamicMapValue Tests
- Test Type method returns DynType [pkg/cel/cel.go]
- Test Value method returns underlying map data [pkg/cel/cel.go]
- Test ConvertToNative returns data when types are assignable [pkg/cel/cel.go]
- Test ConvertToNative returns error when types are incompatible [pkg/cel/cel.go]
- Test ConvertToType returns self for MapType and DynType [pkg/cel/cel.go]
- Test ConvertToType returns JSON string for StringType [pkg/cel/cel.go]
- Test ConvertToType returns error for unsupported types [pkg/cel/cel.go]
- Test Equal returns true for identical DynamicMapValue instances [pkg/cel/cel.go]
- Test Equal returns false for different DynamicMapValue instances [pkg/cel/cel.go]
- Test Equal returns false for non-DynamicMapValue types [pkg/cel/cel.go]
- Test Get returns correct value for existing string keys [pkg/cel/cel.go]
- Test Get returns NullValue for non-existent keys [pkg/cel/cel.go]
- Test Get returns error for non-string key types [pkg/cel/cel.go]
- Test Contains returns true for existing keys [pkg/cel/cel.go]
- Test Contains returns false for non-existent keys [pkg/cel/cel.go]
- Test Contains returns false for non-string key types [pkg/cel/cel.go]
- Test Size returns correct map length [pkg/cel/cel.go]
- Test Iterator returns valid mapIterator with correct keys [pkg/cel/cel.go]
- Test Iterator works with empty maps [pkg/cel/cel.go]

### convertToCELValue Function Tests
- Test convertToCELValue returns NullValue for nil input [pkg/cel/cel.go]
- Test convertToCELValue converts nested maps to DynamicMapValue [pkg/cel/cel.go]
- Test convertToCELValue converts slices to CEL dynamic lists [pkg/cel/cel.go]
- Test convertToCELValue converts string values correctly [pkg/cel/cel.go]
- Test convertToCELValue converts boolean values correctly [pkg/cel/cel.go]
- Test convertToCELValue converts various integer types correctly [pkg/cel/cel.go]
- Test convertToCELValue converts various unsigned integer types correctly [pkg/cel/cel.go]
- Test convertToCELValue converts float32 to double [pkg/cel/cel.go]
- Test convertToCELValue converts whole number float64 to int [pkg/cel/cel.go]
- Test convertToCELValue converts decimal float64 to double [pkg/cel/cel.go]
- Test convertToCELValue converts byte slices correctly [pkg/cel/cel.go]
- Test convertToCELValue panics for unsupported types [pkg/cel/cel.go]

### mapIterator Tests
- Test HasNext returns true when more elements exist [pkg/cel/cel.go]
- Test HasNext returns false when no more elements exist [pkg/cel/cel.go]
- Test Next returns correct key string values sequentially [pkg/cel/cel.go]
- Test Next returns error when no more elements available [pkg/cel/cel.go]
- Test Type returns IteratorType [pkg/cel/cel.go]
- Test Value returns iterator instance itself [pkg/cel/cel.go]
- Test ConvertToNative returns error for iterator conversion [pkg/cel/cel.go]
- Test ConvertToType returns self for IteratorType [pkg/cel/cel.go]
- Test ConvertToType returns error for non-iterator types [pkg/cel/cel.go]
- Test Equal always returns false for iterator comparisons [pkg/cel/cel.go]

## pkg/ops/ops.go

### Singleton Pattern Tests
- Test getInstance() returns same instance on multiple calls ensuring singleton behavior [pkg/ops/ops.go]
- Test getInstance() is thread-safe when called concurrently from multiple goroutines [pkg/ops/ops.go]
- Test once.Do() is called only once even with concurrent getInstance() calls [pkg/ops/ops.go]

### Register Function Tests
- Test Register() successfully stores single operation with correct name mapping [pkg/ops/ops.go]
- Test Register() successfully stores multiple operations in single call [pkg/ops/ops.go]
- Test Register() overwrites existing operation when registering same name [pkg/ops/ops.go]
- Test Register() with nil RegisterableOp panics or handles gracefully [pkg/ops/ops.go]
- Test Register() with empty ops slice does nothing [pkg/ops/ops.go]
- Test Register() is thread-safe when called concurrently [pkg/ops/ops.go]

### Get Function Tests
- Test Get() returns operation and true when operation exists [pkg/ops/ops.go]
- Test Get() returns nil and false when operation doesn't exist [pkg/ops/ops.go]
- Test Get() with empty string name returns nil and false [pkg/ops/ops.go]
- Test Get() correctly type casts stored value to RegisterableOp interface [pkg/ops/ops.go]
- Test Get() is thread-safe when called concurrently with Register() [pkg/ops/ops.go]

### List Function Tests
- Test List() returns empty slice when no operations registered [pkg/ops/ops.go]
- Test List() returns all registered operations in any order [pkg/ops/ops.go]
- Test List() returns slice with correct length matching Size() [pkg/ops/ops.go]
- Test List() creates new slice each call, not sharing internal state [pkg/ops/ops.go]
- Test List() correctly type casts all values to RegisterableOp [pkg/ops/ops.go]

### Clear Function Tests
- Test Clear() removes all operations making Size() return zero [pkg/ops/ops.go]
- Test Clear() on empty registry does nothing [pkg/ops/ops.go]
- Test Clear() followed by Get() returns false for previously registered ops [pkg/ops/ops.go]
- Test Clear() is thread-safe when called concurrently with other operations [pkg/ops/ops.go]

### Size Function Tests
- Test Size() returns zero for empty registry [pkg/ops/ops.go]
- Test Size() increases correctly after Register() calls [pkg/ops/ops.go]
- Test Size() returns zero after Clear() is called [pkg/ops/ops.go]
- Test Size() remains consistent when registering operation with same name [pkg/ops/ops.go]
- Test Size() is thread-safe and returns accurate count during concurrent operations [pkg/ops/ops.go]

### Integration and Edge Case Tests
- Test registry state persists across multiple function calls using same singleton [pkg/ops/ops.go]
- Test concurrent Register(), Get(), List(), Clear(), Size() operations don't corrupt state [pkg/ops/ops.go]
- Test operations with duplicate names overwrite correctly in all functions [pkg/ops/ops.go]
- Test memory usage doesn't grow indefinitely after many Register/Clear cycles [pkg/ops/ops.go]

## pkg/ops/registerable_op.go

### Constructor Functions Tests
- Test NewInlineOp creates op with inline handler and correct metadata [pkg/ops/registerable_op.go]
- Test NewActivityMappedOp creates op with activity handler and correct metadata [pkg/ops/registerable_op.go]
- Test NewActivityMappedOpWithManagement creates op with handler, metadata, and management service [pkg/ops/registerable_op.go]

### opSpecImpl Method Tests
- Test GetInputStruct returns correct zero-value instance of input type [pkg/ops/registerable_op.go]
- Test GetName returns metadata name field correctly [pkg/ops/registerable_op.go]
- Test GetManagementService returns configured management service or nil [pkg/ops/registerable_op.go]
- Test ExecuteAsActivity returns true when inlineHandler is nil, false otherwise [pkg/ops/registerable_op.go]
- Test GetMetadata returns complete OpMetadata struct [pkg/ops/registerable_op.go]

### Execute Method Tests
- Test Execute with valid input map successfully calls handler and returns output [pkg/ops/registerable_op.go]
- Test Execute panics when handler is nil (inline-only op) [pkg/ops/registerable_op.go]
- Test Execute returns error when input decoding fails [pkg/ops/registerable_op.go]
- Test Execute wraps handler errors with descriptive message [pkg/ops/registerable_op.go]
- Test Execute converts output struct to map using JSON tags [pkg/ops/registerable_op.go]

### ExecuteInline Method Tests
- Test ExecuteInline with valid input map successfully calls inline handler [pkg/ops/registerable_op.go]
- Test ExecuteInline panics when inlineHandler is nil (activity-only op) [pkg/ops/registerable_op.go]
- Test ExecuteInline returns error when input decoding fails [pkg/ops/registerable_op.go]
- Test ExecuteInline wraps inline handler errors with descriptive message [pkg/ops/registerable_op.go]
- Test ExecuteInline converts output struct to map using JSON tags [pkg/ops/registerable_op.go]

### Type Reflection Tests
- Test GetInputType returns correct input type from activity handler [pkg/ops/registerable_op.go]
- Test GetInputType returns correct input type from inline handler [pkg/ops/registerable_op.go]
- Test GetOutputType returns correct output type from activity handler [pkg/ops/registerable_op.go]
- Test GetOutputType returns correct output type from inline handler [pkg/ops/registerable_op.go]

### decodeWithJsonTags Function Tests
- Test decodeWithJsonTags successfully decodes valid map to struct with JSON tags [pkg/ops/registerable_op.go]
- Test decodeWithJsonTags returns error for incompatible data types [pkg/ops/registerable_op.go]
- Test decodeWithJsonTags handles empty input map correctly [pkg/ops/registerable_op.go]
- Test decodeWithJsonTags respects JSON tag names for field mapping [pkg/ops/registerable_op.go]

### Interface Implementation Tests
- Test isOpSpec method exists and satisfies RegisterableOp interface [pkg/ops/registerable_op.go]
- Test opSpecImpl implements all RegisterableOp interface methods [pkg/ops/registerable_op.go]

### Edge Cases and Error Conditions
- Test ops with complex nested input/output struct types [pkg/ops/registerable_op.go]
- Test ops with pointer types in input/output structs [pkg/ops/registerable_op.go]
- Test behavior with nil context in Execute methods [pkg/ops/registerable_op.go]
- Test concurrent execution of Execute and ExecuteInline methods [pkg/ops/registerable_op.go]

## pkg/recipe/hash.go

- Test NewHashComputer returns non-nil HashComputer instance [pkg/recipe/hash.go]
- Test ComputeRecipeHash with valid recipe returns consistent SHA-256 hash [pkg/recipe/hash.go]
- Test ComputeRecipeHash produces deterministic hash for identical recipes [pkg/recipe/hash.go]
- Test ComputeRecipeHash produces different hashes for different recipes [pkg/recipe/hash.go]
- Test ComputeRecipeHash handles nil recipe input gracefully [pkg/recipe/hash.go]
- Test ComputeRecipeHash fallback when JSON marshaling fails [pkg/recipe/hash.go]
- Test ComputeRecipeHash fallback uses recipe ID and version correctly [pkg/recipe/hash.go]
- Test ComputeRecipeHash returns valid hexadecimal string format [pkg/recipe/hash.go]
- Test ComputeRecipeHash handles recipe with empty metadata fields [pkg/recipe/hash.go]
- Test ComputeRecipeHash handles recipe with nil GetMetdata() method [pkg/recipe/hash.go]
- Test ComputeRecipeHash returns 64-character hex string (SHA-256 output) [pkg/recipe/hash.go]
- Test ComputeRecipeHash with recipe containing special characters in metadata [pkg/recipe/hash.go]
- Test ComputeRecipeHash performance with large recipe objects [pkg/recipe/hash.go]
- Test ComputeRecipeHash handles recipes with circular references gracefully [pkg/recipe/hash.go]
- Test ComputeRecipeHash order independence for JSON serialization [pkg/recipe/hash.go]

## pkg/recipe/node.go

### Node struct tests
- Test Node.JSONSchema() returns schema with correct reference "#/$defs/Node" [pkg/recipe/node.go]

### Node.UnmarshalYAML tests
- Test UnmarshalYAML creates NodeState when "state" key present in YAML [pkg/recipe/node.go]
- Test UnmarshalYAML creates NodeSequence when "sequence" key present in YAML [pkg/recipe/node.go]
- Test UnmarshalYAML creates NodeOp when "op" key present in YAML [pkg/recipe/node.go]
- Test UnmarshalYAML creates NodeShared when "shared" key present in YAML [pkg/recipe/node.go]
- Test UnmarshalYAML returns error when no recognized keys present [pkg/recipe/node.go]
- Test UnmarshalYAML handles invalid YAML decode gracefully [pkg/recipe/node.go]
- Test UnmarshalYAML handles second pass decode failure properly [pkg/recipe/node.go]
- Test UnmarshalYAML with multiple keys chooses correct node type priority [pkg/recipe/node.go]

### NodeShared tests
- Test NodeShared.isNode() implements NodeImpl interface correctly [pkg/recipe/node.go]
- Test NodeShared struct marshals/unmarshals "shared" field properly [pkg/recipe/node.go]

### NodeSequence tests
- Test NodeSequence.isNode() implements NodeImpl interface correctly [pkg/recipe/node.go]
- Test NodeSequence embeds NodeMetadata and SequenceData inline [pkg/recipe/node.go]

### NodeState tests
- Test NodeState.isNode() implements NodeImpl interface correctly [pkg/recipe/node.go]
- Test NodeState embeds NodeMetadata and StateData inline [pkg/recipe/node.go]

### NodeOp tests
- Test NodeOp.isNode() implements NodeImpl interface correctly [pkg/recipe/node.go]
- Test NodeOp.JSONSchema() generates schema from ops.List() correctly [pkg/recipe/node.go]
- Test NodeOp.JSONSchema() handles empty ops list gracefully [pkg/recipe/node.go]
- Test NodeOp embeds NodeMetadata and OpData inline [pkg/recipe/node.go]

### NodeMetadata tests
- Test NodeMetadata fields (ID, Desc, Timeout, Retry, When) marshal correctly [pkg/recipe/node.go]
- Test NodeMetadata with empty/nil values handles YAML omitempty tags [pkg/recipe/node.go]
- Test NodeMetadata CEL expression validation in When field [pkg/recipe/node.go]

### Integration tests
- Test complete YAML roundtrip for each node type preserves data [pkg/recipe/node.go]
- Test NodeList can contain mixed node types correctly [pkg/recipe/node.go]

## pkg/recipe/op.go

- Test OpData struct initialization with valid op and inputs fields [pkg/recipe/op.go]
- Test OpData struct initialization with empty op field [pkg/recipe/op.go]
- Test OpData struct initialization with nil inputs map [pkg/recipe/op.go]
- Test OpData struct initialization with empty inputs map [pkg/recipe/op.go]
- Test UnmarshalYAML with valid YAML node containing known op [pkg/recipe/op.go]
- Test UnmarshalYAML with invalid YAML node structure [pkg/recipe/op.go]
- Test UnmarshalYAML with unknown/unregistered op name [pkg/recipe/op.go]
- Test UnmarshalYAML with nil YAML node input [pkg/recipe/op.go]
- Test UnmarshalYAML with malformed YAML content [pkg/recipe/op.go]
- Test UnmarshalYAML when ops.Get returns false for op existence [pkg/recipe/op.go]
- Test UnmarshalYAML when yaml.Marshal fails on inputs [pkg/recipe/op.go]
- Test UnmarshalYAML when yaml.Unmarshal fails on concrete input type [pkg/recipe/op.go]
- Test UnmarshalYAML with valid inputs that match expected input struct [pkg/recipe/op.go]
- Test UnmarshalYAML with inputs that don't match expected input struct [pkg/recipe/op.go]
- Test UnmarshalYAML with various input data types (string, int, bool, slice) [pkg/recipe/op.go]
- Test UnmarshalYAML error handling returns proper error messages [pkg/recipe/op.go]
- Test type alias Alias prevents infinite recursion during unmarshaling [pkg/recipe/op.go]
- Test GetInputStruct integration with different op types [pkg/recipe/op.go]
- Test concurrent UnmarshalYAML calls with same OpData instance [pkg/recipe/op.go]
- Test OpData YAML tags are properly respected during marshaling [pkg/recipe/op.go]

## pkg/recipe/parser.go

### LoadRecipeFromString Function Tests
- Test LoadRecipeFromString with valid YAML data should return parsed Recipe and nil error [pkg/recipe/parser.go]
- Test LoadRecipeFromString with empty byte slice should return Recipe with zero values and nil error [pkg/recipe/parser.go]
- Test LoadRecipeFromString with invalid YAML syntax should return nil Recipe and unmarshal error [pkg/recipe/parser.go]
- Test LoadRecipeFromString with malformed YAML structure should return nil Recipe and type conversion error [pkg/recipe/parser.go]
- Test LoadRecipeFromString with nil byte slice should handle gracefully without panicking [pkg/recipe/parser.go]
- Test LoadRecipeFromString with valid RecipeState YAML should return Recipe with RecipeState implementation [pkg/recipe/parser.go]
- Test LoadRecipeFromString with valid RecipeSequence YAML should return Recipe with RecipeSequence implementation [pkg/recipe/parser.go]
- Test LoadRecipeFromString with valid RecipeOp YAML should return Recipe with RecipeOp implementation [pkg/recipe/parser.go]
- Test LoadRecipeFromString with mixed valid/invalid YAML fields should parse valid fields and ignore invalid ones [pkg/recipe/parser.go]
- Test LoadRecipeFromString with Unicode characters should parse correctly without encoding issues [pkg/recipe/parser.go]
- Test LoadRecipeFromString with very large YAML data should handle memory efficiently [pkg/recipe/parser.go]

### LoadRecipeFromReader Function Tests
- Test LoadRecipeFromReader with valid YAML reader should return parsed Recipe and nil error [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with empty reader should return Recipe with zero values and nil error [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with nil reader should return nil Recipe and appropriate error [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with invalid YAML syntax should return nil Recipe and wrapped decode error [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with io.Reader that returns read errors should propagate the error [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with string reader containing valid YAML should successfully parse Recipe [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with bytes reader containing malformed YAML should return decode error [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with file reader should parse YAML content from file correctly [pkg/recipe/parser.go]
- Test LoadRecipeFromReader error message should contain "failed to decode recipe" prefix [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with stream containing multiple YAML documents should parse first document only [pkg/recipe/parser.go]
- Test LoadRecipeFromReader with reader that closes mid-stream should handle EOF gracefully [pkg/recipe/parser.go]

### Integration and Edge Case Tests
- Test both functions with identical YAML content should return equivalent Recipe objects [pkg/recipe/parser.go]
- Test parsing Recipe with all supported RecipeImpl types should correctly set interface implementation [pkg/recipe/parser.go]
- Test parsing YAML with deeply nested structures should handle without stack overflow [pkg/recipe/parser.go]
- Test concurrent calls to both parsing functions should be thread-safe [pkg/recipe/parser.go]
- Test parsing YAML with special characters and escape sequences should preserve data integrity [pkg/recipe/parser.go]

## pkg/recipe/recipe.go

### Recipe Interface and Type Tests
- Test Recipe struct implements RecipeImpl interface correctly [pkg/recipe/recipe.go]
- Test RecipeSequence implements isRecipe method without errors [pkg/recipe/recipe.go]
- Test RecipeState implements isRecipe method without errors [pkg/recipe/recipe.go]
- Test RecipeOp implements isRecipe method without errors [pkg/recipe/recipe.go]

### GetMetadata Function Tests
- Test GetMetdata returns correct RecipeMetadata for RecipeState type [pkg/recipe/recipe.go]
- Test GetMetdata returns correct RecipeMetadata for RecipeSequence type [pkg/recipe/recipe.go]
- Test GetMetdata returns correct RecipeMetadata for RecipeOp type [pkg/recipe/recipe.go]
- Test GetMetdata panics when RecipeImpl is unknown/invalid type [pkg/recipe/recipe.go]
- Test GetMetdata panics when RecipeImpl is nil [pkg/recipe/recipe.go]

### YAML Marshaling Tests
- Test MarshalYAML returns RecipeImpl without modification [pkg/recipe/recipe.go]
- Test MarshalYAML works correctly with RecipeState implementation [pkg/recipe/recipe.go]
- Test MarshalYAML works correctly with RecipeSequence implementation [pkg/recipe/recipe.go]
- Test MarshalYAML works correctly with RecipeOp implementation [pkg/recipe/recipe.go]

### YAML Unmarshaling Tests
- Test UnmarshalYAML correctly identifies and creates RecipeState from state field [pkg/recipe/recipe.go]
- Test UnmarshalYAML correctly identifies and creates RecipeSequence from sequence field [pkg/recipe/recipe.go]
- Test UnmarshalYAML correctly identifies and creates RecipeOp from op field [pkg/recipe/recipe.go]
- Test UnmarshalYAML returns error when no valid recipe type found [pkg/recipe/recipe.go]
- Test UnmarshalYAML returns error when initial YAML decode fails [pkg/recipe/recipe.go]
- Test UnmarshalYAML returns error when concrete type decode fails [pkg/recipe/recipe.go]
- Test UnmarshalYAML handles empty YAML node gracefully [pkg/recipe/recipe.go]
- Test UnmarshalYAML handles malformed YAML structure [pkg/recipe/recipe.go]

### RecipeMetadata Tests
- Test RecipeMetadata struct fields are properly tagged for YAML [pkg/recipe/recipe.go]
- Test RecipeMetadata Defs field accepts map of string to Node [pkg/recipe/recipe.go]
- Test RecipeMetadata InputSchema field accepts map of string to InputSchema [pkg/recipe/recipe.go]
- Test RecipeMetadata omitempty tags work for optional fields [pkg/recipe/recipe.go]

### InputSchema Tests
- Test InputSchema struct marshals/unmarshals correctly with all fields [pkg/recipe/recipe.go]
- Test InputSchema Default field accepts string and number types [pkg/recipe/recipe.go]
- Test InputSchema omitempty tags work for optional fields [pkg/recipe/recipe.go]
- Test InputSchema Required field defaults to false when not specified [pkg/recipe/recipe.go]

### Integration Tests
- Test complete Recipe round-trip marshal/unmarshal preserves data [pkg/recipe/recipe.go]
- Test Recipe with complex nested structures marshals correctly [pkg/recipe/recipe.go]
- Test Recipe handles multiple recipe types in same document [pkg/recipe/recipe.go]

## pkg/recipe/schema.go

### GenerateSchemaString Function Tests
- Test GenerateSchemaString returns valid JSON schema string for Recipe type [pkg/recipe/schema.go]
- Test GenerateSchemaString includes Node definitions in schema output [pkg/recipe/schema.go]
- Test GenerateSchemaString handles empty ops.List() gracefully [pkg/recipe/schema.go]
- Test GenerateSchemaString returns error when getNodeSchema fails [pkg/recipe/schema.go]
- Test GenerateSchemaString returns error when json.MarshalIndent fails [pkg/recipe/schema.go]
- Test GenerateSchemaString creates definitions map when nil [pkg/recipe/schema.go]

### getNodeSchema Function Tests
- Test getNodeSchema generates schema with all ops from ops.List() [pkg/recipe/schema.go]
- Test getNodeSchema includes NodeSequence and NodeState in OneOf items [pkg/recipe/schema.go]
- Test getNodeSchema sets correct op name as const value [pkg/recipe/schema.go]
- Test getNodeSchema includes required fields op and inputs [pkg/recipe/schema.go]
- Test getNodeSchema returns error when cloneSchema fails [pkg/recipe/schema.go]
- Test getNodeSchema sets correct titles for sequence and state nodes [pkg/recipe/schema.go]
- Test getNodeSchema handles empty operations list [pkg/recipe/schema.go]

### cloneSchema Function Tests
- Test cloneSchema returns nil when input schema is nil [pkg/recipe/schema.go]
- Test cloneSchema creates deep copy of schema object [pkg/recipe/schema.go]
- Test cloneSchema returns error when json.Marshal fails [pkg/recipe/schema.go]
- Test cloneSchema returns error when json.Unmarshal fails [pkg/recipe/schema.go]
- Test cloneSchema preserves all schema properties correctly [pkg/recipe/schema.go]

### stripSchema Function Tests
- Test stripSchema removes version field from schema [pkg/recipe/schema.go]
- Test stripSchema returns same schema reference [pkg/recipe/schema.go]
- Test stripSchema handles nil schema input [pkg/recipe/schema.go]

### PrintSchema Function Tests
- Test PrintSchema prints schema without errors when GenerateSchemaString succeeds [pkg/recipe/schema.go]
- Test PrintSchema calls log.Fatal when GenerateSchemaString returns error [pkg/recipe/schema.go]

### oneOfSchema Function Tests
- Test oneOfSchema creates schema with OneOf containing all input types [pkg/recipe/schema.go]
- Test oneOfSchema sets correct title from name parameter [pkg/recipe/schema.go]
- Test oneOfSchema handles empty type list [pkg/recipe/schema.go]
- Test oneOfSchema sets reflector.Anonymous correctly during execution [pkg/recipe/schema.go]
- Test oneOfSchema strips schema version from reflected types [pkg/recipe/schema.go]

### Recipe JSONSchema Method Tests
- Test Recipe.JSONSchema returns OneOf schema with RecipeState, RecipeSequence, RecipeOp [pkg/recipe/schema.go]
- Test Recipe.JSONSchema sets correct title as "recipe" [pkg/recipe/schema.go]

### Global Variables Tests
- Test sharedReflector has correct configuration values set [pkg/recipe/schema.go]
- Test sharedReflector uses "yaml" as FieldNameTag [pkg/recipe/schema.go]
- Test sharedReflector allows additional properties and expanded structs [pkg/recipe/schema.go]

### Integration Tests
- Test complete schema generation produces valid JSON schema format [pkg/recipe/schema.go]
- Test schema includes all registered operations from ops package [pkg/recipe/schema.go]
- Test generated schema validates against expected Recipe structure [pkg/recipe/schema.go]

## pkg/recipe/sequence.go

- Test SequenceData struct creation with all fields populated successfully [pkg/recipe/sequence.go]
- Test SequenceData struct creation with empty/nil values for all fields [pkg/recipe/sequence.go]
- Test YAML marshaling of SequenceData with complete data produces correct output [pkg/recipe/sequence.go]
- Test YAML unmarshaling of valid YAML into SequenceData struct successfully [pkg/recipe/sequence.go]
- Test YAML unmarshaling with malformed YAML returns appropriate error [pkg/recipe/sequence.go]
- Test YAML unmarshaling with missing sequence field handles gracefully [pkg/recipe/sequence.go]
- Test YAML unmarshaling with missing inputs field handles gracefully [pkg/recipe/sequence.go]
- Test YAML unmarshaling with missing outputs field handles gracefully [pkg/recipe/sequence.go]
- Test SequenceData with omitempty tag excludes empty fields from YAML output [pkg/recipe/sequence.go]
- Test SequenceData field validation when Sequence contains invalid NodeList [pkg/recipe/sequence.go]
- Test SequenceData field validation when Inputs contains invalid InputMap [pkg/recipe/sequence.go]
- Test SequenceData field validation when Outputs contains invalid OutputMap [pkg/recipe/sequence.go]
- Test SequenceData struct equality comparison between identical instances [pkg/recipe/sequence.go]
- Test SequenceData struct deep copy preserves all field values correctly [pkg/recipe/sequence.go]
- Test SequenceData zero value initialization has expected default field values [pkg/recipe/sequence.go]

## pkg/recipe/shared.go

### Duration Type Tests
- Test Duration.JSONSchema() returns correct schema with string type, title, and description [pkg/recipe/shared.go]
- Test Duration.MarshalYAML() converts 1 second duration to "1s" string [pkg/recipe/shared.go]
- Test Duration.MarshalYAML() converts 500 milliseconds to "500ms" string [pkg/recipe/shared.go]
- Test Duration.MarshalYAML() converts zero duration to "0s" string [pkg/recipe/shared.go]
- Test Duration.UnmarshalYAML() successfully parses "1s" into 1 second duration [pkg/recipe/shared.go]
- Test Duration.UnmarshalYAML() successfully parses "500ms" into 500 millisecond duration [pkg/recipe/shared.go]
- Test Duration.UnmarshalYAML() successfully parses "2m30s" into complex duration [pkg/recipe/shared.go]
- Test Duration.UnmarshalYAML() returns error for invalid duration string "invalid" [pkg/recipe/shared.go]
- Test Duration.UnmarshalYAML() returns error for non-string YAML input [pkg/recipe/shared.go]
- Test Duration.UnmarshalYAML() returns error with proper wrapping for parse failures [pkg/recipe/shared.go]
- Test Duration.ToDuration() converts Duration back to standard time.Duration correctly [pkg/recipe/shared.go]
- Test Duration.String() returns same format as underlying time.Duration string representation [pkg/recipe/shared.go]
- Test Duration round-trip: marshal to YAML then unmarshal produces same value [pkg/recipe/shared.go]

### RetryPolicy Type Tests
- Test RetryPolicy struct fields can be properly serialized to YAML with omitempty tags [pkg/recipe/shared.go]
- Test RetryPolicy with zero values omits all fields in YAML serialization [pkg/recipe/shared.go]
- Test RetryPolicy with all fields populated serializes all values correctly [pkg/recipe/shared.go]
- Test RetryPolicy NonRetryableErrorTypes slice handles empty, single, and multiple error types [pkg/recipe/shared.go]

### InputMap and OutputMap Type Tests
- Test InputMap can store and retrieve string keys with various interface{} values [pkg/recipe/shared.go]
- Test OutputMap can store and retrieve string keys with various interface{} values [pkg/recipe/shared.go]
- Test InputMap handles nil values, complex nested structures, and type assertions [pkg/recipe/shared.go]
- Test OutputMap handles nil values, complex nested structures, and type assertions [pkg/recipe/shared.go]

## pkg/recipe/state.go

### StateMap Tests
- Test StateMap struct creation with valid initial state and states map [pkg/recipe/state.go]
- Test StateMap with empty initial field should handle gracefully [pkg/recipe/state.go]
- Test StateMap with nil states map should not panic [pkg/recipe/state.go]
- Test StateMap YAML unmarshaling with inline states works correctly [pkg/recipe/state.go]
- Test StateMap YAML marshaling preserves initial and inline states structure [pkg/recipe/state.go]

### State Tests
- Test State struct creation with valid Node and SingleStateMetadata [pkg/recipe/state.go]
- Test State with embedded Node fields are accessible [pkg/recipe/state.go]
- Test State with embedded SingleStateMetadata fields are accessible [pkg/recipe/state.go]
- Test State YAML unmarshaling with inline Node and metadata [pkg/recipe/state.go]
- Test State YAML marshaling preserves inline structure [pkg/recipe/state.go]

### SingleStateMetadata Tests
- Test SingleStateMetadata with nil Error and Transitions fields [pkg/recipe/state.go]
- Test SingleStateMetadata with valid error string pointer [pkg/recipe/state.go]
- Test SingleStateMetadata with empty transitions slice pointer [pkg/recipe/state.go]
- Test SingleStateMetadata JSON marshaling omits empty fields [pkg/recipe/state.go]
- Test SingleStateMetadata JSON unmarshaling handles optional fields [pkg/recipe/state.go]

### Transition Tests
- Test Transition creation with valid To field and CEL expression [pkg/recipe/state.go]
- Test Transition with empty To field should be invalid [pkg/recipe/state.go]
- Test Transition with nil When CEL expression works correctly [pkg/recipe/state.go]
- Test Transition YAML unmarshaling with optional When field [pkg/recipe/state.go]
- Test Transition YAML marshaling omits empty When field [pkg/recipe/state.go]

### StateData Tests
- Test StateData creation with all optional fields nil [pkg/recipe/state.go]
- Test StateData with valid States, Inputs, and Outputs [pkg/recipe/state.go]
- Test StateData YAML unmarshaling omits empty optional fields [pkg/recipe/state.go]
- Test StateData YAML marshaling preserves non-nil fields [pkg/recipe/state.go]
- Test StateData with empty InputMap and OutputMap maps [pkg/recipe/state.go]

### Integration Tests
- Test complete state machine configuration with multiple states and transitions [pkg/recipe/state.go]
- Test state machine with circular transition references [pkg/recipe/state.go]
- Test state machine with invalid initial state reference [pkg/recipe/state.go]
- Test complex YAML document with nested states and CEL expressions [pkg/recipe/state.go]

## pkg/recipe/types.go

### RecipeFile Tests
- Test RecipeFile creation with valid fields populates all properties correctly [pkg/recipe/types.go]
- Test RecipeFile with empty ID, Version, or Description handles empty strings appropriately [pkg/recipe/types.go]
- Test RecipeFile Hash field accepts valid hash strings and rejects invalid formats [pkg/recipe/types.go]
- Test RecipeFile LastModified field stores and retrieves time.Time values accurately [pkg/recipe/types.go]
- Test RecipeFile WorkerStatus field accepts all valid WorkerStatus constants [pkg/recipe/types.go]

### WorkerStatus Tests
- Test WorkerStatus constants match expected string values exactly [pkg/recipe/types.go]
- Test WorkerStatus type validation accepts only defined constant values [pkg/recipe/types.go]
- Test WorkerStatus string conversion returns correct string representation [pkg/recipe/types.go]

### Job Tests
- Test Job creation with all required fields initializes properly [pkg/recipe/types.go]
- Test Job with nil EndTime pointer handles unfinished jobs correctly [pkg/recipe/types.go]
- Test Job Input and Inputs alias fields maintain data consistency [pkg/recipe/types.go]
- Test Job Output and Outputs alias fields maintain data consistency [pkg/recipe/types.go]
- Test Job Duration calculation with nil pointer handles ongoing jobs [pkg/recipe/types.go]
- Test Job Activities slice handles empty and populated activity lists [pkg/recipe/types.go]
- Test Job with empty or invalid RecipeName and RecipeVersion handles edge cases [pkg/recipe/types.go]
- Test Job Error field stores and retrieves error messages correctly [pkg/recipe/types.go]

### WorkflowExecutionInfo Tests
- Test WorkflowExecutionInfo creation with valid WorkflowID and RunID [pkg/recipe/types.go]
- Test WorkflowExecutionInfo with empty WorkflowID or RunID handles edge cases [pkg/recipe/types.go]

### JobStatus Tests
- Test JobStatus constants match expected string values exactly [pkg/recipe/types.go]
- Test JobStatus type validation accepts only defined constant values [pkg/recipe/types.go]
- Test JobStatus string conversion returns correct string representation [pkg/recipe/types.go]

### ActivityExecution Tests
- Test ActivityExecution creation with all fields populates correctly [pkg/recipe/types.go]
- Test ActivityExecution with nil Duration pointer handles ongoing activities [pkg/recipe/types.go]
- Test ActivityExecution Result map handles empty and populated data [pkg/recipe/types.go]
- Test ActivityExecution Error field stores error messages properly [pkg/recipe/types.go]
- Test ActivityExecution Attempt counter tracks retry attempts correctly [pkg/recipe/types.go]
- Test ActivityExecution with empty Name or ActivityType handles edge cases [pkg/recipe/types.go]

### ActivityStatus Tests
- Test ActivityStatus constants match expected string values exactly [pkg/recipe/types.go]
- Test ActivityStatus type validation accepts only defined constant values [pkg/recipe/types.go]
- Test ActivityStatus string conversion returns correct string representation [pkg/recipe/types.go]

### RecipeManifest Tests
- Test RecipeManifest YAML unmarshaling with valid recipe.yaml structure [pkg/recipe/types.go]
- Test RecipeManifest with missing required fields handles parsing errors [pkg/recipe/types.go]
- Test RecipeManifest Files struct populates workflow, activities, agents paths correctly [pkg/recipe/types.go]
- Test RecipeManifest with empty or invalid YAML tags handles edge cases [pkg/recipe/types.go]

### RecipeFilter Tests
- Test RecipeFilter with nil Status pointer handles all-status filtering [pkg/recipe/types.go]
- Test RecipeFilter with specific WorkerStatus filters correctly [pkg/recipe/types.go]
- Test RecipeFilter IncludeRemoved boolean controls removed recipe visibility [pkg/recipe/types.go]

### JobFilter Tests
- Test JobFilter with empty RecipeName handles all-recipe filtering [pkg/recipe/types.go]
- Test JobFilter with nil Status pointer handles all-status filtering [pkg/recipe/types.go]
- Test JobFilter with nil StartTime and EndTime handles unbounded time filtering [pkg/recipe/types.go]
- Test JobFilter Limit field enforces result count restrictions properly [pkg/recipe/types.go]
- Test JobFilter with zero or negative Limit handles edge cases [pkg/recipe/types.go]

## pkg/recipe/validate.go

### Happy Path Tests
- Test Validate with valid YAML recipe returns nil error [pkg/recipe/validate.go]
- Test Validate with valid JSON recipe returns nil error [pkg/recipe/validate.go]
- Test Validate with complex nested recipe structure passes validation [pkg/recipe/validate.go]

### Schema Generation Error Tests
- Test Validate returns error when GenerateSchemaString fails [pkg/recipe/validate.go]
- Test Validate handles schema generation timeout gracefully [pkg/recipe/validate.go]

### JSON Schema Compilation Tests
- Test Validate returns error when schema JSON is malformed [pkg/recipe/validate.go]
- Test Validate returns error when schema cannot be added to compiler [pkg/recipe/validate.go]
- Test Validate returns error when schema compilation fails [pkg/recipe/validate.go]
- Test Validate uses correct Draft2020 schema version [pkg/recipe/validate.go]
- Test Validate enables format, vocab, and content assertions [pkg/recipe/validate.go]

### YAML Parsing Error Tests
- Test Validate returns error for invalid YAML syntax [pkg/recipe/validate.go]
- Test Validate returns error for malformed YAML structure [pkg/recipe/validate.go]
- Test Validate handles empty recipe string gracefully [pkg/recipe/validate.go]
- Test Validate handles YAML with invalid UTF-8 encoding [pkg/recipe/validate.go]

### Schema Validation Error Tests
- Test Validate returns error when recipe missing required fields [pkg/recipe/validate.go]
- Test Validate returns error when recipe has invalid field types [pkg/recipe/validate.go]
- Test Validate returns error when recipe has unknown properties [pkg/recipe/validate.go]
- Test Validate returns detailed validation errors for multiple issues [pkg/recipe/validate.go]

### Edge Cases
- Test Validate handles extremely large recipe files [pkg/recipe/validate.go]
- Test Validate handles recipe with special characters and unicode [pkg/recipe/validate.go]
- Test Validate handles recipe with null values appropriately [pkg/recipe/validate.go]
- Test Validate handles recipe with deeply nested structures [pkg/recipe/validate.go]

### Integration Tests
- Test Validate works correctly with actual recipe examples [pkg/recipe/validate.go]
- Test Validate error messages are user-friendly and actionable [pkg/recipe/validate.go]