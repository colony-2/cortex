package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration
type Config struct {
	Port         int
	RootPath     string
	DatabasePath string
	CORSOrigins  []string
	Production   bool
	StaticPath   string
}

// Validate validates and completes the configuration
func (c *Config) Validate() error {
	// Resolve root path
	absPath, err := filepath.Abs(c.RootPath)
	if err != nil {
		return fmt.Errorf("invalid root path: %w", err)
	}
	c.RootPath = absPath

	// Check if root path exists
	if _, err := os.Stat(c.RootPath); os.IsNotExist(err) {
		return fmt.Errorf("root path does not exist: %s", c.RootPath)
	}

	// Set default database path if not specified
	if c.DatabasePath == "" {
		c.DatabasePath = filepath.Join(c.RootPath, ".vibethis")
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(c.DatabasePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	// Set static path for production mode
	if c.Production && c.StaticPath == "" {
		// In production, static files are embedded
		c.StaticPath = "embedded"
	}

	return nil
}