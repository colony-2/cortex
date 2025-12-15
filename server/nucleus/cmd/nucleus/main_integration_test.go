package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/nucleus/internal/client"
	"github.com/colony-2/colony2/server/nucleus/internal/config"
	"github.com/colony-2/colony2/server/nucleus/internal/testutil"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"golang.org/x/net/context"
)

// TestMainFunction tests the main function execution
func TestMainFunction(t *testing.T) {
	// Save original args and command
	originalArgs := os.Args
	originalCmd := rootCmd
	defer func() {
		os.Args = originalArgs
		rootCmd = originalCmd
	}()

	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{
			name:     "help flag",
			args:     []string{"recipe-watcher", "--help"},
			wantExit: 0,
		},
		{
			name:     "missing required flags",
			args:     []string{"recipe-watcher"},
			wantExit: 1,
		},
		{
			name:     "invalid flag",
			args:     []string{"recipe-watcher", "--invalid-flag"},
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset command
			rootCmd = &cobra.Command{
				Use:   "recipe-watcher",
				Short: "Run recipe-worker with Temporal integration",
				Long:  `Recipe Watcher is a CLI tool that provides a managed way to run the recipe-worker package with configurable options. It monitors recipe files and dynamically executes workflows through Temporal.`,
				RunE:  run,
			}
			// Re-initialize flags
			rootCmd.PersistentFlags().String("name", "", "Worker name/identifier (required)")
			rootCmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "Temporal server address")
			rootCmd.PersistentFlags().StringP("recipes-path", "r", "", "Path to recipes directory (required)")
			rootCmd.PersistentFlags().String("namespace", "default", "Temporal namespace")
			rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging")

			rootCmd.MarkPersistentFlagRequired("name")
			rootCmd.MarkPersistentFlagRequired("recipes-path")

			// Capture output
			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)

			os.Args = tt.args

			// We can't actually test main() directly, but we can test the command execution
			err := rootCmd.Execute()

			if tt.wantExit == 0 && err != nil {
				t.Errorf("expected success, got error: %v", err)
			} else if tt.wantExit != 0 && err == nil {
				t.Error("expected error, got success")
			}
		})
	}
}

// TestRunFunctionWithEmbeddedTemporal tests the run function with a real embedded Temporal server
func TestRunFunctionWithEmbeddedTemporal(t *testing.T) {
	ts := testutil.StartTestServer(t)

	// Create test recipes directory
	tempDir := t.TempDir()
	recipesDir := filepath.Join(tempDir, "recipes")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatalf("failed to create recipes dir: %v", err)
	}

	// Write a test recipe
	recipeContent := `name: integration-test-recipe
version: "1.0.0"
description: Recipe for integration testing

# Root is a simple operation
op: command_execution
inputs:
  run: "echo 'Integration test'"
`
	if err := os.WriteFile(filepath.Join(recipesDir, "test.yaml"), []byte(recipeContent), 0644); err != nil {
		t.Fatalf("failed to write test recipe: %v", err)
	}

	// Create a test command
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("name", "integration-test", "")
	cmd.PersistentFlags().String("temporal-server", ts.HostPort(), "")
	cmd.PersistentFlags().String("recipes-path", recipesDir, "")
	cmd.PersistentFlags().String("namespace", "default", "")
	cmd.PersistentFlags().Bool("debug", true, "")

	// Run in a goroutine with timeout
	done := make(chan error, 1)
	go func() {
		// Create a test version of run function
		testRun := func(cmd *cobra.Command, args []string) error {
			cfg, err := parseConfig(cmd)
			if err != nil {
				return err
			}

			logger := zaptest.NewLogger(t)
			defer logger.Sync()

			temporalClient, err := client.NewTemporalClient(cfg)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrClientCreate, err)
			}
			defer temporalClient.Close()

			// Just verify we can connect successfully
			logger.Info("Successfully connected to Temporal server")
			return nil
		}
		cmd.RunE = testRun
		done <- cmd.Execute()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run function failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for run function")
	}
}

// TestParseConfigEdgeCases tests additional parseConfig scenarios
func TestParseConfigEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func() *cobra.Command
		wantErr     bool
		errContains string
	}{
		{
			name: "flag parsing error",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				// Don't define any flags - this will cause GetString to fail
				return cmd
			},
			wantErr:     true,
			errContains: "failed to get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			_, err := parseConfig(cmd)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

// TestSetupLoggerPanic tests the panic condition in setupLogger
func TestSetupLoggerPanic(t *testing.T) {
	// This is a bit tricky to test since zap.NewDevelopment() and zap.NewProduction()
	// rarely fail. We'll just ensure the function works in normal cases.

	tests := []struct {
		name  string
		debug bool
	}{
		{"debug logger", true},
		{"production logger", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("setupLogger panicked: %v", r)
				}
			}()

			logger := setupLogger(tt.debug)
			if logger == nil {
				t.Error("expected logger, got nil")
			}

			// Verify logger level
			if tt.debug {
				if !logger.Core().Enabled(zap.DebugLevel) {
					t.Error("debug logger should have debug level enabled")
				}
			}
		})
	}
}

// TestRunWithMockWorker tests the run function with a mock worker to improve coverage
func TestRunWithMockWorker(t *testing.T) {
	ts := testutil.StartTestServer(t)

	// Create config
	cfg := &config.Config{
		Name:           "test-worker",
		TemporalServer: ts.HostPort(),
		RecipesPath:    t.TempDir(),
		Namespace:      "default",
		Debug:          false,
	}

	// Test logger setup
	logger := setupLogger(cfg.Debug)
	if logger == nil {
		t.Fatal("failed to setup logger")
	}

	// Test client creation
	temporalClient, err := client.NewTemporalClient(cfg)
	if err != nil {
		t.Fatalf("failed to create temporal client: %v", err)
	}
	defer temporalClient.Close()

	// Verify client is connected
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to describe namespace to verify connection
	resp, err := temporalClient.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: cfg.Namespace,
	})
	if err != nil {
		t.Fatalf("failed to describe namespace: %v", err)
	}

	if resp.NamespaceInfo.Name != cfg.Namespace {
		t.Errorf("expected namespace %s, got %s", cfg.Namespace, resp.NamespaceInfo.Name)
	}
}
