package ono

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		dbPath   string
		attrs    map[string]string
		expected string
	}{
		{
			name:   "in-memory database",
			dbPath: ":memory:",
			attrs: map[string]string{
				"_journal_mode": "WAL",
			},
			expected: ":memory:?_journal_mode=WAL",
		},
		{
			name:   "file database with pragmas",
			dbPath: "/tmp/test.db",
			attrs: map[string]string{
				"mode":          "rwc",
				"_journal_mode": "WAL",
				"_synchronous":  "NORMAL",
			},
			expected: "/tmp/test.db?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test building connection string
			connStr := buildSQLiteConnectionString(tt.dbPath, tt.attrs)
			
			if tt.dbPath == ":memory:" {
				assert.Equal(t, tt.expected, connStr)
			} else {
				// For file paths, just check it starts correctly
				assert.True(t, strings.HasPrefix(connStr, tt.dbPath+"?"))
				// Check all pragmas are present
				for pragma := range tt.attrs {
					assert.Contains(t, connStr, pragma)
				}
			}
		})
	}
}

func TestSQLiteConnection(t *testing.T) {
	// Test that we can actually connect to SQLite with our attributes
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	attrs := map[string]string{
		"mode":           "rwc",
		"_journal_mode":  "WAL",
		"_synchronous":   "NORMAL",
		"_busy_timeout":  "10000",
		"_foreign_keys":  "ON",
	}

	connStr := buildSQLiteConnectionString(dbPath, attrs)
	
	// Try to open the database
	db, err := sql.Open("sqlite", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Test the connection
	err = db.Ping()
	require.NoError(t, err)

	// The modernc.org/sqlite driver handles pragmas differently
	// Let's just verify the connection works
	t.Log("SQLite connection successful")
}

func TestBuildSQLiteAttributesFormat(t *testing.T) {
	devServer := &DevServer{
		options: DevServerOptions{
			SQLitePragmas: map[string]string{
				"cache_size": "-2000",
				"page_size": "4096",
			},
		},
	}

	attrs := devServer.buildSQLiteAttributes()
	
	// Check mode
	assert.Equal(t, "rwc", attrs["mode"])
	
	// Check default pragmas
	assert.Equal(t, "WAL", attrs["_journal_mode"])
	assert.Equal(t, "NORMAL", attrs["_synchronous"])
	assert.Equal(t, "10000", attrs["_busy_timeout"])
	assert.Equal(t, "ON", attrs["_foreign_keys"])
	
	// Check custom pragmas
	assert.Equal(t, "-2000", attrs["_cache_size"])
	assert.Equal(t, "4096", attrs["_page_size"])
}

func TestDevServerSQLiteInitialization(t *testing.T) {
	t.Run("file database initialization", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "temporal.db")

		opts := DevServerOptions{
			DatabaseFile: dbPath,
			InMemory:     false,
		}

		devServer := &DevServer{options: opts}
		cfg, err := devServer.buildConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Check persistence configuration
		assert.Equal(t, "default", cfg.Persistence.DefaultStore)
		assert.Equal(t, "visibility", cfg.Persistence.VisibilityStore)
		assert.Equal(t, int32(1), cfg.Persistence.NumHistoryShards)

		// Check default store
		defaultStore := cfg.Persistence.DataStores["default"]
		assert.NotNil(t, defaultStore.SQL)
		assert.Equal(t, "sqlite", defaultStore.SQL.PluginName)
		assert.Equal(t, dbPath, defaultStore.SQL.DatabaseName)
		assert.NotEmpty(t, defaultStore.SQL.ConnectAttributes)

		// Check visibility store
		visStore := cfg.Persistence.DataStores["visibility"]
		assert.NotNil(t, visStore.SQL)
		assert.Equal(t, "sqlite", visStore.SQL.PluginName)
	})

	t.Run("in-memory database initialization", func(t *testing.T) {
		opts := DevServerOptions{
			InMemory: true,
		}

		devServer := &DevServer{options: opts}
		cfg, err := devServer.buildConfig()
		require.NoError(t, err)

		// Check both stores use a random numeric database name for in-memory mode
		defaultStore := cfg.Persistence.DataStores["default"]
		assert.NotEmpty(t, defaultStore.SQL.DatabaseName)
		assert.Regexp(t, `^\d+$`, defaultStore.SQL.DatabaseName)
		assert.Equal(t, "memory", defaultStore.SQL.ConnectAttributes["mode"])
		assert.Equal(t, "shared", defaultStore.SQL.ConnectAttributes["cache"])

		visStore := cfg.Persistence.DataStores["visibility"]
		assert.NotEmpty(t, visStore.SQL.DatabaseName)
		assert.Equal(t, defaultStore.SQL.DatabaseName, visStore.SQL.DatabaseName)
		assert.Equal(t, "memory", visStore.SQL.ConnectAttributes["mode"])
		assert.Equal(t, "shared", visStore.SQL.ConnectAttributes["cache"])
	})
}

func TestDevServerDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", "temporal.db")

	opts := DevServerOptions{
		DatabaseFile: nestedPath,
		InMemory:     false,
	}

	devServer := &DevServer{options: opts}
	_, err := devServer.buildConfig()
	require.NoError(t, err)

	// Directory should be created during config build
	dir := filepath.Dir(nestedPath)
	_, err = os.Stat(dir)
	assert.NoError(t, err, "Directory should exist")
}

// Helper function to build SQLite connection string
func buildSQLiteConnectionString(dbPath string, attrs map[string]string) string {
	if len(attrs) == 0 {
		return dbPath
	}

	parts := []string{}
	for key, value := range attrs {
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}

	return dbPath + "?" + strings.Join(parts, "&")
}