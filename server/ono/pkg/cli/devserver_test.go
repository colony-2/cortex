package cli

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
		DatabaseFile:  "./test.db",
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

func TestDevServerOptions(t *testing.T) {
	tests := []struct {
		name string
		opts DevServerOptions
	}{
		{
			name: "with default namespace",
			opts: DevServerOptions{
				FrontendIP:   "127.0.0.1",
				FrontendPort: 7233,
				Namespaces:   []string{"default"},
				DatabaseFile: "./test.db",
			},
		},
		{
			name: "with multiple namespaces",
			opts: DevServerOptions{
				FrontendIP:   "127.0.0.1", 
				FrontendPort: 7233,
				Namespaces:   []string{"default", "testing", "production"},
				DatabaseFile: "./test.db",
			},
		},
		{
			name: "with empty namespaces",
			opts: DevServerOptions{
				FrontendIP:   "127.0.0.1",
				FrontendPort: 7233,
				Namespaces:   []string{},
				DatabaseFile: "./test.db",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewDevServer(tt.opts)
			assert.NoError(t, err)
			assert.NotNil(t, server)
			assert.Equal(t, tt.opts.Namespaces, server.options.Namespaces)
		})
	}
}
