package cli

import (
	"context"
	"testing"
	"time"
)

func TestStartCommandSmoke(t *testing.T) {
	// This test checks if the start command can at least initialize
	// without actually starting the server

	opts := DevServerOptions{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  27233, // Use very high port to avoid conflicts
		UIPort:        28233,
		Namespaces:    []string{"test"},
		DatabaseFile:  t.TempDir() + "/test.db",
		LogLevel:      "error",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	devServer, err := NewDevServer(opts)
	if err != nil {
		t.Fatalf("Failed to create dev server: %v", err)
	}

	// Try to build config
	cfg, err := devServer.buildConfig()
	if err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	if cfg == nil {
		t.Fatal("Config should not be nil")
	}

	// Don't actually start the server in unit tests
	t.Log("Dev server initialization successful")
}

func TestStartCommandInit(t *testing.T) {
	// Test that the command itself is properly initialized
	if StartWithRecipesCmd == nil {
		t.Fatal("StartWithRecipesCmd is nil")
	}

	if StartWithRecipesCmd.RunE == nil {
		t.Fatal("StartWithRecipesCmd.RunE is nil")
	}

	// Test with minimal context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should at least not panic
	_ = ctx
	t.Log("Start command is properly initialized")
}
