package shai

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/mount"
)

func TestNewMountBuilder(t *testing.T) {
	// Create temporary test directory structure
	tempDir := t.TempDir()
	testDir1 := filepath.Join(tempDir, "dir1")
	testDir2 := filepath.Join(tempDir, "dir2")
	os.MkdirAll(testDir1, 0755)
	os.MkdirAll(testDir2, 0755)

	tests := []struct {
		name    string
		workDir string
		rwPaths []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid single path",
			workDir: tempDir,
			rwPaths: []string{"dir1"},
			wantErr: false,
		},
		{
			name:    "valid multiple paths",
			workDir: tempDir,
			rwPaths: []string{"dir1", "dir2"},
			wantErr: false,
		},
		{
			name:    "current directory",
			workDir: tempDir,
			rwPaths: []string{"."},
			wantErr: false,
		},
		{
			name:    "non-existent working directory",
			workDir: "/non/existent/path",
			rwPaths: []string{"dir1"},
			wantErr: true,
			errMsg:  "working directory does not exist",
		},
		{
			name:    "non-existent rw path",
			workDir: tempDir,
			rwPaths: []string{"nonexistent"},
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name:    "path escapes working directory",
			workDir: tempDir,
			rwPaths: []string{"../outside"},
			wantErr: true,
			errMsg:  "escapes working directory",
		},
		{
			name:    "conflicting paths",
			workDir: tempDir,
			rwPaths: []string{".", "dir1"},
			wantErr: true,
			errMsg:  "mount conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb, err := NewMountBuilder(tt.workDir, tt.rwPaths)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if mb == nil {
					t.Error("expected non-nil MountBuilder")
				}
			}
		})
	}
}

func TestBuildMounts(t *testing.T) {
	tempDir := t.TempDir()
	testDir1 := filepath.Join(tempDir, "dir1")
	testDir2 := filepath.Join(tempDir, "dir2")
	os.MkdirAll(testDir1, 0755)
	os.MkdirAll(testDir2, 0755)

	tests := []struct {
		name           string
		rwPaths        []string
		expectedMounts []mount.Mount
	}{
		{
			name:    "single rw directory",
			rwPaths: []string{"dir1"},
			expectedMounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   tempDir,
					Target:   "/workspace",
					ReadOnly: true,
				},
				{
					Type:     mount.TypeBind,
					Source:   filepath.Join(tempDir, "dir1"),
					Target:   "/workspace/dir1",
					ReadOnly: false,
				},
			},
		},
		{
			name:    "multiple rw directories",
			rwPaths: []string{"dir1", "dir2"},
			expectedMounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   tempDir,
					Target:   "/workspace",
					ReadOnly: true,
				},
				{
					Type:     mount.TypeBind,
					Source:   filepath.Join(tempDir, "dir1"),
					Target:   "/workspace/dir1",
					ReadOnly: false,
				},
				{
					Type:     mount.TypeBind,
					Source:   filepath.Join(tempDir, "dir2"),
					Target:   "/workspace/dir2",
					ReadOnly: false,
				},
			},
		},
		{
			name:    "current directory as rw",
			rwPaths: []string{"."},
			expectedMounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   tempDir,
					Target:   "/workspace",
					ReadOnly: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb, err := NewMountBuilder(tempDir, tt.rwPaths)
			if err != nil {
				t.Fatalf("unexpected error creating MountBuilder: %v", err)
			}

			mounts := mb.BuildMounts()
			if !reflect.DeepEqual(mounts, tt.expectedMounts) {
				t.Errorf("BuildMounts() = %v, want %v", mounts, tt.expectedMounts)
			}
		})
	}
}

func TestValidateNoConflicts(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no conflicts",
			paths:   []string{"dir1", "dir2", "dir3"},
			wantErr: false,
		},
		{
			name:    "parent-child conflict",
			paths:   []string{"dir1", "dir1/subdir"},
			wantErr: true,
			errMsg:  "mount conflict",
		},
		{
			name:    "current directory conflicts with any",
			paths:   []string{".", "dir1"},
			wantErr: true,
			errMsg:  "mount conflict",
		},
		{
			name:    "nested directories conflict",
			paths:   []string{"a/b", "a/b/c"},
			wantErr: true,
			errMsg:  "mount conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := &MountBuilder{
				ReadWritePaths: tt.paths,
			}
			err := mb.ValidateNoConflicts()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsParentPath(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{".", "anything", true},
		{".", ".", true},
		{"dir1", "dir1/subdir", true},
		{"dir1", "dir1/subdir/nested", true},
		{"dir1", "dir2", false},
		{"dir1/sub", "dir1", false},
		{"a/b", "a/b/c", true},
		{"a/b", "a/bc", false},
	}

	for _, tt := range tests {
		t.Run(tt.parent+"_"+tt.child, func(t *testing.T) {
			got := isParentPath(tt.parent, tt.child)
			if got != tt.want {
				t.Errorf("isParentPath(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
			}
		})
	}
}

func TestBuildMountStrings(t *testing.T) {
	tempDir := t.TempDir()
	testDir1 := filepath.Join(tempDir, "dir1")
	os.MkdirAll(testDir1, 0755)

	mb, err := NewMountBuilder(tempDir, []string{"dir1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mountStrings := mb.BuildMountStrings()
	expected := []string{
		tempDir + ":/workspace:ro",
		filepath.Join(tempDir, "dir1") + ":/workspace/dir1:rw",
	}

	if !reflect.DeepEqual(mountStrings, expected) {
		t.Errorf("BuildMountStrings() = %v, want %v", mountStrings, expected)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   len(s) > len(substr) && contains(s[1:], substr)
}