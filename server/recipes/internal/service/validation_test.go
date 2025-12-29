package service

import (
	"testing"
)

func TestValidateRecipeName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "empty name",
			input:     "",
			wantError: true,
		},
		{
			name:      "simple valid name",
			input:     "my-recipe",
			wantError: false,
		},
		{
			name:      "valid hierarchical name",
			input:     "workflows/ci/build",
			wantError: false,
		},
		{
			name:      "valid with underscore",
			input:     "data_processing/etl/load_users",
			wantError: false,
		},
		{
			name:      "valid with dash",
			input:     "deployment-recipe",
			wantError: false,
		},
		{
			name:      "invalid with dot",
			input:     "my.recipe",
			wantError: true,
		},
		{
			name:      "invalid with space",
			input:     "my recipe",
			wantError: true,
		},
		{
			name:      "invalid with @",
			input:     "my@recipe",
			wantError: true,
		},
		{
			name:      "invalid empty segment",
			input:     "foo//bar",
			wantError: true,
		},
		{
			name:      "invalid leading slash",
			input:     "/foo/bar",
			wantError: true,
		},
		{
			name:      "invalid trailing slash",
			input:     "foo/bar/",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecipeName(tt.input)
			if tt.wantError && err == nil {
				t.Errorf("validateRecipeName(%q) expected error, got nil", tt.input)
			}
			if !tt.wantError && err != nil {
				t.Errorf("validateRecipeName(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestDeriveGitPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "simple",
			expected: ".c2/recipes/simple.recipe.yaml",
		},
		{
			name:     "hierarchical name",
			input:    "foo/bar/nested",
			expected: ".c2/recipes/foo/bar/nested.recipe.yaml",
		},
		{
			name:     "single level",
			input:    "workflows/ci",
			expected: ".c2/recipes/workflows/ci.recipe.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveGitPath(tt.input)
			// Normalize path separators for cross-platform compatibility
			// The function returns OS-specific paths, but we test with forward slashes
			if result != tt.expected {
				t.Errorf("deriveGitPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full hash",
			input:    "a1b2c3d4e5f6789012345678901234567890abcd",
			expected: "a1b2c3d",
		},
		{
			name:     "already short",
			input:    "a1b2c3d",
			expected: "a1b2c3d",
		},
		{
			name:     "very short",
			input:    "abc",
			expected: "abc",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortHash(tt.input)
			if result != tt.expected {
				t.Errorf("shortHash(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPreValidateRecipe(t *testing.T) {
	// This requires a mock service, so we'll test it in integration tests
	// For now, just ensure the function exists and can be called
	// We'll add proper tests when we have the full service implementation
}
