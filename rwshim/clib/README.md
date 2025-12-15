# Read/Write Intercept Shim Protocol Documentation

## Overview

The intercept shim is a shared library that uses `LD_PRELOAD` to intercept `read()` and `write()` system calls. It communicates with a monitoring application via a Unix domain socket to determine whether to allow or deny each operation.

## How the Shim Works

### Loading
The shim is loaded by setting the `LD_PRELOAD` environment variable:
```bash
LD_PRELOAD=/path/to/intercept.so ./target_application
```

### Interception
The shim intercepts all `read()` and `write()` system calls by providing its own implementations that:
1. Connect to the monitoring socket
2. Request permission for the operation
3. Execute the real system call if allowed
4. Return an error (EPERM) if denied

## Socket Protocol

### Socket Location
The shim expects a Unix domain socket at: `/tmp/colony2-rwshim.sock`

### Request Format
The shim sends requests as ASCII text messages with the following format:
```
<OPERATION> <FD> <SIZE> <FILENAME>\n
```

Fields:
- `OPERATION`: Either "READ" or "WRITE"
- `FD`: File descriptor number (integer)
- `SIZE`: Number of bytes to read/write (size_t)
- `FILENAME`: Path to the file or special descriptor name

Example requests:
```
WRITE 1 21 stdout
READ 3 1024 /home/user/data.txt
WRITE 5 4096 /var/log/app.log
```

### Filename Resolution
The shim attempts to resolve filenames by:
1. Reading the symlink at `/proc/self/fd/<FD>`
2. For special descriptors, using friendly names:
   - fd 0 → "stdin"
   - fd 1 → "stdout"  
   - fd 2 → "stderr"
3. For unresolvable descriptors: "fd:<NUMBER>"

### Response Format
The monitoring application must respond with one of:
- `ALLOW\n` - Operation is permitted
- `DENY\n` - Operation is denied

The response must be sent before closing the connection.

## Monitoring Application Requirements

### 1. Socket Setup
```c
// Create Unix domain socket
int sock = socket(AF_UNIX, SOCK_STREAM, 0);

// Bind to /tmp/colony2-rwshim.sock
struct sockaddr_un addr = {0};
addr.sun_family = AF_UNIX;
strcpy(addr.sun_path, "/tmp/colony2-rwshim.sock");
bind(sock, (struct sockaddr*)&addr, sizeof(addr));

// Listen for connections
listen(sock, 5);
```

### 2. Request Handling
```c
// Accept connection
int client = accept(sock, NULL, NULL);

// Read request
char buffer[512];
recv(client, buffer, sizeof(buffer)-1, 0);

// Parse request
char op[16], filename[256];
int fd;
size_t size;
sscanf(buffer, "%15s %d %zu %255s", op, &fd, &size, filename);

// Make decision based on policy
bool allowed = check_policy(op, fd, size, filename);

// Send response
send(client, allowed ? "ALLOW\n" : "DENY\n", 6, 0);

// Close connection
close(client);
```

### 3. Connection Lifecycle
- Each operation creates a new connection
- The shim expects a response before the connection closes
- No persistent connections are maintained

### 4. Default Behavior
If the monitoring socket is not available:
- The shim allows all operations by default
- No error is reported to the application

## Implementation Considerations

### Performance
- Each intercepted call creates a new socket connection
- Consider the overhead for high-frequency I/O operations
- The monitoring application should respond quickly

### Security
- The socket should have appropriate permissions
- Consider using SO_PEERCRED to verify the calling process
- Validate all input from the shim

### Error Handling
- Handle partial reads/writes of the protocol messages
- Be prepared for malformed requests
- Clean up sockets properly on errors

### Threading
- The shim uses pthread_once for initialization
- The monitoring application should handle concurrent connections
- Consider thread safety in policy decisions

## Example Monitoring Application

See `mock_monitor.c` for a complete example that implements:
- Multiple policy modes (allow all, deny all, deny reads, deny writes)
- Proper socket lifecycle management
- Request parsing and response generation
- Signal handling for cleanup

## Testing

The shim can be tested by:
1. Starting the monitoring application
2. Running a test program with `LD_PRELOAD`
3. Verifying operations are intercepted and controlled

Example:
```bash
# Terminal 1: Start monitor
./monitor --policy=deny-writes

# Terminal 2: Run with shim
LD_PRELOAD=./intercept.so ./test_app
```

## Limitations

- Only works on Linux (uses `/proc/self/fd/` for filename resolution)
- Requires `LD_PRELOAD` support (may not work with setuid binaries)
- Cannot intercept statically linked programs
- Some applications may use direct system calls that bypass the shim