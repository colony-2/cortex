# CEL Custom Function Spec: json_parse

## Goal
Enable CEL templates to convert a JSON string into a structured value (map/list/primitive) so it can be used as a subtree in templates.

## Function Signature
- Name: `json_parse`
- Inputs: `string`
- Output: `dyn` (map/list/number/bool/string/null based on JSON)
- Errors: invalid JSON -> CEL error that fails resolution

## Behavior
- Accepts a JSON string and returns a CEL dynamic value.
- Whitespace is allowed; input must be valid JSON.
- If the input is not a string or is empty, return a CEL error.

## Integration Points
- Register the function when creating the CEL environment in `pkg/template/template_resolver.go`.
- Use `cel.Function` with `cel.Overload` (or `cel.MemberOverload` for method-style usage).
- Implementation should call `encoding/json.Unmarshal` into `interface{}`.
- Use `env.CELTypeAdapter().NativeToValue(...)` (or the active adapter) to convert the decoded value to CEL `dyn`.

## Example Usage
```yaml
inputs:
  op_config: "{{ json_parse(inputs.raw_config_json) }}"
```

## Error Messages
- Invalid JSON: `json_parse: invalid JSON: <error>`
- Non-string: `json_parse: expected string`

## Testing
Add unit tests (new file or `pkg/template/template_resolver_test.go`):
- parses object JSON into map
- parses array JSON into list
- invalid JSON returns error
- non-string returns error
