package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewDevServer tests that we can create a DevServer instance
// This is now essentially a wrapper test since DevServer is a type alias
func TestNewDevServer(t *testing.T) {
	opts := DevServerOptions{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  7234,
		UIPort:        8234,
		Namespaces:    []string{"test"},
		DatabaseFile:  "./test.db",
		LogLevel:      "info",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	server, err := NewDevServer(opts)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	// Note: We can't access internal fields anymore since DevServer is now a type alias
	// The actual implementation tests are in the embeddedtemporal package
}

// Unit tests for configuration and attribute building have been moved to embeddedtemporal package
// Integration tests remain in other test files