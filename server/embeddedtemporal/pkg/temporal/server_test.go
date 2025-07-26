package temporal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

func TestServerLifecycle(t *testing.T) {
	// Create temp directory for database
	tmpDir, err := os.MkdirTemp("", "embeddedtemporal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "temporal.db")

	// Create server options
	opts := temporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 17233,
		DatabaseFile: dbPath,
		LogLevel:     "error",
		Namespaces:   []string{"test-namespace"},
	}

	// Create server
	server, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Verify server is running by creating a client
	c, err := client.Dial(client.Options{
		HostPort:  server.GetFrontendAddress(),
		Namespace: client.DefaultNamespace,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Verify we can describe the system namespace
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: "temporal-system",
	})
	if err != nil {
		t.Fatalf("Failed to describe system namespace: %v", err)
	}

	if resp.NamespaceInfo == nil || resp.NamespaceInfo.Name != "temporal-system" {
		t.Fatal("System namespace not properly initialized")
	}

	// Verify custom namespace was created
	resp, err = c.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: "test-namespace",
	})
	if err != nil {
		t.Fatalf("Failed to describe test namespace: %v", err)
	}

	if resp.NamespaceInfo == nil || resp.NamespaceInfo.Name != "test-namespace" {
		t.Fatal("Test namespace not properly created")
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Database file was not created")
	}
}

func TestServerWithCustomPragmas(t *testing.T) {
	// Create temp directory for database
	tmpDir, err := os.MkdirTemp("", "embeddedtemporal-pragmas-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "temporal.db")

	// Create server options with custom pragmas
	opts := temporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 17234,
		DatabaseFile: dbPath,
		LogLevel:     "error",
		SQLitePragmas: map[string]string{
			"cache_size": "-64000", // 64MB cache
			"mmap_size":  "268435456", // 256MB mmap
		},
	}

	// Create and start server
	server, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Just verify it started successfully with custom pragmas
	if err := server.Stop(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestServerRestart(t *testing.T) {
	// Create temp directory for database
	tmpDir, err := os.MkdirTemp("", "embeddedtemporal-restart-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "temporal.db")

	// Create server options
	opts := temporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 17235,
		DatabaseFile: dbPath,
		LogLevel:     "error",
		Namespaces:   []string{"persistent-namespace"},
	}

	// First server instance
	server1, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create first server: %v", err)
	}

	if err := server1.Start(); err != nil {
		t.Fatalf("Failed to start first server: %v", err)
	}

	// Stop first server
	if err := server1.Stop(); err != nil {
		t.Fatalf("Failed to stop first server: %v", err)
	}

	// Create second server instance with same database
	server2, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create second server: %v", err)
	}

	if err := server2.Start(); err != nil {
		t.Fatalf("Failed to start second server: %v", err)
	}

	// Verify namespace still exists
	c, err := client.Dial(client.Options{
		HostPort:  server2.GetFrontendAddress(),
		Namespace: client.DefaultNamespace,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: "persistent-namespace",
	})
	if err != nil {
		t.Fatalf("Failed to describe persistent namespace: %v", err)
	}

	if resp.NamespaceInfo == nil || resp.NamespaceInfo.Name != "persistent-namespace" {
		t.Fatal("Persistent namespace not found after restart")
	}

	// Stop second server
	if err := server2.Stop(); err != nil {
		t.Fatalf("Failed to stop second server: %v", err)
	}
}

func TestPortAvailability(t *testing.T) {
	// Test IsPortAvailable function
	if !temporal.IsPortAvailable("127.0.0.1", 0) {
		t.Fatal("Port 0 should always be available")
	}

	// Find a free port
	port := temporal.FindFreePort()
	if port <= 0 {
		t.Fatal("FindFreePort should return a valid port")
	}

	// Verify the port is actually available
	if !temporal.IsPortAvailable("127.0.0.1", port) {
		t.Fatalf("Port %d should be available", port)
	}
}

func TestClientCreation(t *testing.T) {
	// Create temp directory for database
	tmpDir, err := os.MkdirTemp("", "embeddedtemporal-client-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "temporal.db")

	// Create and start server
	opts := temporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 17236,
		DatabaseFile: dbPath,
		LogLevel:     "error",
		Namespaces:   []string{"client-test"},
	}

	server, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Test NewClient helper
	c, err := temporal.NewClient(temporal.ClientOptions{
		HostPort:  server.GetFrontendAddress(),
		Namespace: "client-test",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Verify client works
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: "client-test",
	})
	if err != nil {
		t.Fatalf("Failed to describe namespace: %v", err)
	}

	if resp.NamespaceInfo == nil || resp.NamespaceInfo.Name != "client-test" {
		t.Fatal("Client not properly connected")
	}

	// Test NewNamespaceClient helper
	nc, err := temporal.NewNamespaceClient(server.GetFrontendAddress())
	if err != nil {
		t.Fatalf("Failed to create namespace client: %v", err)
	}
	defer nc.Close()

	// List namespaces to verify it works
	_, err = nc.Describe(ctx, "temporal-system")
	if err != nil {
		t.Fatalf("Failed to describe system namespace with namespace client: %v", err)
	}
}