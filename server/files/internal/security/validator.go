package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Validator validates file paths for security
type Validator interface {
	ValidatePath(path string) error
}

// PathValidator implements path validation
type PathValidator struct {
	rootPath string
}

// NewValidator creates a new path validator
func NewValidator(rootPath string) Validator {
	return &PathValidator{
		rootPath: filepath.Clean(rootPath),
	}
}

// ValidatePath ensures the path is within the allowed root directory
func (v *PathValidator) ValidatePath(path string) error {
	// Clean and resolve the path
	cleanPath := filepath.Clean(path)
	
	// Convert to absolute path if not already
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Ensure the path is within the root directory
	if !strings.HasPrefix(absPath, v.rootPath) {
		return fmt.Errorf("path %s is outside the allowed directory", path)
	}

	// Check for suspicious patterns
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal pattern")
	}

	return nil
}

// NoOpValidator is a validator that performs no validation
type NoOpValidator struct{}

// NewNoOpValidator creates a validator that performs no validation
func NewNoOpValidator() Validator {
	return &NoOpValidator{}
}

// ValidatePath always returns nil (no validation)
func (v *NoOpValidator) ValidatePath(path string) error {
	// Still check for obvious security issues
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal pattern")
	}
	return nil
}