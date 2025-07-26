package cli

import (
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
)

func TestDevServerRestart(t *testing.T) {
	// Test that server can be restarted after stopping
	opts := temporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 17233, // Use a non-default port to avoid conflicts
		UIPort:       18233,
		Namespaces:   []string{"test-namespace"},
		DatabaseFile: t.TempDir() + "/test.db",
		LogLevel:     "error", // Reduce log noise in tests
		EnableUI:     false,
	}

	// First start
	server1, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create first server: %v", err)
	}

	err = server1.Start()
	if err != nil {
		t.Fatalf("Failed to start first server: %v", err)
	}

	// Give server time to fully start
	time.Sleep(2 * time.Second)

	// Stop first server
	err = server1.Stop()
	if err != nil {
		t.Fatalf("Failed to stop first server: %v", err)
	}

	// Wait a moment to ensure cleanup
	time.Sleep(1 * time.Second)

	// Second start - this should work without errors
	server2, err := temporal.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create second server: %v", err)
	}

	err = server2.Start()
	if err != nil {
		t.Fatalf("Failed to start second server (restart failed): %v", err)
	}

	// Give server time to fully start
	time.Sleep(2 * time.Second)

	// Clean up
	err = server2.Stop()
	if err != nil {
		t.Fatalf("Failed to stop second server: %v", err)
	}
}