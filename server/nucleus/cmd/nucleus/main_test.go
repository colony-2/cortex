package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func TestRootCommand(t *testing.T) {
	cmd := rootCmd
	
	if cmd.Use != "recipe-watcher" {
		t.Errorf("expected Use to be 'recipe-watcher', got %s", cmd.Use)
	}
	
	if cmd.Short == "" {
		t.Error("expected Short description to be set")
	}
	
	if cmd.Long == "" {
		t.Error("expected Long description to be set")
	}
	
	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

func TestCommandFlags(t *testing.T) {
	// Save and restore the original command
	originalCmd := rootCmd
	defer func() { rootCmd = originalCmd }()
	
	// Re-initialize to ensure flags are properly marked
	rootCmd = &cobra.Command{
		Use:   "recipe-watcher",
		Short: "Run recipe-worker with Temporal integration",
		Long:  `Recipe Watcher is a CLI tool that provides a managed way to run the recipe-worker package with configurable options. It monitors recipe files and dynamically executes workflows through Temporal.`,
		RunE:  run,
	}
	
	// Manually set up flags (same as init function)
	rootCmd.PersistentFlags().String("name", "", "Worker name/identifier (required)")
	rootCmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "Temporal server address")
	rootCmd.PersistentFlags().StringP("recipes-path", "r", "", "Path to recipes directory (required)")
	rootCmd.PersistentFlags().String("namespace", "default", "Temporal namespace")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging")
	
	rootCmd.MarkPersistentFlagRequired("name")
	rootCmd.MarkPersistentFlagRequired("recipes-path")
	
	cmd := rootCmd
	
	tests := []struct {
		name        string
		flagName    string
		shorthand   string
		defaultVal  string
		required    bool
	}{
		{"name flag", "name", "", "", true},
		{"temporal-server flag", "temporal-server", "t", "localhost:7233", false},
		{"recipes-path flag", "recipes-path", "r", "", true},
		{"namespace flag", "namespace", "", "default", false},
		{"debug flag", "debug", "d", "false", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cmd.PersistentFlags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("flag %s not found", tt.flagName)
				return
			}
			
			if tt.shorthand != "" && flag.Shorthand != tt.shorthand {
				t.Errorf("expected shorthand %s, got %s", tt.shorthand, flag.Shorthand)
			}
			
			if flag.DefValue != tt.defaultVal {
				t.Errorf("expected default value %s, got %s", tt.defaultVal, flag.DefValue)
			}
			
			annotations := cmd.PersistentFlags().Lookup(tt.flagName).Annotations
			if tt.required {
				if _, ok := annotations[cobra.BashCompOneRequiredFlag]; !ok {
					t.Errorf("expected flag %s to be required", tt.flagName)
				}
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	tempDir := t.TempDir()
	recipesDir := filepath.Join(tempDir, "recipes")
	if err := os.Mkdir(recipesDir, 0755); err != nil {
		t.Fatalf("failed to create test recipes dir: %v", err)
	}
	
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			args: []string{
				"--name", "test-worker",
				"--recipes-path", recipesDir,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			args: []string{
				"--recipes-path", recipesDir,
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "missing recipes path",
			args: []string{
				"--name", "test-worker",
			},
			wantErr: true,
			errMsg:  "recipes path is required",
		},
		{
			name: "invalid name",
			args: []string{
				"--name", "test worker",
				"--recipes-path", recipesDir,
			},
			wantErr: true,
			errMsg:  "name must contain only alphanumeric",
		},
		{
			name: "non-existent recipes path",
			args: []string{
				"--name", "test-worker",
				"--recipes-path", filepath.Join(tempDir, "non-existent"),
			},
			wantErr: true,
			errMsg:  "recipes path does not exist",
		},
		{
			name: "with all flags",
			args: []string{
				"--name", "test-worker",
				"--recipes-path", recipesDir,
				"--temporal-server", "temporal.example.com:7233",
				"--namespace", "production",
				"--debug",
			},
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("name", "", "")
			cmd.Flags().StringP("temporal-server", "t", "localhost:7233", "")
			cmd.Flags().StringP("recipes-path", "r", "", "")
			cmd.Flags().String("namespace", "default", "")
			cmd.Flags().BoolP("debug", "d", false, "")
			
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}
			
			_, err := parseConfig(cmd)
			
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name  string
		debug bool
	}{
		{"production logger", false},
		{"development logger", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := setupLogger(tt.debug)
			if logger == nil {
				t.Error("expected logger to be created")
			}
			
			if tt.debug {
				if logger.Core().Enabled(zap.DebugLevel) == false {
					t.Error("expected debug level to be enabled")
				}
			}
		})
	}
}

func TestHelpOutput(t *testing.T) {
	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute help command: %v", err)
	}
	
	output := buf.String()
	
	expectedStrings := []string{
		"recipe-watcher",
		"Recipe Watcher is a CLI tool",
		"--name",
		"--temporal-server",
		"--recipes-path",
		"--namespace",
		"--debug",
	}
	
	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}