package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

var (
	ErrInvalidConfig     = errors.New("invalid configuration")
	ErrNameRequired      = errors.New("name is required")
	ErrRecipesPathRequired = errors.New("recipes path is required")
	ErrInvalidName       = errors.New("name must contain only alphanumeric characters and hyphens")
	ErrRecipesPathNotExist = errors.New("recipes path does not exist")
	ErrRecipesPathNotDir   = errors.New("recipes path is not a directory")
	ErrInvalidTemporalServer = errors.New("invalid temporal server address")
)

type Config struct {
	Name           string
	TemporalServer string
	RecipesPath    string
	Namespace      string
	Debug          bool
}

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)

func (c *Config) Validate() error {
	if c.Name == "" {
		return ErrNameRequired
	}

	if !nameRegex.MatchString(c.Name) {
		return ErrInvalidName
	}

	if c.RecipesPath == "" {
		return ErrRecipesPathRequired
	}

	absPath, err := filepath.Abs(c.RecipesPath)
	if err != nil {
		return fmt.Errorf("failed to resolve recipes path: %w", err)
	}
	c.RecipesPath = absPath

	info, err := os.Stat(c.RecipesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrRecipesPathNotExist
		}
		return fmt.Errorf("failed to stat recipes path: %w", err)
	}

	if !info.IsDir() {
		return ErrRecipesPathNotDir
	}

	if err := c.validateTemporalServer(); err != nil {
		return err
	}

	if c.Namespace == "" {
		c.Namespace = "default"
	}

	return nil
}

func (c *Config) validateTemporalServer() error {
	if c.TemporalServer == "" {
		c.TemporalServer = "localhost:7233"
		return nil
	}

	u, err := url.Parse("http://" + c.TemporalServer)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidTemporalServer, err)
	}

	if u.Hostname() == "" {
		return fmt.Errorf("%w: missing hostname", ErrInvalidTemporalServer)
	}

	if u.Port() == "" {
		return fmt.Errorf("%w: missing port", ErrInvalidTemporalServer)
	}

	return nil
}