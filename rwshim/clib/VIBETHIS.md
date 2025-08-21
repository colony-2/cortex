# VIBETHIS.md - Read/Write Intercept Shim (C Library)

## Overview

The rwshim/clib is a C-based interception library that uses LD_PRELOAD to monitor and control read/write system calls in target applications. It provides policy-based access control by communicating with external monitoring processes via Unix domain sockets.

## Architecture

### Core Components

- **intercept.c**: LD_PRELOAD shim that intercepts read()/write() calls and queries monitor for permission
- **mock_monitor.c**: Reference monitoring server implementation with configurable access policies  
- **test_app.c**: Test application that performs various I/O operations for validation

### Communication Flow

```
Target App → intercept.so → Unix Socket → Monitor Process → Policy Decision → Allow/Deny
```

The shim connects to `/tmp/vibethis-rwshim.sock` for each intercepted operation, sends operation details, and waits for ALLOW/DENY response.

### Policy Engine

Monitor supports four policy modes:
- `POLICY_ALLOW_ALL`: Permits all operations (default)
- `POLICY_DENY_ALL`: Blocks all operations
- `POLICY_DENY_WRITES`: Blocks write operations only
- `POLICY_DENY_READS`: Blocks read operations only

## Key Interfaces

### Intercept Shim API

```c
// Intercepted system calls
ssize_t write(int fd, const void *buf, size_t count);
ssize_t read(int fd, void *buf, size_t count);

// Internal functions
static void init_syms(void);                          // Initialize real syscall pointers
static int ask_ok(const char *op, int fd, size_t cnt); // Query monitor for permission
```

### Monitor Socket Protocol

Request format (ASCII):
```
<OPERATION> <FD> <SIZE> <FILENAME>\n
```

Response format:
```
ALLOW\n   // Operation permitted
DENY\n    // Operation denied
```

### Monitor Server Interface

```c
// Policy types
typedef enum {
    POLICY_ALLOW_ALL,
    POLICY_DENY_ALL, 
    POLICY_DENY_WRITES,
    POLICY_DENY_READS
} policy_t;

// Core functions
void handle_request(int client_fd);     // Process incoming requests
void cleanup(int sig);                  // Signal handler for cleanup
int main(int argc, char *argv[]);       // Server entry point
```

## Usage Examples

### Basic Interception

```bash
# Build the shim
gcc -shared -fPIC -o intercept.so intercept.c -ldl

# Start monitor with allow-all policy
./mock_monitor &

# Run target application with interception
LD_PRELOAD=./intercept.so ./target_app
```

### Policy-Based Control

```bash
# Deny all operations
./mock_monitor deny-all &
LD_PRELOAD=./intercept.so ./app  # All I/O will fail with EPERM

# Deny only writes
./mock_monitor deny-writes &
LD_PRELOAD=./intercept.so ./app  # Reads allowed, writes denied
```

### Docker-Based Testing

```bash
# Build and test in isolated environment
docker build -t rwshim:latest .
./run_tests_docker.sh
```

### Cross-Platform Build

```bash
# Linux
gcc -shared -fPIC -o intercept.so intercept.c -ldl

# macOS  
gcc -dynamiclib -o intercept.dylib intercept.c
export DYLD_INSERT_LIBRARIES=./intercept.dylib
```

## Configuration

### Environment Variables

- `LD_PRELOAD`: Path to intercept.so (Linux)
- `DYLD_INSERT_LIBRARIES`: Path to intercept.dylib (macOS)

### Socket Configuration

- Socket path: `/tmp/vibethis-rwshim.sock` (hardcoded)
- Protocol: Unix domain socket, SOCK_STREAM
- Timeout: None (blocking operations)

### Build Configuration

Moon.js task configuration:
```yaml
tasks:
  build:
    command: "docker"
    args: ["build", "-t", "rwshim:latest", "."]
  test:
    command: "./run_tests_docker.sh"
    options:
      outputStyle: buffer-only-failure
```

### Default Behavior

- If monitor unavailable: Allow all operations
- On connection failure: Allow operation (fail-open)
- Thread safety: pthread_once initialization
- Error handling: Returns EPERM for denied operations