package rwshim

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMonitorStartStop(t *testing.T) {
	monitor := NewMonitor(AllowAll)

	// Test starting monitor
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	if !monitor.IsRunning() {
		t.Error("Monitor should be running after Start()")
	}

	// Check socket exists
	if _, err := os.Stat(monitor.SocketPath()); os.IsNotExist(err) {
		t.Error("Socket file should exist after Start()")
	}

	// Test stopping monitor
	err = monitor.Stop()
	if err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	if monitor.IsRunning() {
		t.Error("Monitor should not be running after Stop()")
	}

	// Check socket cleaned up
	if _, err := os.Stat(monitor.SocketPath()); !os.IsNotExist(err) {
		t.Error("Socket file should be removed after Stop()")
	}
}

func TestMonitorDoubleStart(t *testing.T) {
	monitor := NewMonitor(AllowAll)

	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Try to start again
	err = monitor.Start()
	if err == nil {
		t.Error("Starting an already running monitor should return an error")
	}
}

func TestCustomSocketPath(t *testing.T) {
	customPath := "/tmp/test-rwshim.sock"
	monitor := NewMonitorWithPath(customPath, AllowAll)

	if monitor.SocketPath() != customPath {
		t.Errorf("Expected socket path %s, got %s", customPath, monitor.SocketPath())
	}

	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Check custom socket exists
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Error("Custom socket file should exist")
	}
}

func TestPolicyFunctions(t *testing.T) {
	readReq := Request{Operation: OpRead, FD: 3, Size: 1024, Filename: "/test.txt"}
	writeReq := Request{Operation: OpWrite, FD: 3, Size: 1024, Filename: "/test.txt"}

	// Test AllowAll
	if resp := AllowAll(readReq); !resp.Allow {
		t.Error("AllowAll should allow read operations")
	}
	if resp := AllowAll(writeReq); !resp.Allow {
		t.Error("AllowAll should allow write operations")
	}

	// Test DenyAll
	if resp := DenyAll(readReq); resp.Allow {
		t.Error("DenyAll should deny read operations")
	}
	if resp := DenyAll(writeReq); resp.Allow {
		t.Error("DenyAll should deny write operations")
	}

	// Test DenyWrites
	if resp := DenyWrites(readReq); !resp.Allow {
		t.Error("DenyWrites should allow read operations")
	}
	if resp := DenyWrites(writeReq); resp.Allow {
		t.Error("DenyWrites should deny write operations")
	}

	// Test DenyReads
	if resp := DenyReads(readReq); resp.Allow {
		t.Error("DenyReads should deny read operations")
	}
	if resp := DenyReads(writeReq); !resp.Allow {
		t.Error("DenyReads should allow write operations")
	}
}

func TestMonitorWithMockClient(t *testing.T) {
	// Track requests
	var mu sync.Mutex
	var requests []Request
	
	policy := func(req Request) Response {
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		
		// Deny writes to /secure
		if req.Operation == OpWrite && strings.HasPrefix(req.Filename, "/secure") {
			return Response{Allow: false}
		}
		return Response{Allow: true}
	}

	monitor := NewMonitor(policy)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Test cases
	tests := []struct {
		request  string
		expected string
	}{
		{"READ 3 1024 /home/user/file.txt\n", "ALLOW\n"},
		{"WRITE 1 42 stdout\n", "ALLOW\n"},
		{"WRITE 5 100 /secure/file.txt\n", "DENY\n"},
		{"READ 5 100 /secure/file.txt\n", "ALLOW\n"},
	}

	for i, tt := range tests {
		// Connect as a client
		conn, err := net.Dial("unix", monitor.SocketPath())
		if err != nil {
			t.Fatalf("Test %d: Failed to connect: %v", i, err)
		}

		// Send request
		_, err = conn.Write([]byte(tt.request))
		if err != nil {
			conn.Close()
			t.Fatalf("Test %d: Failed to send request: %v", i, err)
		}

		// Read response
		buf := make([]byte, 10)
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			t.Fatalf("Test %d: Failed to read response: %v", i, err)
		}
		conn.Close()

		response := string(buf[:n])
		if response != tt.expected {
			t.Errorf("Test %d: Expected %q, got %q", i, tt.expected, response)
		}
	}

	// Verify requests were tracked
	mu.Lock()
	if len(requests) != len(tests) {
		t.Errorf("Expected %d requests, got %d", len(tests), len(requests))
	}
	mu.Unlock()
}

func TestPolicyBuilder(t *testing.T) {
	policy := NewPolicyBuilder().
		AllowRead(MatchFilenamePrefix("/allowed/")).
		DenyWrite(MatchFilenameSuffix(".protected")).
		AllowWrite(MatchStdStreams()).
		Default(false).
		Build()

	tests := []struct {
		name     string
		request  Request
		expected bool
	}{
		{
			"allow read from /allowed/",
			Request{Operation: OpRead, FD: 3, Size: 100, Filename: "/allowed/file.txt"},
			true,
		},
		{
			"deny read from /other/",
			Request{Operation: OpRead, FD: 3, Size: 100, Filename: "/other/file.txt"},
			false,
		},
		{
			"deny write to .protected",
			Request{Operation: OpWrite, FD: 3, Size: 100, Filename: "/data/file.protected"},
			false,
		},
		{
			"allow write to stdout",
			Request{Operation: OpWrite, FD: 1, Size: 100, Filename: "stdout"},
			true,
		},
		{
			"deny write to regular file",
			Request{Operation: OpWrite, FD: 3, Size: 100, Filename: "/data/file.txt"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := policy(tt.request)
			if resp.Allow != tt.expected {
				t.Errorf("Expected Allow=%v, got %v", tt.expected, resp.Allow)
			}
		})
	}
}

func TestMatchers(t *testing.T) {
	// Test MatchFilename
	matcher := MatchFilename("/exact/match.txt")
	if !matcher(Request{Filename: "/exact/match.txt"}) {
		t.Error("MatchFilename should match exact filename")
	}
	if matcher(Request{Filename: "/exact/other.txt"}) {
		t.Error("MatchFilename should not match different filename")
	}

	// Test MatchFilenamePrefix
	matcher = MatchFilenamePrefix("/prefix/")
	if !matcher(Request{Filename: "/prefix/file.txt"}) {
		t.Error("MatchFilenamePrefix should match prefix")
	}
	if matcher(Request{Filename: "/other/file.txt"}) {
		t.Error("MatchFilenamePrefix should not match without prefix")
	}

	// Test MatchFilenameSuffix
	matcher = MatchFilenameSuffix(".log")
	if !matcher(Request{Filename: "/var/app.log"}) {
		t.Error("MatchFilenameSuffix should match suffix")
	}
	if matcher(Request{Filename: "/var/app.txt"}) {
		t.Error("MatchFilenameSuffix should not match without suffix")
	}

	// Test MatchFD
	matcher = MatchFD(5)
	if !matcher(Request{FD: 5}) {
		t.Error("MatchFD should match FD")
	}
	if matcher(Request{FD: 3}) {
		t.Error("MatchFD should not match different FD")
	}

	// Test MatchStdStreams
	matcher = MatchStdStreams()
	for _, fd := range []int{0, 1, 2} {
		if !matcher(Request{FD: fd}) {
			t.Errorf("MatchStdStreams should match FD %d", fd)
		}
	}
	for _, name := range []string{"stdin", "stdout", "stderr"} {
		if !matcher(Request{Filename: name}) {
			t.Errorf("MatchStdStreams should match %s", name)
		}
	}
	if matcher(Request{FD: 5, Filename: "/file.txt"}) {
		t.Error("MatchStdStreams should not match non-std stream")
	}

	// Test MatchLargeOperations
	matcher = MatchLargeOperations(1000)
	if !matcher(Request{Size: 1001}) {
		t.Error("MatchLargeOperations should match size > threshold")
	}
	if matcher(Request{Size: 1000}) {
		t.Error("MatchLargeOperations should not match size <= threshold")
	}
}

func TestConcurrentConnections(t *testing.T) {
	var counter int
	var mu sync.Mutex

	policy := func(req Request) Response {
		mu.Lock()
		counter++
		mu.Unlock()
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		return Response{Allow: true}
	}

	monitor := NewMonitor(policy)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Launch multiple concurrent connections
	numConnections := 10
	var wg sync.WaitGroup
	wg.Add(numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			defer wg.Done()
			
			conn, err := net.Dial("unix", monitor.SocketPath())
			if err != nil {
				t.Errorf("Connection %d: Failed to connect: %v", id, err)
				return
			}
			defer conn.Close()

			request := fmt.Sprintf("READ %d 1024 /file%d.txt\n", id, id)
			conn.Write([]byte(request))

			buf := make([]byte, 10)
			n, err := conn.Read(buf)
			if err != nil {
				t.Errorf("Connection %d: Failed to read response: %v", id, err)
				return
			}

			if string(buf[:n]) != "ALLOW\n" {
				t.Errorf("Connection %d: Unexpected response: %s", id, string(buf[:n]))
			}
		}(i)
	}

	wg.Wait()

	// Verify all connections were processed
	mu.Lock()
	if counter != numConnections {
		t.Errorf("Expected %d connections processed, got %d", numConnections, counter)
	}
	mu.Unlock()
}

// Test helpers for creating temporary files
func createTempExecutable(t *testing.T, content string) string {
	tmpfile, err := ioutil.TempFile("", "test-*.sh")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}
	
	if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
		t.Fatalf("Failed to chmod temp file: %v", err)
	}
	
	return tmpfile.Name()
}

func TestMonitorShutdownCleanup(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	
	// Start and stop multiple times
	for i := 0; i < 3; i++ {
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Iteration %d: Failed to start: %v", i, err)
		}
		
		// Verify socket exists
		if _, err := os.Stat(monitor.SocketPath()); os.IsNotExist(err) {
			t.Errorf("Iteration %d: Socket should exist", i)
		}
		
		err = monitor.Stop()
		if err != nil {
			t.Fatalf("Iteration %d: Failed to stop: %v", i, err)
		}
		
		// Verify socket is cleaned up
		if _, err := os.Stat(monitor.SocketPath()); !os.IsNotExist(err) {
			t.Errorf("Iteration %d: Socket should be cleaned up", i)
		}
	}
}

func TestMonitorContextCancellation(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// Create a client connection
	conn, err := net.Dial("unix", monitor.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Stop the monitor while connection is active
	err = monitor.Stop()
	if err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	// Try to send data on the connection - should fail
	_, err = conn.Write([]byte("READ 1 100 test.txt\n"))
	if err == nil {
		// Read might also fail
		buf := make([]byte, 10)
		_, err = conn.Read(buf)
	}
	conn.Close()

	// Verify monitor is stopped
	if monitor.IsRunning() {
		t.Error("Monitor should not be running after Stop()")
	}
}