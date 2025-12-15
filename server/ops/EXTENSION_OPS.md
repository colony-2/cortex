**Extension Ops (Runtime-Discovered)**

- Goal: Allow projects to add custom operations without changing server code.
- Where: Project-local directories under `.colony2/ops/<op_name>` discovered at runtime.

**How It Works**

- Discovery: `server/ops/pkg/extensions` scans for `.colony2/ops/*/op.yaml` starting from the current working directory and walking up to find the project root. You can also set `VIBETHIS_PROJECT_ROOT` to pin the root.
- Registration: `extensions.DiscoverAndRegister(startDir)` registers each discovered op as a `RegisterableOp`. The standard `export.GetAll()` now appends discovered ops automatically.
- Execution: These ops run as Temporal activities (not inline). The op’s process receives the input as JSON over stdin and must write JSON to stdout as its result.

**Directory Layout**

- `.colony2/ops/<op_name>/` must contain:
  - `op.yaml` (required): Defines metadata, command, and optional settings.
  - Additional files (optional): Scripts, binaries, or other resources referenced by the op.

**op.yaml Schema**

- Minimal, pragmatic spec (all fields optional unless marked required):
  - `name` (string): Human/readable op name. Defaults to the directory name as the registered op type.
  - `description` (string): One-line description.
  - `version` (string): Semantic version (default `0.1.0`).
  - `timeout` (string): Go duration (e.g., `30s`, `5m`). Default `5m`.
  - Command (choose one):
    - `run` (string, recommended): Shell command to execute.
    - `shell` (string): `bash`, `sh`, or `zsh`. Default tries `bash` then falls back to `sh`.
    - OR `command` (string[]): Argv vector (e.g., `["python3", "main.py"]`).
    - `args` (string[]): Extra args appended to `command`.
  - Working dir and env:
    - `working_directory` (string): Default is the op directory. Relative paths resolve from project root.
    - `env` (object<string,string>): Extra environment variables.
  - Schemas (optional):
    - `input_schema` (object): JSON Schema draft-like object (not validated at runtime yet; reserved for docs/UX).
    - `output_schema` (object): Same as above.

Example op.yaml (shell form):

```yaml
name: python_sum
description: Sums numbers provided in input.
version: 1.0.0
shell: bash
run: |
  python3 sum.py # reads stdin JSON, prints JSON
timeout: 30s
env:
  PYTHONUNBUFFERED: "1"
```

Example op.yaml (argv form):

```yaml
name: jq_filter
description: Filters input via jq program.
version: 0.1.0
command: ["jq"]
args: [".items | map(select(.active == true)) | {active: ., count: length}"]
timeout: 10s
```

**Process I/O Contract**

- Input: The op receives its input as JSON on stdin.
- Output: The op must write a single JSON object to stdout; this becomes the op’s outputs.
- Environment variables provided:
  - `VIBETHIS_PROJECT_ROOT`: Absolute project root directory.
  - `VIBETHIS_OP_DIR`: Absolute path to the op directory.
  - `VIBETHIS_OP_NAME`: Resolved op name/type.
  - `VIBETHIS_INPUT_JSON`: Input JSON as a single string (duplicate of stdin for convenience).
  - Plus any variables from `env`.

**Registration API**

- Programmatic:
  - `extensions.Discover(startDir) ([]ops.RegisterableOp, error)`
  - `extensions.DiscoverAndRegister(startDir) ([]ops.RegisterableOp, error)`
- Automatic:
  - `export.GetAll()` includes discovered extension ops in addition to built-ins (LLM, recipe, input).

**Usage in Recipes**

- Use the op by its `<op_name>`—the directory name or `op.yaml` `name` if provided:

```yaml
- id: run_custom
  op: python_sum
  inputs:
    numbers: [1, 2, 3, 4]
```

**Notes & Limitations**

- Inline (workflow) execution is not supported for external commands (non‑deterministic). These ops always run as activities.
- Input/output schemas are stored for documentation and future validation; runtime schema enforcement may be added later.
- Working directory defaults to the op folder to make local scripts/binaries easy to reference; you can override via `working_directory`.

**Example: Simple Python Op**

- `.colony2/ops/python_sum/op.yaml` and `.colony2/ops/python_sum/sum.py`:

`op.yaml`:
```yaml
name: python_sum
description: Sums numbers.
run: |
  python3 sum.py
```

`sum.py`:
```python
import sys, json
data = json.load(sys.stdin)
nums = data.get("numbers", [])
print(json.dumps({"sum": sum(nums)}))
```

Run in a recipe node using `op: python_sum` and provide `inputs.numbers`.

