package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration
type Config struct {
	Port       int
	RootPath   string
	CreateNew  bool
}

// Validate validates and completes the configuration
func (c *Config) Validate() error {
	// Resolve root path
	absPath, err := filepath.Abs(c.RootPath)
	if err != nil {
		return fmt.Errorf("invalid root path: %w", err)
	}
	c.RootPath = absPath

	// Check if root path exists or create if --new flag is set
	if _, err := os.Stat(c.RootPath); os.IsNotExist(err) {
		if c.CreateNew {
			if err := os.MkdirAll(c.RootPath, 0755); err != nil {
				return fmt.Errorf("failed to create root path: %w", err)
			}
		} else {
			return fmt.Errorf("root path does not exist: %s", c.RootPath)
		}
	}

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	return nil
}