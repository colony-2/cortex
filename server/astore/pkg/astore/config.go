package astore

import (
	"os"
	"strconv"
	"strings"
)

// Limits constrains artifact persistence to avoid runaway usage.
type Limits struct {
	// MaxTotalBytes caps the sum of file sizes. Zero means no limit.
	MaxTotalBytes int64
	// MaxFiles caps the number of files scanned. Zero means no limit.
	MaxFiles int
	// Excludes skips any path segment matching one of these values.
	Excludes []string
}

// Config drives directory creation and persistence behavior.
type Config struct {
	Root   string
	Limits Limits
}

var defaultExcludes = []string{".git", ".gitignore", ".DS_Store"}

// DefaultConfig returns a config with temp-root and default excludes.
func DefaultConfig() Config {
	return Config{
		Root:   os.TempDir(),
		Limits: DefaultLimits(),
	}
}

// DefaultLimits returns the default limit set.
func DefaultLimits() Limits {
	return Limits{
		Excludes: append([]string(nil), defaultExcludes...),
	}
}

func (c Config) withEnv() Config {
	if root := strings.TrimSpace(os.Getenv("ASTORE_ROOT")); root != "" {
		c.Root = root
	}

	if maxBytes := strings.TrimSpace(os.Getenv("ASTORE_MAX_BYTES")); maxBytes != "" {
		if v, err := strconv.ParseInt(maxBytes, 10, 64); err == nil && v >= 0 {
			c.Limits.MaxTotalBytes = v
		}
	}

	if maxFiles := strings.TrimSpace(os.Getenv("ASTORE_MAX_FILES")); maxFiles != "" {
		if v, err := strconv.Atoi(maxFiles); err == nil && v >= 0 {
			c.Limits.MaxFiles = v
		}
	}

	if excludes := strings.TrimSpace(os.Getenv("ASTORE_EXCLUDES")); excludes != "" {
		parts := strings.Split(excludes, ",")
		var cleaned []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) > 0 {
			c.Limits.Excludes = cleaned
		}
	}

	return c
}

func (l Limits) withDefaults() Limits {
	out := l
	if len(out.Excludes) == 0 {
		out.Excludes = append([]string(nil), defaultExcludes...)
	}
	return out
}
