package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the application configuration
type Config struct {
	Port         int
	StoragePath  string
	StaticPath   string
	CORSOrigins  []string
	DatabaseDSN  string
	CreateNew    bool
	InitializeDB bool
	StrataMode   string
	StrataURL    string
	StrataAPIKey string
}

// Validate validates and completes the configuration
func (c *Config) Validate() error {
	if c.StoragePath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		absWD, err := filepath.Abs(wd)
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		c.StoragePath = filepath.Join(absWD, ".colony2")
	}
	if c.StaticPath == "" {
		c.StaticPath = "embedded"
	}
	if c.StrataMode == "" {
		c.StrataMode = "embedded"
	}
	c.StrataMode = strings.ToLower(c.StrataMode)
	if c.StrataMode != "embedded" && c.StrataMode != "remote" {
		return fmt.Errorf("invalid strata-mode: %s", c.StrataMode)
	}
	if c.StrataMode == "remote" && c.StrataURL == "" {
		return fmt.Errorf("strata-url is required when strata-mode=remote")
	}

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	return nil
}
