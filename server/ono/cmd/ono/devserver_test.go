package ono

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDevServer(t *testing.T) {
	opts := DevServerOptions{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  7234,
		UIPort:        8234,
		Namespaces:    []string{"test"},
		DatabaseFile:  ":memory:",
		InMemory:      true,
		LogLevel:      "info",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	server, err := NewDevServer(opts)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, opts, server.options)
}

func TestBuildSQLiteAttributes(t *testing.T) {
	server := &DevServer{
		options: DevServerOptions{
			SQLitePragmas: map[string]string{
				"cache_size": "2000",
				"foreign_keys": "ON",
			},
		},
	}

	attrs := server.buildSQLiteAttributes()
	
	// Check default pragmas
	assert.Equal(t, "1", attrs["_pragma=journal_mode(WAL)"])
	assert.Equal(t, "1", attrs["_pragma=synchronous(NORMAL)"])
	assert.Equal(t, "1", attrs["_pragma=busy_timeout(10000)"])
	assert.Equal(t, "1", attrs["_pragma=temp_store(MEMORY)"])
	
	// Check custom pragmas
	assert.Equal(t, "1", attrs["_pragma=cache_size(2000)"])
	assert.Equal(t, "1", attrs["_pragma=foreign_keys(ON)"])
}