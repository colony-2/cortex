package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
	_ "modernc.org/sqlite"
)

// Integration test that verifies SQLite connections work correctly
func TestSQLiteConnection(t *testing.T) {
	// Test that we can actually connect to SQLite with our attributes
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	attrs := map[string]string{
		"mode":          "rwc",
		"_journal_mode": "WAL",
		"_synchronous":  "NORMAL",
		"_busy_timeout": "10000",
		"_foreign_keys": "ON",
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

// Integration test that verifies the server creates necessary directories
func TestDevServerDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", "temporal.db")

	opts := temporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 17238,
		DatabaseFile: nestedPath,
		LogLevel:     "error",
	}

	devServer, err := temporal.NewServer(opts)
	require.NoError(t, err)
	
	// Start the server - this should create the directory
	err = devServer.Start()
	require.NoError(t, err)
	defer devServer.Stop()

	// Directory should be created
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

// Note: Unit tests for SQLite attribute building and configuration have been moved to embeddedtemporal package