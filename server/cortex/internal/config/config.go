package config

import (
	"fmt"
)

// Config holds the application configuration
type Config struct {
	Port int
}

// Validate validates and completes the configuration
func (c *Config) Validate() error {
	// Resolve root path

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	return nil
}
