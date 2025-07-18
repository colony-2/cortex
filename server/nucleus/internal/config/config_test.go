package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tempDir := t.TempDir()
	validRecipesDir := filepath.Join(tempDir, "recipes")
	if err := os.Mkdir(validRecipesDir, 0755); err != nil {
		t.Fatalf("failed to create test recipes dir: %v", err)
	}

	notADir := filepath.Join(tempDir, "not-a-dir")
	if err := os.WriteFile(notADir, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "valid config",
			config: Config{
				Name:           "test-worker",
				RecipesPath:    validRecipesDir,
				TemporalServer: "localhost:7233",
				Namespace:      "default",
			},
			wantErr: nil,
		},
		{
			name: "valid config with defaults",
			config: Config{
				Name:        "test-worker",
				RecipesPath: validRecipesDir,
			},
			wantErr: nil,
		},
		{
			name: "missing name",
			config: Config{
				RecipesPath: validRecipesDir,
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "invalid name with spaces",
			config: Config{
				Name:        "test worker",
				RecipesPath: validRecipesDir,
			},
			wantErr: ErrInvalidName,
		},
		{
			name: "invalid name with special chars",
			config: Config{
				Name:        "test@worker",
				RecipesPath: validRecipesDir,
			},
			wantErr: ErrInvalidName,
		},
		{
			name: "valid name with hyphens",
			config: Config{
				Name:        "test-worker-123",
				RecipesPath: validRecipesDir,
			},
			wantErr: nil,
		},
		{
			name: "missing recipes path",
			config: Config{
				Name: "test-worker",
			},
			wantErr: ErrRecipesPathRequired,
		},
		{
			name: "non-existent recipes path",
			config: Config{
				Name:        "test-worker",
				RecipesPath: filepath.Join(tempDir, "non-existent"),
			},
			wantErr: ErrRecipesPathNotExist,
		},
		{
			name: "recipes path is not a directory",
			config: Config{
				Name:        "test-worker",
				RecipesPath: notADir,
			},
			wantErr: ErrRecipesPathNotDir,
		},
		{
			name: "invalid temporal server - no port",
			config: Config{
				Name:           "test-worker",
				RecipesPath:    validRecipesDir,
				TemporalServer: "localhost",
			},
			wantErr: ErrInvalidTemporalServer,
		},
		{
			name: "invalid temporal server - no host",
			config: Config{
				Name:           "test-worker",
				RecipesPath:    validRecipesDir,
				TemporalServer: ":7233",
			},
			wantErr: ErrInvalidTemporalServer,
		},
		{
			name: "valid temporal server with domain",
			config: Config{
				Name:           "test-worker",
				RecipesPath:    validRecipesDir,
				TemporalServer: "temporal.example.com:7233",
			},
			wantErr: nil,
		},
		{
			name: "valid temporal server with IP",
			config: Config{
				Name:           "test-worker",
				RecipesPath:    validRecipesDir,
				TemporalServer: "192.168.1.1:7233",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			err := config.Validate()
			
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
					return
				}
				if err != tt.wantErr && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}

			if err == nil && config.Namespace == "" {
				t.Error("expected namespace to be set to default")
			}

			if err == nil && config.TemporalServer == "" {
				t.Error("expected temporal server to be set to default")
			}
		})
	}
}

func TestConfig_RelativePathConversion(t *testing.T) {
	tempDir := t.TempDir()
	recipesDir := filepath.Join(tempDir, "recipes")
	if err := os.Mkdir(recipesDir, 0755); err != nil {
		t.Fatalf("failed to create test recipes dir: %v", err)
	}

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	config := Config{
		Name:        "test-worker",
		RecipesPath: "./recipes",
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if !filepath.IsAbs(config.RecipesPath) {
		t.Error("expected recipes path to be converted to absolute path")
	}

	expectedPath, _ := filepath.EvalSymlinks(recipesDir)
	actualPath, _ := filepath.EvalSymlinks(config.RecipesPath)
	if actualPath != expectedPath {
		t.Errorf("expected recipes path to be %s, got %s", expectedPath, actualPath)
	}
}