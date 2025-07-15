# rwshim/clib Directory

## Purpose
This directory contains a C-based read/write interception shim that uses dynamic library preloading (`LD_PRELOAD` on Linux, `DYLD_INSERT_LIBRARIES` on macOS) to intercept and control file I/O operations at the system call level. It provides a foundation for monitoring and controlling process I/O behavior.

## Architecture Overview
The shim operates by:
1. Intercepting `read()` and `write()` system calls via dynamic library injection
2. Communicating with a monitor process over Unix domain socket
3. Allowing or denying I/O operations based on monitor policy decisions

## Key Components

### Core Shim Library
- **intercept.c**: Main interception library that overrides read/write syscalls
  - Uses `dlsym(RTLD_NEXT)` to get original function pointers
  - Connects to monitor socket at `/tmp/vibethis-rwshim.sock`
  - Resolves file descriptors to filenames via `/proc/self/fd/` (Linux)
  - Returns EPERM when operations are denied

### Monitor Implementation
- **mock_monitor.c**: Reference implementation of monitoring service
  - Creates Unix domain socket server
  - Supports multiple policy modes:
    - `POLICY_ALLOW_ALL`: Permits all operations (default)
    - `POLICY_DENY_ALL`: Denies all operations
    - `POLICY_DENY_WRITES`: Denies only write operations
    - `POLICY_DENY_READS`: Denies only read operations
  - Handles cleanup via signal handlers

### Testing Infrastructure
- **test_app.c**: Simple test application performing various I/O operations
  - Tests stdout writes, file creation/writing, and file reading
  - Used to verify shim behavior under different policies
- **run_tests_docker.sh**: Runs tests in Docker container (Linux)
- **run_tests_macos.sh**: Runs tests natively on macOS
- **test_runner_linux.sh**: Core test script for Linux environment

## Protocol Specification

### Request Format
```
<OPERATION> <FD> <SIZE> <FILENAME>\n
```
- OPERATION: "READ" or "WRITE"
- FD: File descriptor number
- SIZE: Byte count for operation
- FILENAME: Resolved file path or special name (stdin/stdout/stderr)

### Response Format
- `ALLOW\n`: Operation permitted
- `DENY\n`: Operation denied

### Socket Path
Fixed location: `/tmp/vibethis-rwshim.sock`

## Build System

### Docker Build
- **Dockerfile**: Multi-stage build for Linux shared library
  - Stage 1: Ubuntu 24.04 with build-essential
  - Stage 2: Minimal runtime with just the .so file
- Builds `intercept.so` with position-independent code (`-fPIC`)

### Moon Integration
- **moon.yml**: Defines project tasks
  - `build`: Creates Docker image `rwshim:latest`
  - `test`: Executes Docker-based test suite

### Platform Differences
- Linux: Produces `intercept.so`, uses `LD_PRELOAD`
- macOS: Produces `intercept.dylib`, uses `DYLD_INSERT_LIBRARIES`
- Both platforms share the same source code and protocol

## Usage Example
```bash
# Start monitor with specific policy
./mock_monitor deny-writes

# Run application with shim
LD_PRELOAD=./intercept.so ./target_app
```

## Integration Points
- Provides C library foundation for higher-level rwshim components
- Protocol designed for easy implementation in other languages (e.g., Go)
- Monitor can be replaced with more sophisticated policy engines

## Security Considerations
- Shim defaults to ALLOW when monitor unavailable (fail-open)
- No authentication between shim and monitor
- Socket permissions rely on filesystem security
- Cannot intercept statically linked binaries or direct syscalls

## Limitations
- Filename resolution requires `/proc` filesystem (Linux-specific)
- Performance overhead per I/O operation due to socket communication
- Thread-safe but creates new connection per operation
- Some applications may detect and bypass the preload mechanism