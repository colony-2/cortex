package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Config holds CLI configuration values.
type Config struct {
	APIURL  string        `yaml:"api_url"`
	Token   string        `yaml:"token"`
	Project string        `yaml:"project"`
	Output  string        `yaml:"output"`  // table|json
	Timeout time.Duration `yaml:"timeout"` // e.g. 30s
	Trace   bool          `yaml:"trace"`
}

// Default returns the baseline configuration.
func Default() Config {
	return Config{
		APIURL: "http://localhost:8080",
		Output: "table",
		Timeout: 30 * time.Second,
	}
}

// DefaultPath returns the default config file path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "colony2", "config.yaml")
}

// FromEnv builds a Config from environment variables.
func FromEnv() Config {
	var cfg Config
	if v := os.Getenv("COLONY2_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if v := os.Getenv("COLONY2_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("COLONY2_PROJECT"); v != "" {
		cfg.Project = v
	}
	if v := os.Getenv("COLONY2_OUTPUT"); v != "" {
		cfg.Output = v
	}
	if v := os.Getenv("COLONY2_TRACE"); v != "" {
		cfg.Trace = strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("COLONY2_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Timeout = d
		}
	}
	return cfg
}

// LoadFile loads configuration from a YAML file. Missing file is not an error.
func LoadFile(path string) (Config, error) {
	if path == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Merge overlays the given configs onto the base in order.
func Merge(base Config, overlays ...Config) Config {
	out := base
	for _, o := range overlays {
		if o.APIURL != "" {
			out.APIURL = o.APIURL
		}
		if o.Token != "" {
			out.Token = o.Token
		}
		if o.Project != "" {
			out.Project = o.Project
		}
		if o.Output != "" {
			out.Output = o.Output
		}
		if o.Timeout > 0 {
			out.Timeout = o.Timeout
		}
		if o.Trace {
			out.Trace = true
		}
	}
	return out
}

// Validate ensures the configuration is usable.
func (c Config) Validate(requireProject bool) error {
	if c.APIURL == "" {
		return errors.New("api url is required (flag --api-url or COLONY2_API_URL)")
	}
	switch strings.ToLower(c.Output) {
	case "table", "json":
	default:
		return fmt.Errorf("invalid output format %q (expected table|json)", c.Output)
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if requireProject && c.Project == "" {
		return errors.New("project id is required (flag --project or COLONY2_PROJECT)")
	}
	return nil
}
