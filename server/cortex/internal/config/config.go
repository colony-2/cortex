package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration
type Config struct {
	Port        int
	RootPath    string
	StoragePath string
	StaticPath  string
	CORSOrigins []string
	DatabaseDSN string
	CreateNew   bool
}

// Validate validates and completes the configuration
func (c *Config) Validate() error {
	if c.RootPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve root path: %w", err)
		}
		c.RootPath = wd
	}
	absRoot, err := filepath.Abs(c.RootPath)
	if err != nil {
		return fmt.Errorf("resolve root path: %w", err)
	}
	c.RootPath = absRoot

	if c.StoragePath == "" {
		c.StoragePath = filepath.Join(c.RootPath, ".colony2")
	}
	if c.StaticPath == "" {
		c.StaticPath = "embedded"
	}

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	return nil
}
