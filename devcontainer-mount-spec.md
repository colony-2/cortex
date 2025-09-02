# DevContainer Specification: Selective Read-Write Mount

## Overview
A devcontainer solution that mounts an entire GitHub directory tree as read-only while allowing a dynamically selected subdirectory to be mounted as read-write.

## Requirements

### Base Configuration
- **Base Image**: Official Go image (e.g., `golang:1.21` or `golang:1.21-bullseye`)
- **Primary Process**: Interactive bash shell
- **Mount Strategy**: Overlay/bind mounts with selective permissions

### Directory Structure Support
```
/path/to/gitrepo/                    # Root (read-only)
├── project1/                       # Read-only
├── project2/                       # Read-only
├── mydir/
│   ├── project5/                   # Could be read-write
│   └── project6/                   # Could be read-write
└── deeply/
    └── nested/
        └── path/
            └── project7/            # Could be read-write
```

## Implementation Approaches

### Option 1: Docker Compose with Environment Variables

```yaml
# docker-compose.yml
version: '3.8'
services:
  devcontainer:
    image: golang:1.21-bullseye
    stdin_open: true
    tty: true
    volumes:
      # Mount entire github directory as read-only
      - ${GITHUB_ROOT}:/workspace:ro
      # Overlay mount for specific subdirectory as read-write
      - ${GITHUB_ROOT}/${RW_SUBDIR}:/workspace/${RW_SUBDIR}:rw
    working_dir: /workspace/${RW_SUBDIR}
    command: /bin/bash
    environment:
      - GOPATH=/go
      - PATH=/go/bin:/usr/local/go/bin:${PATH}
```

**Usage**:
```bash
GITHUB_ROOT=/path/to/github RW_SUBDIR=mydir/project5 docker-compose up -d
docker-compose exec devcontainer bash
docker-compose down
```

### Option 2: Shell Script Wrapper

```bash
#!/bin/bash
# start-devcontainer.sh

GITHUB_ROOT="${1:-$(pwd)}"
RW_SUBDIR="${2}"

if [ -z "$RW_SUBDIR" ]; then
    echo "Usage: $0 <github-root> <relative-path-to-rw-dir>"
    echo "Example: $0 /path/to/github mydir/project5"
    exit 1
fi

CONTAINER_NAME="devcontainer-${RW_SUBDIR//\//-}"

docker run -it --rm \
    --name "$CONTAINER_NAME" \
    -v "${GITHUB_ROOT}:/workspace:ro" \
    -v "${GITHUB_ROOT}/${RW_SUBDIR}:/workspace/${RW_SUBDIR}:rw" \
    -w "/workspace/${RW_SUBDIR}" \
    golang:1.21-bullseye \
    bash
```

### Option 3: VS Code DevContainer Configuration

```json
// .devcontainer/devcontainer.json
{
    "name": "Selective RW DevContainer",
    "image": "golang:1.21-bullseye",
    "mounts": [
        "source=${localEnv:GITHUB_ROOT},target=/workspace,type=bind,readonly",
        "source=${localEnv:GITHUB_ROOT}/${localEnv:RW_SUBDIR},target=/workspace/${localEnv:RW_SUBDIR},type=bind"
    ],
    "workspaceFolder": "/workspace/${localEnv:RW_SUBDIR}",
    "customizations": {
        "vscode": {
            "extensions": [
                "golang.go"
            ]
        }
    },
    "remoteUser": "root",
    "postCreateCommand": "go version"
}
```

## Advanced Option: Using OverlayFS (More Complex)

```bash
#!/bin/bash
# advanced-devcontainer.sh

GITHUB_ROOT="${1}"
RW_SUBDIR="${2}"
CONTAINER_NAME="devcontainer-overlay"

# Create temporary directories for overlay
WORK_DIR=$(mktemp -d)
UPPER_DIR=$(mktemp -d)

docker run -it --rm \
    --name "$CONTAINER_NAME" \
    --cap-add SYS_ADMIN \
    -v "${GITHUB_ROOT}:/lower:ro" \
    -v "${UPPER_DIR}:/upper" \
    -v "${WORK_DIR}:/work" \
    -e "RW_SUBDIR=${RW_SUBDIR}" \
    golang:1.21-bullseye \
    bash -c "
        mkdir -p /workspace
        mount -t overlay overlay \
            -o lowerdir=/lower,upperdir=/upper,workdir=/work \
            /workspace
        cd /workspace/${RW_SUBDIR}
        exec bash
    "

# Cleanup
rm -rf "$WORK_DIR" "$UPPER_DIR"
```

## Process Management

### Simple Start/Stop Script
```bash
#!/bin/bash
# devcontainer-manager.sh

ACTION="${1}"
GITHUB_ROOT="${2}"
RW_SUBDIR="${3}"
CONTAINER_NAME="dev-${RW_SUBDIR//\//-}"

start() {
    docker run -d \
        --name "$CONTAINER_NAME" \
        -v "${GITHUB_ROOT}:/workspace:ro" \
        -v "${GITHUB_ROOT}/${RW_SUBDIR}:/workspace/${RW_SUBDIR}:rw" \
        -w "/workspace/${RW_SUBDIR}" \
        golang:1.21-bullseye \
        tail -f /dev/null
    
    echo "Container started. Attach with: docker exec -it $CONTAINER_NAME bash"
}

stop() {
    docker stop "$CONTAINER_NAME"
    docker rm "$CONTAINER_NAME"
    echo "Container stopped and removed"
}

attach() {
    docker exec -it "$CONTAINER_NAME" bash
}

case "$ACTION" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    attach)
        attach
        ;;
    *)
        echo "Usage: $0 {start|stop|attach} <github-root> <rw-subdir>"
        exit 1
        ;;
esac
```

## Considerations

### File System Behavior
1. **Read-only mount**: The entire GitHub directory is mounted read-only at `/workspace`
2. **Read-write overlay**: The specific subdirectory is re-mounted with read-write permissions
3. **Path preservation**: The directory structure is maintained inside the container

### Limitations
1. **Double mount**: The RW directory technically exists twice (once as RO, once as RW)
2. **Inode differences**: The RW mount will have different inodes than the RO version
3. **No write propagation**: Changes in the RW directory won't affect the RO mount

### Security Considerations
- Container runs as root by default (can be changed)
- No network isolation configured (add `--network=none` if needed)
- No resource limits set (add `--memory`, `--cpus` as needed)

## Recommended Approach

For simplicity and flexibility, **Option 2 (Shell Script Wrapper)** is recommended because:
1. Easy to understand and modify
2. No additional configuration files needed
3. Works with existing container tools
4. Can be easily integrated into existing workflows

## Next Steps

1. Choose an implementation approach
2. Test with actual GitHub directory structure
3. Add any project-specific customizations (env vars, tools, etc.)
4. Consider adding volume caching for better performance
5. Implement logging and error handling

## Example Usage

```bash
# Start a devcontainer with project5 as read-write
./start-devcontainer.sh /home/user/github mydir/project5

# Or with the manager script
./devcontainer-manager.sh start /home/user/github deeply/nested/path/project7
./devcontainer-manager.sh attach /home/user/github deeply/nested/path/project7
./devcontainer-manager.sh stop /home/user/github deeply/nested/path/project7
```