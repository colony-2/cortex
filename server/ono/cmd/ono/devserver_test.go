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
				"temp_store": "MEMORY",
			},
		},
	}

	attrs := server.buildSQLiteAttributes()
	
	// Check default attributes
	assert.Equal(t, "rwc", attrs["mode"])
	assert.Equal(t, "WAL", attrs["_journal_mode"])
	assert.Equal(t, "NORMAL", attrs["_synchronous"])
	assert.Equal(t, "10000", attrs["_busy_timeout"])
	assert.Equal(t, "ON", attrs["_foreign_keys"])
	
	// Check custom pragmas
	assert.Equal(t, "2000", attrs["_cache_size"])
	assert.Equal(t, "MEMORY", attrs["_temp_store"])
}