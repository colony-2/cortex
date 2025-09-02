package shai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/mount"
)

// MountBuilder builds selective read-write mount configurations
type MountBuilder struct {
	WorkingDir     string
	ReadWritePaths []string
}

// NewMountBuilder creates a mount builder for selective RW access
func NewMountBuilder(workingDir string, rwPaths []string) (*MountBuilder, error) {
	// Validate working directory exists
	if _, err := os.Stat(workingDir); err != nil {
		return nil, fmt.Errorf("working directory does not exist: %w", err)
	}
	
	// Clean and validate all paths
	cleanedPaths := make([]string, 0, len(rwPaths))
	for _, path := range rwPaths {
		// Clean the path
		cleanPath := filepath.Clean(path)
		
		// Ensure path doesn't escape working directory
		if strings.HasPrefix(cleanPath, "..") {
			return nil, fmt.Errorf("path %q escapes working directory", path)
		}
		
		// Check if path exists
		fullPath := filepath.Join(workingDir, cleanPath)
		if _, err := os.Stat(fullPath); err != nil {
			return nil, fmt.Errorf("path %q does not exist: %w", cleanPath, err)
		}
		
		cleanedPaths = append(cleanedPaths, cleanPath)
	}
	
	// Validate no conflicts
	mb := &MountBuilder{
		WorkingDir:     workingDir,
		ReadWritePaths: cleanedPaths,
	}
	
	if err := mb.ValidateNoConflicts(); err != nil {
		return nil, err
	}
	
	return mb, nil
}

// BuildMounts creates Docker mount specifications
// Base directory is read-only, specific paths are read-write
func (m *MountBuilder) BuildMounts() []mount.Mount {
	mounts := []mount.Mount{
		// Base mount: read-only
		{
			Type:     mount.TypeBind,
			Source:   m.WorkingDir,
			Target:   "/workspace",
			ReadOnly: true,
		},
	}
	
	// Add read-write overlays
	// These will override the read-only base mount for specific paths
	for _, rwPath := range m.ReadWritePaths {
		// Handle special case for current directory
		if rwPath == "." {
			// Override the base mount to be read-write
			mounts[0].ReadOnly = false
		} else {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   filepath.Join(m.WorkingDir, rwPath),
				Target:   filepath.Join("/workspace", rwPath),
				ReadOnly: false,
			})
		}
	}
	
	return mounts
}

// ValidateNoConflicts ensures mount paths don't conflict
func (m *MountBuilder) ValidateNoConflicts() error {
	// Check for overlapping paths
	for i, path1 := range m.ReadWritePaths {
		for j, path2 := range m.ReadWritePaths {
			if i == j {
				continue
			}
			
			// Check if one path is a parent of another
			if isParentPath(path1, path2) {
				return fmt.Errorf("mount conflict: %q is a parent of %q", path1, path2)
			}
			if isParentPath(path2, path1) {
				return fmt.Errorf("mount conflict: %q is a parent of %q", path2, path1)
			}
		}
	}
	
	return nil
}

// isParentPath checks if parent is a parent directory of child
func isParentPath(parent, child string) bool {
	// Handle special case for current directory
	if parent == "." {
		return true
	}
	
	// Normalize paths
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	
	// Add trailing separator to parent to ensure exact directory match
	if !strings.HasSuffix(parent, string(filepath.Separator)) {
		parent = parent + string(filepath.Separator)
	}
	
	// Check if child starts with parent
	return strings.HasPrefix(child, parent)
}

// BuildMountStrings returns mount specifications as strings for Docker CLI
func (m *MountBuilder) BuildMountStrings() []string {
	var mountStrings []string
	
	// Base mount
	mountStrings = append(mountStrings, fmt.Sprintf(
		"%s:/workspace:ro",
		m.WorkingDir,
	))
	
	// Read-write mounts
	for _, rwPath := range m.ReadWritePaths {
		if rwPath == "." {
			// Override base mount to be read-write
			mountStrings[0] = fmt.Sprintf(
				"%s:/workspace:rw",
				m.WorkingDir,
			)
		} else {
			mountStrings = append(mountStrings, fmt.Sprintf(
				"%s:/workspace/%s:rw",
				filepath.Join(m.WorkingDir, rwPath),
				rwPath,
			))
		}
	}
	
	return mountStrings
}