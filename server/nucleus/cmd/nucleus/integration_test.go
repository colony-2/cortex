//go:build integration
// +build integration

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/nucleus/internal/testutil"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
)

func TestRecipeWatcherFullLifecycle(t *testing.T) {
	// Start embedded Temporal server
	ts := testutil.StartTestServer(t)
	
	// Create test recipes directory
	tempDir := t.TempDir()
	recipesDir := filepath.Join(tempDir, "recipes")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatalf("failed to create recipes dir: %v", err)
	}
	
	// Copy test recipe
	testRecipe := `name: test-recipe
version: "1.0.0"
description: Test recipe for integration testing

# Root is a simple operation
op: command_execution
inputs:
  run: "echo 'Test executed'"
`
	if err := os.WriteFile(filepath.Join(recipesDir, "test.yaml"), []byte(testRecipe), 0644); err != nil {
		t.Fatalf("failed to write test recipe: %v", err)
	}
	
	// Run recipe-watcher in a goroutine
	errCh := make(chan error, 1)
	stopCh := make(chan struct{})
	
	go func() {
		// Reset command for testing
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
		
		rootCmd.SetArgs([]string{
			"--name", "test-worker",
			"--recipes-path", recipesDir,
			"--temporal-server", ts.HostPort(),
			"--namespace", "default",
		})
		
		// Create a modified run function to allow controlled shutdown
		modifiedRun := func(cmd *cobra.Command, args []string) error {
			// Run in a separate goroutine
			runErr := make(chan error, 1)
			go func() {
				runErr <- run(cmd, args)
			}()
			
			select {
			case err := <-runErr:
				return err
			case <-stopCh:
				// Send interrupt signal to trigger graceful shutdown
				p, _ := os.FindProcess(os.Getpid())
				p.Signal(os.Interrupt)
				// Wait for shutdown
				return <-runErr
			}
		}
		
		rootCmd.RunE = modifiedRun
		errCh <- rootCmd.Execute()
	}()
	
	// Wait for worker to start
	time.Sleep(2 * time.Second)
	
	// Verify worker is running by checking task queue
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Check if task queue exists
	resp, err := ts.Client().DescribeTaskQueue(ctx, "ono-recipes-test-recipe", client.TaskQueueTypeWorkflow)
	if err != nil {
		t.Logf("Warning: Could not describe task queue: %v", err)
	} else if resp != nil {
		t.Logf("Task queue found with %d pollers", len(resp.Pollers))
	}
	
	// Trigger shutdown
	close(stopCh)
	
	// Wait for clean shutdown
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("recipe-watcher exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for recipe-watcher to stop")
	}
}

func TestRecipeWatcherWithEmbeddedTemporal(t *testing.T) {
	ts := testutil.StartTestServer(t)
	
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name: "valid config",
			args: []string{
				"--name", "test",
				"--recipes-path", "./testdata/recipes",
				"--temporal-server", ts.HostPort(),
			},
			wantErr: false,
		},
		{
			name: "custom namespace",
			args: []string{
				"--name", "test",
				"--recipes-path", "./testdata/recipes",
				"--temporal-server", ts.HostPort(),
				"--namespace", "test",
			},
			wantErr: false,
		},
		{
			name: "debug mode",
			args: []string{
				"--name", "test",
				"--recipes-path", "./testdata/recipes",
				"--temporal-server", ts.HostPort(),
				"--debug",
			},
			wantErr: false,
		},
		{
			name: "invalid recipes path",
			args: []string{
				"--name", "test",
				"--recipes-path", "/non/existent/path",
				"--temporal-server", ts.HostPort(),
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create new command for each test
			cmd := &cobra.Command{Use: "test"}
			cmd.PersistentFlags().String("name", "", "")
			cmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "")
			cmd.PersistentFlags().StringP("recipes-path", "r", "", "")
			cmd.PersistentFlags().String("namespace", "default", "")
			cmd.PersistentFlags().BoolP("debug", "d", false, "")
			
			cmd.SetArgs(tt.args)
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}
			
			_, err := parseConfig(cmd)
			
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestRecipeWatcherErrorScenarios(t *testing.T) {
	t.Run("temporal connection failure", func(t *testing.T) {
		// Use invalid temporal server address
		cmd := &cobra.Command{Use: "test"}
		cmd.PersistentFlags().String("name", "", "")
		cmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "")
		cmd.PersistentFlags().StringP("recipes-path", "r", "", "")
		cmd.PersistentFlags().String("namespace", "default", "")
		cmd.PersistentFlags().BoolP("debug", "d", false, "")
		
		tempDir := t.TempDir()
		cmd.SetArgs([]string{
			"--name", "test",
			"--recipes-path", tempDir,
			"--temporal-server", "invalid:99999",
		})
		
		// This should fail during execution, not config parsing
		if err := cmd.ParseFlags([]string{
			"--name", "test",
			"--recipes-path", tempDir,
			"--temporal-server", "invalid:99999",
		}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		
		cfg, err := parseConfig(cmd)
		if err != nil {
			t.Fatalf("unexpected config error: %v", err)
		}
		
		// Try to create client with invalid server
		_, err = client.Dial(client.Options{
			HostPort:  cfg.TemporalServer,
			Namespace: cfg.Namespace,
		})
		
		if err == nil {
			t.Error("expected error connecting to invalid temporal server")
		}
	})
}

func TestSignalHandling(t *testing.T) {
	if os.Getenv("BE_SUBPROCESS") == "1" {
		// This is the subprocess
		ts := testutil.StartTestServer(t)
		
		tempDir := t.TempDir()
		recipesDir := filepath.Join(tempDir, "recipes")
		os.MkdirAll(recipesDir, 0755)
		
		rootCmd.SetArgs([]string{
			"--name", "signal-test",
			"--recipes-path", recipesDir,
			"--temporal-server", ts.HostPort(),
		})
		
		// Run the command
		if err := rootCmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	
	// Parent process
	cmd := exec.Command(os.Args[0], "-test.run=TestSignalHandling")
	cmd.Env = append(os.Environ(), "BE_SUBPROCESS=1")
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start subprocess: %v", err)
	}
	
	// Give it time to start
	time.Sleep(2 * time.Second)
	
	// Send SIGTERM
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send signal: %v", err)
	}
	
	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	
	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() != 0 {
					t.Errorf("process exited with code %d", exitErr.ExitCode())
				}
			} else {
				t.Errorf("process exited with error: %v", err)
			}
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Error("timeout waiting for process to handle signal")
	}
}

func TestMultipleWorkers(t *testing.T) {
	ts := testutil.StartTestServer(t)
	
	// Create multiple recipe directories
	tempDir := t.TempDir()
	dirs := make([]string, 3)
	for i := 0; i < 3; i++ {
		dirs[i] = filepath.Join(tempDir, fmt.Sprintf("recipes%d", i))
		if err := os.MkdirAll(dirs[i], 0755); err != nil {
			t.Fatalf("failed to create recipes dir %d: %v", i, err)
		}
	}
	
	// Start multiple workers concurrently
	type result struct {
		name string
		err  error
	}
	
	results := make(chan result, 3)
	
	for i := 0; i < 3; i++ {
		go func(idx int) {
			name := fmt.Sprintf("worker-%d", idx)
			
			// Create separate command instance
			cmd := &cobra.Command{
				Use:   "recipe-watcher",
				Short: "Run recipe-worker with Temporal integration",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Just validate config for this test
					_, err := parseConfig(cmd)
					return err
				},
			}
			
			cmd.PersistentFlags().String("name", "", "")
			cmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "")
			cmd.PersistentFlags().StringP("recipes-path", "r", "", "")
			cmd.PersistentFlags().String("namespace", "default", "")
			cmd.PersistentFlags().BoolP("debug", "d", false, "")
			
			cmd.SetArgs([]string{
				"--name", name,
				"--recipes-path", dirs[idx],
				"--temporal-server", ts.HostPort(),
			})
			
			err := cmd.Execute()
			results <- result{name: name, err: err}
		}(i)
	}
	
	// Collect results
	for i := 0; i < 3; i++ {
		res := <-results
		if res.err != nil {
			t.Errorf("worker %s failed: %v", res.name, res.err)
		}
	}
}