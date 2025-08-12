# Cortex Execute Command Specification

## Overview

The `cortex execute` subcommand provides a way to run recipe YAML files directly from the command line, passing inputs as arguments and receiving outputs when the recipe completes. This command is designed for CI/CD integration, testing, and standalone recipe execution without requiring a full server deployment.

## Command Syntax

```bash
cortex execute <recipe-file> [flags]
```

### Positional Arguments

- `<recipe-file>` (required): Path to the recipe YAML file to execute

### Flags

- `-i, --input <key=value>`: Set a recipe input value (can be specified multiple times)
- `-f, --input-file <file>`: Load inputs from a JSON or YAML file
- `-o, --output <format>`: Output format for results (text, json) (default: text)
- `-l, --log-level <level>`: Set logging verbosity (debug, info, warn, error) (default: info)
- `--log-format <format>`: Log output format (text, json) (default: text)
- `--timeout <duration>`: Maximum execution time (e.g., 30s, 5m, 1h) (default: 30m)
- `--dry-run`: Validate the recipe and inputs without executing
- `--no-color`: Disable colored output for logs
- `--state-dir <path>`: Directory for temporary state files (default: system temp)
- `--cleanup`: Clean up state files after execution (default: true)
- `--parallel-limit <n>`: Maximum number of parallel operations (default: 10)

## Input Handling

### Command-Line Inputs

Inputs can be provided directly via command-line flags:

```bash
cortex execute recipe.yaml \
  -i "user_prompt=Analyze this data" \
  -i "max_tokens=2000" \
  -i "temperature=0.7"
```

### File-Based Inputs

For complex inputs, use a JSON or YAML file:

```bash
cortex execute recipe.yaml -f inputs.json
```

Example `inputs.json`:
```json
{
  "user_prompt": "Analyze this data",
  "max_tokens": 2000,
  "temperature": 0.7,
  "data": {
    "source": "database",
    "query": "SELECT * FROM users"
  }
}
```

Example `inputs.yaml`:
```yaml
user_prompt: Analyze this data
max_tokens: 2000
temperature: 0.7
data:
  source: database
  query: SELECT * FROM users
```

### Input Type Coercion

The command will automatically coerce string inputs to the appropriate types based on the recipe's input definitions:
- Strings: Passed as-is
- Numbers: Parsed from string representation
- Booleans: Recognized values: true/false, yes/no, 1/0
- Objects/Arrays: Must be provided via JSON notation or file input

## Output Handling

### Output Formats

#### JSON (default)
```json
{
  "success": true,
  "outputs": {
    "result": "Analysis complete",
    "confidence": 0.95,
    "details": {...}
  },
  "execution_time": "15.3s",
  "recipe": "analysis-pipeline",
  "run_id": "run_abc123"
}
```

#### YAML
```yaml
success: true
outputs:
  result: Analysis complete
  confidence: 0.95
  details: ...
execution_time: 15.3s
recipe: analysis-pipeline
run_id: run_abc123
```

#### Text (human-readable)
```
Recipe: analysis-pipeline
Status: SUCCESS
Execution Time: 15.3s
Run ID: run_abc123

Outputs:
  result: Analysis complete
  confidence: 0.95
  details: [complex object]
```

### Error Output

On failure, the output includes error details:

```json
{
  "success": false,
  "error": {
    "message": "Activity 'llm_inference' failed",
    "step_id": "step1",
    "details": "API rate limit exceeded"
  },
  "partial_outputs": {...},
  "execution_time": "5.2s",
  "recipe": "analysis-pipeline",
  "run_id": "run_abc123"
}
```

## Logging Format

### Log Levels

- **DEBUG**: Detailed execution trace, including all activity inputs/outputs
- **INFO**: Recipe and operation lifecycle events
- **WARN**: Non-fatal issues and deprecation warnings
- **ERROR**: Fatal errors and stack traces

### Log Output Structure

Each log line follows this format:

```
[timestamp] [level] [component] message {metadata}
```

Example execution log:

```
[2024-01-15T10:30:00Z] [INFO] [executor] Recipe started: test-recipe {run_id: run_abc123, inputs: {user_prompt: "Hello", max_tokens: 1000}}
[2024-01-15T10:30:00Z] [INFO] [executor] Step started: step1 (Generate response) {type: llm_inference, parent: root}
[2024-01-15T10:30:01Z] [DEBUG] [activity.llm] Calling LLM API {model: gpt-4, tokens: 1000}
[2024-01-15T10:30:03Z] [INFO] [activity.llm] LLM response received {tokens_used: 750, latency: 2.1s}
[2024-01-15T10:30:03Z] [INFO] [executor] Step completed: step1 {outputs: {response: "..."}, duration: 3s}
[2024-01-15T10:30:03Z] [INFO] [executor] Step started: step2 (Process result) {type: command_execution, parent: root}
[2024-01-15T10:30:03Z] [DEBUG] [activity.cmd] Executing command: echo [...] {timeout: 30s}
[2024-01-15T10:30:03Z] [INFO] [executor] Step completed: step2 {outputs: {stdout: "..."}, duration: 0.1s}
[2024-01-15T10:30:03Z] [INFO] [executor] Recipe completed: test-recipe {total_duration: 3.1s, status: success}
```

### Nested Recipe Execution

When recipes invoke child recipes:

```
[2024-01-15T10:30:00Z] [INFO] [executor] Recipe started: parent-recipe {run_id: run_parent}
[2024-01-15T10:30:01Z] [INFO] [executor] Step started: invoke_child {type: recipe_invocation}
[2024-01-15T10:30:01Z] [INFO] [executor] Child recipe started: child-recipe {run_id: run_child, parent: run_parent}
[2024-01-15T10:30:02Z] [INFO] [executor] Step started: child_step1 {recipe: child-recipe}
[2024-01-15T10:30:03Z] [INFO] [executor] Step completed: child_step1 {recipe: child-recipe}
[2024-01-15T10:30:03Z] [INFO] [executor] Child recipe completed: child-recipe {parent: run_parent}
[2024-01-15T10:30:03Z] [INFO] [executor] Step completed: invoke_child
[2024-01-15T10:30:03Z] [INFO] [executor] Recipe completed: parent-recipe
```

### JSON Log Format

With `--log-format json`:

```json
{"timestamp":"2024-01-15T10:30:00Z","level":"INFO","component":"executor","message":"Recipe started","recipe":"test-recipe","run_id":"run_abc123","inputs":{"user_prompt":"Hello","max_tokens":1000}}
{"timestamp":"2024-01-15T10:30:00Z","level":"INFO","component":"executor","message":"Step started","step_id":"step1","step_name":"Generate response","activity_type":"llm_inference"}
```

## Signal Handling

### Ctrl-C (SIGINT) Behavior

When the user presses Ctrl-C:

1. **First signal**: Graceful shutdown initiated
   - Current operations are allowed to complete
   - No new operations are started
   - State is preserved for potential resume
   - Cleanup handlers are executed
   - Log message: `[INFO] Shutdown requested, waiting for current operations to complete...`

2. **Second signal** (within grace period): Force shutdown
   - All operations are immediately terminated
   - State may be inconsistent
   - Log message: `[WARN] Force shutdown initiated`

### State Preservation

On interruption, the command saves:
- Current execution state
- Completed step outputs
- Pending operations queue

State file location: `{state-dir}/cortex-run-{run_id}/state.json`

### Resume Capability (Future Enhancement)

While not implemented in the initial version, the state preservation enables future resume functionality:

```bash
# Future capability
cortex execute recipe.yaml --resume run_abc123
```

## Exit Codes

- `0`: Successful execution
- `1`: Recipe execution failed
- `2`: Invalid arguments or recipe file
- `3`: Recipe validation failed
- `4`: Input validation failed
- `5`: Timeout exceeded
- `130`: Interrupted by user (SIGINT)
- `143`: Terminated (SIGTERM)

## Examples

### Basic Execution

```bash
# Execute with inline inputs
cortex execute analysis.yaml -i "data=sample.csv" -i "format=json"

# Execute with input file
cortex execute pipeline.yaml -f inputs.yaml

# Execute with debug logging
cortex execute recipe.yaml -l debug

# Dry run to validate
cortex execute recipe.yaml --dry-run -i "test=value"
```

## Implementation Notes

### Architecture

1. **Command Handler**: Parses arguments, loads recipe and inputs
2. **Validator**: Validates recipe schema and input compatibility
3. **Executor Engine**: Manages recipe execution lifecycle
4. **Activity Registry**: Loads and manages available activities
5. **State Manager**: Handles state persistence and cleanup
6. **Logger**: Formats and outputs execution logs
7. **Signal Handler**: Manages graceful shutdown

### Dependencies

- Reuses existing recipe validation logic from `validate` command
- Leverages recipe-worker execution engine
- Integrates with activity registry from ops package
- Uses context cancellation for timeout and interruption

### Performance Considerations

- Lazy loading of activities to minimize startup time
- Streaming logs to avoid memory buildup
- Efficient state serialization for large outputs
- Connection pooling for external service calls
