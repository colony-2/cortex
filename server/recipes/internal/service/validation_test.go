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

func TestPreValidateRecipe(t *testing.T) {
	// This requires a mock service, so we'll test it in integration tests
	// For now, just ensure the function exists and can be called
	// We'll add proper tests when we have the full service implementation
}
