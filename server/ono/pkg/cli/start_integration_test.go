//go:build integration
// +build integration

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibethis/server/embeddedtemporal"
)

func TestDevServerStartStop(t *testing.T) {
	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	opts := embeddedtemporal.Options{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  17233, // Use non-standard port to avoid conflicts
		UIPort:        18233,
		Namespaces:    []string{"default"},
		DatabaseFile:  dbPath,
		LogLevel:      "warn",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	// Create dev server
	devServer, err := embeddedtemporal.NewServer(opts)
	require.NoError(t, err)
	require.NotNil(t, devServer)

	// Start server in background
	startErr := make(chan error, 1)
	go func() {
		startErr <- devServer.Start()
	}()

	// Wait a bit for server to start or fail
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Failed to start server: %v", err)
		}
	case <-time.After(5 * time.Second):
		// Server is likely running
	}

	// Give it a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Check that database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "Database file should exist")

	// Stop the server
	err = devServer.Stop()
	assert.NoError(t, err)
}

func TestDevServerWithCustomNamespace(t *testing.T) {
	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-custom.db")

	opts := embeddedtemporal.Options{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  17234, // Different port
		UIPort:        18234,
		Namespaces:    []string{"test-namespace"},
		DatabaseFile:  dbPath,
		LogLevel:      "error",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	devServer, err := embeddedtemporal.NewServer(opts)
	require.NoError(t, err)

	// Start server in background
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	startErr := make(chan error, 1)
	go func() {
		startErr <- devServer.Start()
	}()

	// Wait for start or timeout
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Failed to start server with custom namespace: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for server to start")
	case <-time.After(3 * time.Second):
		// Likely started successfully
	}

	// Check that database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "Database file should exist")

	// Stop the server
	err = devServer.Stop()
	assert.NoError(t, err)
}

// TestBuildSQLiteAttributesIntegration has been removed as it tested internal implementation details
// that are now handled within the embeddedtemporal package

// TestDevServerConfig has been removed as it tested internal implementation details
// that are now handled within the embeddedtemporal package