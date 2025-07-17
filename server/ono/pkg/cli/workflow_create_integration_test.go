//go:build integration
// +build integration

package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibethis/server/embeddedtemporal/pkg/temporal"
	"go.temporal.io/sdk/client"
)

func TestWorkflowCreateWithRunningServer(t *testing.T) {
	// Start a test server
	opts := temporal.Options{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  17240,
		UIPort:        18240,
		Namespaces:    []string{"test-namespace"},
		DatabaseFile:  t.TempDir() + "/test.db",
		LogLevel:      "warn",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	devServer, err := embeddedtemporal.NewServer(opts)
	require.NoError(t, err)

	err = devServer.Start()
	require.NoError(t, err)
	defer devServer.Stop()

	// Give server time to fully initialize
	time.Sleep(2 * time.Second)

	// Setup workflow create command
	createProjectPath = ""
	createFilePath = "../../example/simple_workflow.yaml"
	serverHost = "127.0.0.1"
	serverPort = 17240
	namespace = "test-namespace"

	// Create a goroutine to run the create command (since it blocks)
	done := make(chan error)
	go func() {
		cmd := workflowCreateCmd
		err := cmd.RunE(cmd, []string{})
		done <- err
	}()

	// Give it time to register
	time.Sleep(2 * time.Second)

	// Connect to the server and verify we can start the workflow
	c, err := client.Dial(client.Options{
		HostPort:  "127.0.0.1:17240",
		Namespace: "test-namespace",
	})
	require.NoError(t, err)
	defer c.Close()

	// Try to start the workflow that should now be registered
	ctx := context.Background()
	we, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "test-workflow-1",
		TaskQueue: "simple_research-queue",
	}, "simple_research", map[string]interface{}{
		"query": "test query",
	})
	
	// This should succeed now that the workflow is registered
	assert.NoError(t, err)
	assert.NotNil(t, we)
	assert.NotEmpty(t, we.GetID())
	assert.NotEmpty(t, we.GetRunID())
}

func TestWorkflowCreateCommandIntegration(t *testing.T) {
	// Start a test server
	opts := temporal.Options{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  17241,
		UIPort:        18241,
		Namespaces:    []string{"default"},
		DatabaseFile:  t.TempDir() + "/test.db",
		LogLevel:      "error",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	devServer, err := embeddedtemporal.NewServer(opts)
	require.NoError(t, err)

	err = devServer.Start()
	require.NoError(t, err)
	defer devServer.Stop()

	// Give server time to fully initialize
	time.Sleep(2 * time.Second)

	tests := []struct {
		name           string
		args           []string
		setupFunc      func()
		expectedError  bool
		errorContains  string
		outputContains []string
	}{
		{
			name: "valid file",
			setupFunc: func() {
				createProjectPath = ""
				createFilePath = "../../example/simple_workflow.yaml"
				serverHost = "127.0.0.1"
				serverPort = 17241
				namespace = "default"
			},
			args: []string{},
			outputContains: []string{
				"Loading project from:",
				"Workflow: simple_research",
				"Registering activities:",
				"quick_search",
				"summarize_results", 
				"Workflow structure:",
				"Required inputs:",
				"query",
				"✓ Workflow 'simple_research' is valid!",
			},
			expectedError: false,
		},
		{
			name: "valid project",
			setupFunc: func() {
				createProjectPath = "../../example/research_project"
				createFilePath = ""
				serverHost = "127.0.0.1"
				serverPort = 17241
				namespace = "default"
			},
			args: []string{},
			outputContains: []string{
				"Loading project from:",
				"Project: research_project",
				"Workflow: research_report_workflow",
				"research_activity",
				"analyze_activity",
				"write_report_activity",
				"✓ Workflow 'research_report_workflow' is valid!",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			createProjectPath = ""
			createFilePath = ""

			// Create command with output buffer
			cmd := &cobra.Command{
				Use:  "create",
				RunE: createWorkflow,
			}

			// Add flags to command
			cmd.Flags().StringVar(&createProjectPath, "project", "", "")
			cmd.Flags().StringVar(&createFilePath, "file", "", "")

			// Setup test - this sets the flag variables
			tt.setupFunc()

			// Set up output capture
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			// Create a context with timeout for the command
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Execute command in a goroutine since it will run a worker
			done := make(chan error, 1)
			go func() {
				done <- cmd.ExecuteContext(ctx)
			}()

			// Let it run for a few seconds to register the workflow
			select {
			case err := <-done:
				// Command completed (might be due to error or early exit)
				output := buf.String()
				
				if tt.expectedError {
					require.Error(t, err)
					if tt.errorContains != "" {
						assert.Contains(t, err.Error(), tt.errorContains)
					}
				} else {
					if err != nil && err != context.Canceled {
						t.Logf("Command output: %s", output)
						t.Logf("Command error: %v", err)
					}
				}

				for _, expected := range tt.outputContains {
					assert.Contains(t, output, expected, "Output should contain: %s", expected)
				}

			case <-time.After(3 * time.Second):
				// Command is still running (worker mode), this is expected
				// Cancel the context to stop it
				cancel()
				
				// Wait for it to actually stop
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Log("Command didn't stop after cancellation")
				}

				output := buf.String()
				for _, expected := range tt.outputContains {
					assert.Contains(t, output, expected, "Output should contain: %s", expected)
				}
			}
		})
	}
}

func TestWorkflowCreateExamplesIntegration(t *testing.T) {
	// Start a test server
	opts := temporal.Options{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  17242,
		UIPort:        18242,
		Namespaces:    []string{"default"},
		DatabaseFile:  t.TempDir() + "/test.db",
		LogLevel:      "error",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	devServer, err := embeddedtemporal.NewServer(opts)
	require.NoError(t, err)

	err = devServer.Start()
	require.NoError(t, err)
	defer devServer.Stop()

	// Give server time to fully initialize
	time.Sleep(2 * time.Second)

	examples := []struct {
		name   string
		args   []string
		checks []string
	}{
		{
			name: "simple workflow file",
			args: []string{"--file", "../../example/simple_workflow.yaml"},
			checks: []string{
				"simple_research",
				"v1.0",
				"A simple research workflow",
				"✓ Workflow 'simple_research' is valid!",
			},
		},
		{
			name: "research project",
			args: []string{"--project", "../../example/research_project"},
			checks: []string{
				"research_report_workflow",
				"Research a topic and generate a comprehensive report",
				"✓ Workflow 'research_report_workflow' is valid!",
			},
		},
		{
			name: "gemini workflow",
			args: []string{"--file", "../../example/gemini_workflow.yaml"},
			checks: []string{
				"gemini_research_workflow",
				"gemini_report_activity",
				"✓ Workflow 'gemini_research_workflow' is valid!",
			},
		},
	}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			// Set server connection details
			serverHost = "127.0.0.1"
			serverPort = 17242
			namespace = "default"

			cmd := &cobra.Command{
				Use:  "create",
				RunE: createWorkflow,
			}

			cmd.Flags().StringVar(&createProjectPath, "project", "", "")
			cmd.Flags().StringVar(&createFilePath, "file", "", "")

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(ex.args)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- cmd.ExecuteContext(ctx)
			}()

			// Let it run briefly then cancel
			time.Sleep(3 * time.Second)
			cancel()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}

			output := buf.String()

			for _, check := range ex.checks {
				assert.Contains(t, output, check, "Output should contain: %s", check)
			}
		})
	}
}