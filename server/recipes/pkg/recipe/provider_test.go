package recipe

import (
	"testing"
)

func TestParseRecipeRef(t *testing.T) {
	provider := &Provider{} // No service needed for this test

	tests := []struct {
		name         string
		input        string
		expectedName string
		expectedRef  string
	}{
		{
			name:         "simple name without ref",
			input:        "workflows/ci/build",
			expectedName: "workflows/ci/build",
			expectedRef:  "",
		},
		{
			name:         "name with commit hash",
			input:        "workflows/ci/build@a1b2c3d4e5f6",
			expectedName: "workflows/ci/build",
			expectedRef:  "a1b2c3d4e5f6",
		},
		{
			name:         "name with short hash",
			input:        "workflows/ci/build@a1b2c3d",
			expectedName: "workflows/ci/build",
			expectedRef:  "a1b2c3d",
		},
		{
			name:         "name with branch",
			input:        "workflows/ci/build@main",
			expectedName: "workflows/ci/build",
			expectedRef:  "main",
		},
		{
			name:         "name with feature branch",
			input:        "workflows/ci/build@feature/new-api",
			expectedName: "workflows/ci/build",
			expectedRef:  "feature/new-api",
		},
		{
			name:         "name with tag",
			input:        "workflows/ci/build@v1.0.0",
			expectedName: "workflows/ci/build",
			expectedRef:  "v1.0.0",
		},
		{
			name:         "name with relative ref",
			input:        "workflows/ci/build@HEAD~1",
			expectedName: "workflows/ci/build",
			expectedRef:  "HEAD~1",
		},
		{
			name:         "simple name with tag",
			input:        "simple@v2.0.0",
			expectedName: "simple",
			expectedRef:  "v2.0.0",
		},
		{
			name:         "name with multiple @ (use last one)",
			input:        "foo@bar@baz",
			expectedName: "foo@bar",
			expectedRef:  "baz",
		},
		{
			name:         "empty ref after @",
			input:        "foo@",
			expectedName: "foo",
			expectedRef:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ref := provider.parseRecipeRef(tt.input)
			if name != tt.expectedName {
				t.Errorf("parseRecipeRef(%q) name = %q, want %q", tt.input, name, tt.expectedName)
			}
			if ref != tt.expectedRef {
				t.Errorf("parseRecipeRef(%q) ref = %q, want %q", tt.input, ref, tt.expectedRef)
			}
		})
	}
}
