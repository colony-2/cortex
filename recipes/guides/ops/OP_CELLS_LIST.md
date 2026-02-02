# cells.list Op

Returns all current cells with their IDs, names, working paths, and descriptions.

## Input

```json
{}
```

No fields are required. The worker must be configured with a database connection because the op queries the cells table.

## Output

```json
{
  "cells": [
    {
      "id": "cel123",
      "name": "alpha",
      "working_path": "cells/alpha",
      "description": "first cell"
    }
  ]
}
```

`cells` is an array; an empty array is returned when no cells exist.
