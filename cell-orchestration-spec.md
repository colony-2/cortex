# Cell Orchestration and Container Management Specification

## Overview
This specification defines a monitoring and orchestration system that manages cell nuclei as containerized processes, automatically starting and stopping them based on pending work in Temporal workflows.

## Architecture Components

### 1. Cell Monitor Service
A dedicated monitoring process that continuously evaluates the state of Temporal workflows to determine which cells require active nuclei.

#### Responsibilities
- Query Temporal for pending cell work using efficient search patterns
- Start nucleus containers for cells with pending work
- Stop nucleus containers when work is completed (with configurable grace period)
- Health check running nuclei
- Report metrics on cell activity

#### Workflow Discovery Pattern
To avoid per-cell queries (which would not scale to hundreds of cells), we use Temporal search attributes:

```
SearchAttributes:
  CellId: string (indexed)
  RecipeType: string (indexed) // "triage" | "spec" | "code" | "doc"
  Status: string (indexed) // "pending" | "running" | "completed"
  RequiresNucleus: boolean (indexed)
```

Query pattern for finding cells with work:
```
RequiresNucleus=true AND Status IN ("pending", "running")
```

This returns all cells needing nuclei in a single query, grouped by CellId.

### 2. DevContainer Configuration

#### DevContainer Pattern
The system uses the existing `server/container` module to manage devcontainer lifecycle. Each cell nucleus runs in a devcontainer with proper development environment setup.

#### Global DevContainer Definition
```json
// /cortex/devcontainer/global.devcontainer.json
{
  "name": "nucleus-${CELL_ID}",
  "image": "cortex/nucleus-base:latest",
  
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/go:1": {},
    "ghcr.io/devcontainers/features/python:1": {},
    "ghcr.io/devcontainers/features/node:1": {}
  },
  
  "mounts": [
    {
      "source": "${CODEBASE_SNAPSHOT_PATH}",
      "target": "/workspace/codebase",
      "type": "bind",
      "readonly": true
    },
    {
      "source": "cell-${CELL_ID}-git",
      "target": "/workspace/git-packs",
      "type": "volume"
    },
    {
      "source": "cell-${CELL_ID}-specs",
      "target": "/workspace/specs", 
      "type": "volume"
    },
    {
      "source": "${GLOBAL_RECIPES_PATH}",
      "target": "/recipes/global",
      "type": "bind",
      "readonly": true
    }
  ],
  
  "containerEnv": {
    "CELL_ID": "${CELL_ID}",
    "TEMPORAL_ADDRESS": "${TEMPORAL_ADDRESS}",
    "TEMPORAL_NAMESPACE": "${TEMPORAL_NAMESPACE}",
    "RECIPE_PATH": "/recipes/cell:/recipes/global",
    "WORKSPACE_PATH": "/workspace"
  },
  
  "customizations": {
    "vscode": {
      "extensions": [],
      "settings": {}
    }
  },
  
  "postCreateCommand": "nucleus start --cell ${CELL_ID}",
  
  "runArgs": [
    "--network=cortex-net",
    "--memory=2g",
    "--cpus=2"
  ]
}
```

#### Cell-Specific DevContainer Override
```json
// /cells/${CELL_ID}/.devcontainer/devcontainer.json
{
  "name": "nucleus-${CELL_ID}-custom",
  "extends": "/cortex/devcontainer/global.devcontainer.json",
  
  "features": {
    // Additional cell-specific features
    "ghcr.io/devcontainers/features/rust:1": {}
  },
  
  "mounts": [
    // Additional cell-specific mounts
    {
      "source": "${CELL_PATH}/.vibethis/recipes",
      "target": "/recipes/cell",
      "type": "bind",
      "readonly": true
    }
  ],
  
  "containerEnv": {
    // Cell-specific environment overrides
    "CUSTOM_VAR": "value"
  }
}
```

#### Container Lifecycle with DevContainer

##### Startup Sequence
1. Monitor detects pending work for cell via Temporal query
2. Check if nucleus already running for cell
3. If not running:
   - Load devcontainer configuration:
     ```go
     config := container.LoadDevContainerConfig(
       globalPath: "/cortex/devcontainer/global.devcontainer.json",
       cellOverride: fmt.Sprintf("/cells/%s/.devcontainer/devcontainer.json", cellID),
       templateVars: map[string]string{
         "CELL_ID": cellID,
         "TEMPORAL_ADDRESS": temporalAddr,
         "TEMPORAL_NAMESPACE": namespace,
         "CODEBASE_SNAPSHOT_PATH": snapshotPath,
         "GLOBAL_RECIPES_PATH": globalRecipesPath,
         "CELL_PATH": cellPath,
       },
     )
     ```
   - Start devcontainer using existing module:
     ```go
     container.StartDevContainer(config)
     ```
4. Nucleus process initializes via postCreateCommand
5. Nucleus connects to Temporal and begins processing

##### Integration with server/container Module
```go
type CellContainerManager struct {
    devcontainer *container.DevContainerManager
    monitor      *CellMonitor
}

func (m *CellContainerManager) StartCellNucleus(cellID string) error {
    // Check for cell-specific override
    configPath := "/cortex/devcontainer/global.devcontainer.json"
    overridePath := fmt.Sprintf("/cells/%s/.devcontainer/devcontainer.json", cellID)
    
    if exists(overridePath) {
        configPath = overridePath
    }
    
    // Template variables for substitution
    vars := map[string]string{
        "CELL_ID": cellID,
        "TEMPORAL_ADDRESS": m.temporalConfig.Address,
        "TEMPORAL_NAMESPACE": m.temporalConfig.Namespace,
        "CODEBASE_SNAPSHOT_PATH": m.getSnapshotPath(),
        "GLOBAL_RECIPES_PATH": "/cortex/recipes/global",
        "CELL_PATH": fmt.Sprintf("/cells/%s", cellID),
    }
    
    // Use existing devcontainer module
    return m.devcontainer.Start(configPath, vars)
}
```

##### Shutdown Sequence
1. Monitor detects no pending/running work for cell
2. Wait for grace period (default: 5 minutes)
3. Re-check for new work
4. If still no work:
   - Send SIGTERM to container
   - Wait for graceful shutdown (max 30 seconds)
   - Force stop if needed
5. Optionally preserve git-packs and specs volumes

### 3. Cortex Configuration

#### Required Cortex Settings
```yaml
cortex:
  orchestration:
    enabled: true
    monitor:
      poll_interval: 10s
      shutdown_grace_period: 5m
      max_nuclei_per_host: 10
    
    container:
      runtime: docker  # or containerd, podman
      network: cortex-net
      resource_limits:
        memory: 2GB
        cpu: 2
      
    snapshots:
      strategy: copy-on-write  # or rsync, btrfs
      retention: 24h
      base_path: /cortex/codebase-snapshot
    
    temporal:
      address: temporal.cortex.local:7233
      namespace: cortex-cells
      search_attributes:
        - CellId
        - RecipeType
        - Status
        - RequiresNucleus
```

#### Cortex Startup Requirements
1. Ensure container runtime is available and configured
2. Create network for inter-container communication
3. Verify Temporal connectivity
4. Initialize snapshot directory
5. Start monitor service

### 4. Monitoring and Observability

#### Metrics to Track
- Cells with pending work
- Active nucleus containers
- Container start/stop events
- Workflow completion rates per cell
- Resource utilization per nucleus

#### Health Checks
- Nucleus process heartbeat to Temporal
- Container health via runtime API
- Disk space for snapshots and volumes
- Memory/CPU usage per container

### 5. Security Considerations

#### Container Isolation
- Each nucleus runs with minimal privileges
- Network segmentation between cells
- No direct access to host filesystem beyond specified mounts

#### Secret Management
- Credentials passed via environment variables
- No secrets in container image
- Optional integration with secret stores (Vault, etc.)

### 6. Scaling Considerations

#### Horizontal Scaling
- Multiple monitor instances with leader election
- Distribute cells across multiple hosts
- Shared storage for snapshots (NFS, S3, etc.)

#### Resource Management
- Queue cells if max_nuclei_per_host reached
- Priority system for critical cells
- Auto-scaling based on queue depth

## Implementation Phases

### Phase 1: Basic Orchestration
- Single monitor process
- Local Docker runtime
- Simple snapshot mechanism
- Basic start/stop logic

### Phase 2: Production Readiness
- High availability monitor
- Multiple container runtime support
- Efficient snapshot strategies
- Complete metrics and monitoring

### Phase 3: Advanced Features
- Multi-host orchestration
- Dynamic resource allocation
- Predictive scaling
- Cost optimization

## Configuration Examples

### Starting Cortex with Orchestration
```bash
cortex start \
  --orchestration-enabled \
  --monitor-config /etc/cortex/monitor.yaml \
  --container-runtime docker \
  --snapshot-path /var/cortex/snapshots \
  --temporal-address temporal:7233
```

### Monitor Service Registration
```yaml
apiVersion: v1
kind: Service
metadata:
  name: cortex-monitor
spec:
  type: ClusterIP
  ports:
    - port: 8080
      name: metrics
    - port: 8081
      name: health
```