package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	embeddedtemporal "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
	"go.temporal.io/sdk/client"
)

// TestServer provides an embedded Temporal server for testing
type TestServer struct {
	server   *embeddedtemporal.Server
	client   client.Client
	dbPath   string
	hostPort string
}

// StartTestServer starts an embedded Temporal server for testing
func StartTestServer(t *testing.T) *TestServer {
	t.Helper()

	// Create temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test-temporal.db")

	// Configure server options
	opts := embeddedtemporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: embeddedtemporal.FindFreePort(),
		DatabaseFile: dbPath,
		Namespaces:   []string{"default", "test"},
		LogLevel:     "error", // Keep logs quiet during tests
		SQLitePragmas: map[string]string{
			"synchronous": "OFF", // Faster for tests
			"temp_store":  "MEMORY",
		},
	}

	// Create and start server
	server, err := embeddedtemporal.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create embedded temporal server: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start embedded temporal server: %v", err)
	}

	hostPort := server.GetFrontendAddress()

	// Create client
	c, err := embeddedtemporal.NewClient(embeddedtemporal.ClientOptions{
		HostPort:  hostPort,
		Namespace: "default",
	})
	if err != nil {
		server.Stop()
		t.Fatalf("failed to create temporal client: %v", err)
	}

	ts := &TestServer{
		server:   server,
		client:   c,
		dbPath:   dbPath,
		hostPort: hostPort,
	}

	// Register cleanup
	t.Cleanup(func() {
		ts.Cleanup()
	})

	return ts
}

// Client returns the Temporal client
func (ts *TestServer) Client() client.Client {
	return ts.client
}

// HostPort returns the server's host:port address
func (ts *TestServer) HostPort() string {
	return ts.hostPort
}

// Cleanup stops the server and cleans up resources
func (ts *TestServer) Cleanup() {
	if ts.client != nil {
		ts.client.Close()
	}
	if ts.server != nil {
		ts.server.Stop()
	}
	// Clean up database file
	os.RemoveAll(filepath.Dir(ts.dbPath))
}

// WaitForWorkerReady waits for a worker to be ready on the given task queue
func (ts *TestServer) WaitForWorkerReady(taskQueue string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		// Try to describe the task queue
		resp, err := ts.client.DescribeTaskQueue(nil, taskQueue, client.TaskQueueTypeWorkflow)
		if err == nil && resp != nil && len(resp.Pollers) > 0 {
			return nil
		}
		
		time.Sleep(100 * time.Millisecond)
	}
	
	return fmt.Errorf("timeout waiting for worker on task queue %s", taskQueue)
}