package internal

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MonitorImpl manages the Unix domain socket server and intercepts I/O operations
type MonitorImpl struct {
	socketPath string
	listener   net.Listener
	policy     PolicyFunc
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    bool
}

// NewMonitorImpl creates a new MonitorImpl with the given socket path and policy function
func NewMonitorImpl(socketPath string, policy PolicyFunc) *MonitorImpl {
	return &MonitorImpl{
		socketPath: socketPath,
		policy:     policy,
	}
}

// Start begins listening on the Unix domain socket
func (m *MonitorImpl) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("monitor is already running")
	}

	// Remove existing socket file if it exists
	os.Remove(m.socketPath)

	// Create Unix domain socket
	listener, err := net.Listen("unix", m.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}

	m.listener = listener
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.running = true

	// Start accepting connections
	m.wg.Add(1)
	go m.acceptLoop()

	return nil
}

// Stop gracefully shuts down the monitor
func (m *MonitorImpl) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	// Cancel context to signal shutdown
	m.cancel()

	// Close listener
	if m.listener != nil {
		m.listener.Close()
	}

	// Wait for all goroutines to finish
	m.wg.Wait()

	// Clean up socket file
	os.Remove(m.socketPath)

	m.running = false
	return nil
}

// acceptLoop handles incoming connections
func (m *MonitorImpl) acceptLoop() {
	defer m.wg.Done()

	for {
		conn, err := m.listener.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-m.ctx.Done():
				return
			default:
				// Log error and continue
				continue
			}
		}

		// Handle connection in a new goroutine
		m.wg.Add(1)
		go m.handleConnection(conn)
	}
}

// handleConnection processes a single client connection
func (m *MonitorImpl) handleConnection(conn net.Conn) {
	defer m.wg.Done()
	defer conn.Close()

	// Set read deadline to prevent blocking forever
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read request
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	// Parse request
	req, err := parseRequest(strings.TrimSpace(line))
	if err != nil {
		return
	}

	// Apply policy
	resp := m.policy(*req)

	// Send response
	responseStr := "DENY\n"
	if resp.Allow {
		responseStr = "ALLOW\n"
	}
	conn.Write([]byte(responseStr))
}

// parseRequest parses a request line into a Request struct
func parseRequest(line string) (*Request, error) {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid request format")
	}

	op := Operation(parts[0])
	if op != OpRead && op != OpWrite {
		return nil, fmt.Errorf("invalid operation: %s", parts[0])
	}

	fd, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid FD: %s", parts[1])
	}

	size, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid size: %s", parts[2])
	}

	// Join remaining parts as filename (in case it contains spaces)
	filename := strings.Join(parts[3:], " ")

	return &Request{
		Operation: op,
		FD:        fd,
		Size:      size,
		Filename:  filename,
	}, nil
}

// IsRunning returns whether the monitor is currently running
func (m *MonitorImpl) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// SocketPath returns the path to the Unix domain socket
func (m *MonitorImpl) SocketPath() string {
	return m.socketPath
}

// StartProcess starts a new process with the shim library preloaded
func (m *MonitorImpl) StartProcess(ctx context.Context, command string, args []string, opts ...ProcessOption) (*ProcessImpl, error) {
	return NewProcess(ctx, m, command, args, opts...)
}