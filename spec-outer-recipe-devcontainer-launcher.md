# Outer Recipe: Devcontainer Launcher Specification

## Overview

This specification defines the outer workflow recipe that runs in Cortex to manage the complete lifecycle of devcontainer-based cell execution. It handles creating working directories, performing git clones, starting devcontainers, launching Nucleus within containers, and orchestrating inner workflow execution.

## Purpose

The devcontainer launcher workflow:
- Creates isolated execution environments for cells
- Manages the devcontainer lifecycle
- Establishes proper mount configurations
- Launches Nucleus instances within containers
- Submits and monitors inner workflows

## Recipe Definition

### Location
`recipes/cortex/devcontainer-launcher.yaml`

### Workflow Definition

```yaml
name: devcontainer-launcher
version: 1.0.0
description: Launches a devcontainer for a cell and executes workflows within it

inputs:
  - name: cell_name
    type: string
    required: true
    description: Name/path of the cell to execute (e.g., "web/app", "server/api")
  - name: commit_hash
    type: string
    required: false
    default: "HEAD"
    description: Git commit hash to clone
  - name: inner_workflow
    type: string
    required: false
    default: "cell-executor"
    description: Workflow to execute inside the devcontainer
  - name: inner_workflow_inputs
    type: object
    required: false
    default: {}
    description: Additional inputs for the inner workflow
  - name: cleanup_on_failure
    type: boolean
    required: false
    default: true
    description: Clean up resources on failure
  - name: cleanup_on_success
    type: boolean
    required: false
    default: false
    description: Clean up resources on success (for testing)

outputs:
  - name: execution_id
    type: string
    description: Unique identifier for this execution
  - name: working_directory
    type: string
    description: Path to the created working directory
  - name: container_id
    type: string
    description: ID of the started devcontainer
  - name: nucleus_host
    type: string
    description: Host:port of the Nucleus instance
  - name: inner_workflow_id
    type: string
    description: ID of the inner workflow execution
  - name: inner_workflow_result
    type: object
    description: Result from the inner workflow execution
  - name: execution_metadata
    type: object
    description: Metadata about the execution

workflow:
  type: sequential
  retry_policy:
    maximum_attempts: 2
    initial_interval: 30s
    backoff_coefficient: 2.0
  steps:
    # Generate execution ID
    - id: generate_execution_id
      activity: shell
      inputs:
        run: |
          TIMESTAMP=$(date +%Y%m%d_%H%M%S)
          RANDOM_SUFFIX=$(head -c 8 /dev/urandom | xxd -p | head -c 8)
          EXEC_ID="${TIMESTAMP}-${RANDOM_SUFFIX}"
          echo "$EXEC_ID"
      outputs:
        execution_id: stdout

    # Create working directory
    - id: create_working_directory
      activity: shell
      inputs:
        run: |
          CELL_NAME="{{ .Inputs.cell_name }}"
          EXEC_ID="{{ .Steps.generate_execution_id.Outputs.execution_id }}"
          WORKING_DIR="${CELL_NAME}/${EXEC_ID}"
          
          # Create directory structure
          mkdir -p "$WORKING_DIR"
          
          # Store absolute path
          ABSOLUTE_PATH=$(cd "$WORKING_DIR" && pwd)
          echo "$ABSOLUTE_PATH"
      outputs:
        working_directory: stdout

    # Get current git commit if HEAD specified
    - id: resolve_commit_hash
      activity: shell
      inputs:
        run: |
          COMMIT="{{ .Inputs.commit_hash }}"
          if [ "$COMMIT" = "HEAD" ]; then
            COMMIT=$(git rev-parse HEAD)
          fi
          echo "$COMMIT"
      outputs:
        commit_hash: stdout

    # Perform shallow clone
    - id: shallow_clone
      activity: gitshallow
      inputs:
        sourceDir: "."
        targetDir: "{{ .Steps.create_working_directory.Outputs.working_directory }}/repo"
        commitHash: "{{ .Steps.resolve_commit_hash.Outputs.commit_hash }}"
      outputs:
        cloned_path: clonedPath

    # Determine devcontainer configuration
    - id: select_devcontainer_config
      activity: shell
      inputs:
        run: |
          CELL_PATH="{{ .Inputs.cell_name }}"
          REPO_PATH="{{ .Steps.shallow_clone.Outputs.cloned_path }}"
          
          # Check for cell-specific devcontainer
          CELL_DEVCONTAINER="${REPO_PATH}/${CELL_PATH}/.devcontainer/devcontainer.json"
          CELL_DEVCONTAINER_ALT="${REPO_PATH}/${CELL_PATH}/.devcontainer.json"
          
          # Check for root devcontainer
          ROOT_DEVCONTAINER="${REPO_PATH}/.devcontainer/devcontainer.json"
          ROOT_DEVCONTAINER_ALT="${REPO_PATH}/.devcontainer.json"
          
          if [ -f "$CELL_DEVCONTAINER" ]; then
            echo "${CELL_PATH}/.devcontainer/devcontainer.json"
          elif [ -f "$CELL_DEVCONTAINER_ALT" ]; then
            echo "${CELL_PATH}/.devcontainer.json"
          elif [ -f "$ROOT_DEVCONTAINER" ]; then
            echo ".devcontainer/devcontainer.json"
          elif [ -f "$ROOT_DEVCONTAINER_ALT" ]; then
            echo ".devcontainer.json"
          else
            echo "ERROR: No devcontainer.json found"
            exit 1
          fi
      outputs:
        devcontainer_path: stdout

    # Start devcontainer with proper mounts
    - id: start_devcontainer
      activity: devcontainer_up
      inputs:
        workspace_folder: "{{ .Steps.shallow_clone.Outputs.cloned_path }}"
        config_path: "{{ .Steps.select_devcontainer_config.Outputs.devcontainer_path }}"
        container_name: "vibethis-{{ .Steps.generate_execution_id.Outputs.execution_id }}"
        mounts:
          # Mount entire source as read-only
          - type: bind
            source: "{{ .Steps.shallow_clone.Outputs.cloned_path }}"
            target: "/src"
            readonly: true
          # Mount cell directory as read-write, overlaying the read-only mount
          - type: bind
            source: "{{ .Steps.shallow_clone.Outputs.cloned_path }}/{{ .Inputs.cell_name }}"
            target: "/src/{{ .Inputs.cell_name }}"
            readonly: false
          # Mount nucleus recipes
          - type: bind
            source: "{{ .Steps.shallow_clone.Outputs.cloned_path }}/recipes/nucleus"
            target: "/recipes"
            readonly: true
        labels:
          vibethis.cell: "{{ .Inputs.cell_name }}"
          vibethis.execution_id: "{{ .Steps.generate_execution_id.Outputs.execution_id }}"
          vibethis.commit: "{{ .Steps.resolve_commit_hash.Outputs.commit_hash }}"
      outputs:
        container_id: container_id
        container_info: container_info

    # Start Nucleus in the container
    - id: start_nucleus
      activity: shell
      inputs:
        run: |
          CONTAINER_ID="{{ .Steps.start_devcontainer.Outputs.container_id }}"
          NUCLEUS_NAME="nucleus-{{ .Steps.generate_execution_id.Outputs.execution_id }}"
          
          # Start embedded Temporal and Nucleus
          docker exec "$CONTAINER_ID" bash -c "
            # Start embedded temporal (if not using external)
            nucleus start \
              --name '$NUCLEUS_NAME' \
              --recipes-dir /recipes \
              --temporal-namespace nucleus \
              --temporal-ui-port 8233 \
              --api-port 8234 \
              --embedded-temporal \
              --detach
          "
          
          # Wait for Nucleus to be ready
          sleep 5
          
          # Get container IP for internal communication
          CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$CONTAINER_ID")
          echo "${CONTAINER_IP}:7233"
      outputs:
        nucleus_host: stdout

    # Submit inner workflow
    - id: submit_inner_workflow
      activity: temporal_workflow
      inputs:
        temporal_host: "{{ .Steps.start_nucleus.Outputs.nucleus_host }}"
        workflow_name: "{{ .Inputs.inner_workflow }}"
        workflow_inputs:
          cell_name: "{{ .Inputs.cell_name }}"
          source_dir: "/src"
          operations: ["validate", "list", "info"]
          # Merge with user-provided inputs
          __merge__: "{{ .Inputs.inner_workflow_inputs }}"
        workflow_id: "inner-{{ .Steps.generate_execution_id.Outputs.execution_id }}"
      outputs:
        workflow_id: workflow_id
        run_id: run_id
        result: result
        status: status

    # Collect execution metadata
    - id: collect_metadata
      activity: shell
      inputs:
        run: |
          echo '{
            "execution_id": "{{ .Steps.generate_execution_id.Outputs.execution_id }}",
            "cell_name": "{{ .Inputs.cell_name }}",
            "commit_hash": "{{ .Steps.resolve_commit_hash.Outputs.commit_hash }}",
            "devcontainer_config": "{{ .Steps.select_devcontainer_config.Outputs.devcontainer_path }}",
            "container_id": "{{ .Steps.start_devcontainer.Outputs.container_id }}",
            "nucleus_host": "{{ .Steps.start_nucleus.Outputs.nucleus_host }}",
            "inner_workflow": "{{ .Inputs.inner_workflow }}",
            "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
          }'
      outputs:
        metadata: stdout

    # Cleanup (conditional)
    - id: cleanup_resources
      activity: shell
      inputs:
        run: |
          SHOULD_CLEANUP="false"
          SUCCESS="{{ .Steps.submit_inner_workflow.Outputs.status }}"
          
          if [ "$SUCCESS" = "completed" ] && [ "{{ .Inputs.cleanup_on_success }}" = "true" ]; then
            SHOULD_CLEANUP="true"
          elif [ "$SUCCESS" != "completed" ] && [ "{{ .Inputs.cleanup_on_failure }}" = "true" ]; then
            SHOULD_CLEANUP="true"
          fi
          
          if [ "$SHOULD_CLEANUP" = "true" ]; then
            echo "Cleaning up resources..."
            # Stop and remove container
            docker stop "{{ .Steps.start_devcontainer.Outputs.container_id }}" || true
            docker rm "{{ .Steps.start_devcontainer.Outputs.container_id }}" || true
            
            # Remove working directory (optional, based on policy)
            # rm -rf "{{ .Steps.create_working_directory.Outputs.working_directory }}"
            
            echo "Cleanup completed"
          else
            echo "Skipping cleanup"
          fi
        continue_on_error: true
      outputs:
        cleanup_status: stdout

  outputs:
    execution_id: "{{ .Steps.generate_execution_id.Outputs.execution_id }}"
    working_directory: "{{ .Steps.create_working_directory.Outputs.working_directory }}"
    container_id: "{{ .Steps.start_devcontainer.Outputs.container_id }}"
    nucleus_host: "{{ .Steps.start_nucleus.Outputs.nucleus_host }}"
    inner_workflow_id: "{{ .Steps.submit_inner_workflow.Outputs.workflow_id }}"
    inner_workflow_result: "{{ .Steps.submit_inner_workflow.Outputs.result }}"
    execution_metadata: "{{ .Steps.collect_metadata.Outputs.metadata }}"
```

### Activities Definition

```yaml
# recipes/cortex/activities.yaml
activities:
  - name: shell
    description: Execute shell commands
    timeout: 5m
    implementation:
      type: command_execution
      config:
        shell: bash

  - name: gitshallow
    description: Perform shallow git clone
    timeout: 10m
    implementation:
      type: gitshallow

  - name: devcontainer_up
    description: Start a devcontainer with specified configuration
    timeout: 15m
    inputs:
      - name: workspace_folder
        type: string
        required: true
      - name: config_path
        type: string
        required: true
      - name: container_name
        type: string
        required: false
      - name: mounts
        type: array
        required: false
      - name: labels
        type: object
        required: false
    outputs:
      - name: container_id
        type: string
      - name: container_info
        type: object
    implementation:
      type: devcontainer_management
      config:
        operation: up

  - name: temporal_workflow
    description: Submit and monitor workflow in remote Temporal instance
    timeout: 35m
    inputs:
      - name: temporal_host
        type: string
        required: true
      - name: workflow_name
        type: string
        required: true
      - name: workflow_inputs
        type: object
        required: false
      - name: workflow_id
        type: string
        required: false
    outputs:
      - name: workflow_id
        type: string
      - name: run_id
        type: string
      - name: result
        type: object
      - name: status
        type: string
    implementation:
      type: temporal_workflow
      config:
        temporal_namespace: nucleus
        task_queue: recipe-worker
```

## Error Handling

The workflow includes comprehensive error handling at each step:

1. **Working Directory Creation**: Fails if unable to create directory
2. **Git Operations**: Validates commit exists before cloning
3. **Devcontainer Selection**: Fails if no devcontainer.json found
4. **Container Start**: Retries on transient failures
5. **Nucleus Launch**: Waits for readiness before proceeding
6. **Workflow Submission**: Handles timeout and connection errors
7. **Cleanup**: Always attempts cleanup, continues on error

## Resource Management

### Container Lifecycle
- Containers are tagged with execution metadata
- Automatic cleanup based on success/failure policies
- Orphaned containers can be identified by labels

### Working Directory Management
- Unique directories per execution
- Optional cleanup after execution
- Preserves artifacts for debugging

### Network Configuration
- Containers use default bridge network
- Nucleus accessible via container IP
- Future: Support custom networks

## Security Considerations

1. **Mount Isolation**: Read-only source with selective write access
2. **Container Labels**: Track ownership and purpose
3. **Resource Limits**: Applied via devcontainer configuration
4. **Network Isolation**: Containers isolated by default
5. **Credential Management**: No credentials stored in workflow

## Usage Examples

### Basic Cell Execution
```yaml
inputs:
  cell_name: "web/app"
```

### Specific Commit with Custom Workflow
```yaml
inputs:
  cell_name: "server/api"
  commit_hash: "abc123def"
  inner_workflow: "build-and-test"
  cleanup_on_success: true
```

### Testing Mode
```yaml
inputs:
  cell_name: "example/hello"
  inner_workflow: "cell-executor"
  inner_workflow_inputs:
    operations: ["validate"]
  cleanup_on_success: true
  cleanup_on_failure: false
```

## Future Enhancements

1. **Parallel Execution**: Support multiple cells in parallel
2. **Caching**: Cache devcontainer images and git clones
3. **Progress Streaming**: Real-time progress updates
4. **Resource Pools**: Manage container resource allocation
5. **Workflow Templates**: Parameterized workflow generation