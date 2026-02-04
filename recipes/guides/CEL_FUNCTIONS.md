# CEL Helper Functions

## `cells()`
- Available in Cortex via the CEL function registry.
- Returns a list of maps, each with keys: `name`, `id`, `path`, `description`.
- Example: `cells()[0].name` yields the first cell's name; you can filter with standard CEL list ops, e.g. `cells().exists(c, c.name == "my-cell")`.

Notes:
- The function is zero-argument and pulls from the cell service at evaluation time.
- Output items are plain maps for portability; fields are `string`-typed.
