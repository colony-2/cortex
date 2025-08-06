# Inner Recipe: Cell Executor Specification

## Overview

This specification defines the inner workflow recipe that executes within a devcontainer environment managed by Nucleus. This workflow operates on a specific cell with read-write access while having read-only access to the entire source tree.

## Purpose

The cell executor workflow provides a standardized way to:
- Validate the devcontainer environment setup
- Perform cell-specific operations
- Demonstrate the isolation and mount configuration
- Serve as a template for more complex cell operations

## Recipe Definition

### Location
`recipes/nucleus/cell-executor.yaml`

### Workflow Definition

```yaml
name: cell-executor
version: 1.0.0
description: Executes within a devcontainer to perform cell-specific tasks

inputs:
  - name: cell_name
    type: string
    required: true
    description: Name of the cell being executed
  - name: source_dir
    type: string
    required: false
    default: "/src"
    description: Path to the mounted source directory
  - name: operations
    type: array
    required: false
    default: ["validate", "list", "info"]
    description: List of operations to perform

outputs:
  - name: environment_info
    type: object
    description: Information about the execution environment
  - name: validation_results
    type: object
    description: Results of environment validation
  - name: cell_operations
    type: object
    description: Results of cell-specific operations
  - name: execution_summary
    type: object
    description: Summary of all operations performed

workflow:
  type: sequential
  retry_policy:
    maximum_attempts: 1  # No retries for environment validation
  steps:
    # Environment validation
    - id: validate_environment
      activity: shell
      inputs:
        run: |
          echo "=== Environment Validation ==="
          echo "Container ID: $(hostname)"
          echo "User: $(whoami)"
          echo "Working Directory: $(pwd)"
          echo "Operating System: $(uname -a)"
          echo "Available Tools:"
          for tool in git docker node python go; do
            if command -v $tool &> /dev/null; then
              echo "  - $tool: $(command -v $tool)"
            else
              echo "  - $tool: NOT FOUND"
            fi
          done
      outputs:
        environment: stdout

    # Validate source mount
    - id: validate_source_mount
      activity: shell
      inputs:
        run: |
          SOURCE_DIR="{{ .Inputs.source_dir }}"
          echo "=== Source Mount Validation ==="
          if [ -d "$SOURCE_DIR" ]; then
            echo "Source directory exists: $SOURCE_DIR"
            echo "Mount type: $(findmnt -n -o FSTYPE "$SOURCE_DIR" 2>/dev/null || echo 'unknown')"
            echo "Mount options: $(findmnt -n -o OPTIONS "$SOURCE_DIR" 2>/dev/null || echo 'unknown')"
            
            # Test read access
            if ls "$SOURCE_DIR" &> /dev/null; then
              echo "Read access: GRANTED"
              echo "Top-level contents: $(ls "$SOURCE_DIR" | head -5 | tr '\n' ' ')"
            else
              echo "Read access: DENIED"
            fi
            
            # Test write access to source root (should fail)
            TEST_FILE="$SOURCE_DIR/test-write-$(date +%s).tmp"
            if touch "$TEST_FILE" 2>/dev/null; then
              echo "Write access to source root: GRANTED (unexpected!)"
              rm "$TEST_FILE"
            else
              echo "Write access to source root: DENIED (expected)"
            fi
          else
            echo "ERROR: Source directory not found: $SOURCE_DIR"
            exit 1
          fi
      outputs:
        source_validation: stdout

    # Validate cell mount
    - id: validate_cell_mount
      activity: shell
      inputs:
        run: |
          CELL_PATH="{{ .Inputs.source_dir }}/{{ .Inputs.cell_name }}"
          echo "=== Cell Mount Validation ==="
          if [ -d "$CELL_PATH" ]; then
            echo "Cell directory exists: $CELL_PATH"
            
            # Test write access to cell (should succeed)
            TEST_FILE="$CELL_PATH/test-write-$(date +%s).tmp"
            if touch "$TEST_FILE" 2>/dev/null; then
              echo "Write access to cell: GRANTED (expected)"
              echo "Test file created: $TEST_FILE"
              rm "$TEST_FILE"
            else
              echo "Write access to cell: DENIED (unexpected!)"
            fi
            
            echo "Cell contents:"
            ls -la "$CELL_PATH" | head -10
          else
            echo "ERROR: Cell directory not found: $CELL_PATH"
            exit 1
          fi
      outputs:
        cell_validation: stdout

    # List source directory
    - id: list_source_directory
      activity: shell
      inputs:
        run: |
          if [[ " {{ .Inputs.operations }} " =~ " list " ]]; then
            echo "=== Source Directory Listing ==="
            ls -la "{{ .Inputs.source_dir }}" | head -20
            echo ""
            echo "Total items: $(ls -1 "{{ .Inputs.source_dir }}" | wc -l)"
          else
            echo "Skipping source directory listing"
          fi
        continue_on_error: true
      outputs:
        listing: stdout

    # Get cell information
    - id: get_cell_info
      activity: shell
      inputs:
        run: |
          if [[ " {{ .Inputs.operations }} " =~ " info " ]]; then
            CELL_PATH="{{ .Inputs.source_dir }}/{{ .Inputs.cell_name }}"
            echo "=== Cell Information ==="
            echo "Cell Name: {{ .Inputs.cell_name }}"
            echo "Cell Path: $CELL_PATH"
            
            if [ -f "$CELL_PATH/package.json" ]; then
              echo "Type: Node.js project"
              echo "Package Info:"
              cat "$CELL_PATH/package.json" | head -20
            elif [ -f "$CELL_PATH/go.mod" ]; then
              echo "Type: Go module"
              echo "Module Info:"
              cat "$CELL_PATH/go.mod"
            elif [ -f "$CELL_PATH/Cargo.toml" ]; then
              echo "Type: Rust project"
              echo "Cargo Info:"
              cat "$CELL_PATH/Cargo.toml" | head -20
            else
              echo "Type: Generic directory"
              echo "Contents summary:"
              find "$CELL_PATH" -maxdepth 2 -type f | head -20
            fi
          else
            echo "Skipping cell info gathering"
          fi
        continue_on_error: true
      outputs:
        info: stdout

    # Check recipe mount
    - id: check_recipe_mount
      activity: shell
      inputs:
        run: |
          echo "=== Recipe Mount Check ==="
          if [ -d "/recipes" ]; then
            echo "Recipes directory mounted at /recipes"
            echo "Available recipes:"
            find /recipes -name "*.yaml" -type f | head -10
          else
            echo "WARNING: Recipes directory not mounted at /recipes"
          fi
      outputs:
        recipes: stdout

    # Create execution summary
    - id: create_summary
      activity: shell
      inputs:
        run: |
          echo "=== Execution Summary ==="
          echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
          echo "Cell: {{ .Inputs.cell_name }}"
          echo "Operations performed: {{ .Inputs.operations }}"
          echo "All validations completed successfully"
      outputs:
        summary: stdout

  outputs:
    environment_info:
      container: "{{ .Steps.validate_environment.Outputs.environment }}"
      recipes: "{{ .Steps.check_recipe_mount.Outputs.recipes }}"
    validation_results:
      source_mount: "{{ .Steps.validate_source_mount.Outputs.source_validation }}"
      cell_mount: "{{ .Steps.validate_cell_mount.Outputs.cell_validation }}"
    cell_operations:
      listing: "{{ .Steps.list_source_directory.Outputs.listing }}"
      info: "{{ .Steps.get_cell_info.Outputs.info }}"
    execution_summary:
      summary: "{{ .Steps.create_summary.Outputs.summary }}"
      success: true
```

### Activities Definition

```yaml
# recipes/nucleus/activities.yaml
activities:
  - name: shell
    description: Execute shell commands in the container environment
    timeout: 5m
    retry:
      maximum_attempts: 3
      non_retryable_errors:
        - "ValidationError"
        - "PermissionError"
    inputs:
      - name: run
        type: string
        required: true
        description: Shell command to execute
      - name: working_directory
        type: string
        required: false
        description: Working directory for command execution
      - name: continue_on_error
        type: boolean
        required: false
        default: false
        description: Continue workflow even if command fails
    outputs:
      - name: stdout
        type: string
        description: Standard output from the command
      - name: stderr
        type: string
        description: Standard error from the command
      - name: exit_code
        type: integer
        description: Exit code of the command
    implementation:
      type: command_execution
      config:
        shell: bash
```

## Usage Patterns

### Basic Validation
```yaml
inputs:
  cell_name: "web/app"
  operations: ["validate"]
```

### Full Analysis
```yaml
inputs:
  cell_name: "server/api"
  operations: ["validate", "list", "info"]
```

### Custom Source Directory
```yaml
inputs:
  cell_name: "server/ono"
  source_dir: "/workspace"
  operations: ["info"]
```

## Extension Points

This base workflow can be extended for:

1. **Build Operations**: Add steps to build the cell's code
2. **Test Execution**: Run cell-specific tests
3. **Dependency Analysis**: Analyze and validate dependencies
4. **Code Generation**: Generate code or documentation
5. **Deployment Preparation**: Package cell for deployment

## Error Handling

The workflow includes comprehensive error handling:
- Environment validation failures halt execution
- Optional operations can fail without stopping the workflow
- All errors are captured in outputs for analysis

## Future Enhancements

1. **Parameterized Operations**: Support custom operation definitions
2. **Output Artifacts**: Store generated files or build artifacts
3. **Progress Reporting**: Real-time progress updates via activities
4. **Resource Monitoring**: Track CPU/memory usage during execution
5. **Cache Management**: Utilize container caches for dependencies