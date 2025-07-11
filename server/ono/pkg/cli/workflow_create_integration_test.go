// +build integration

package cli

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
)

func TestWorkflowCreateWithRunningServer(t *testing.T) {
	// Start a test server
	opts := DevServerOptions{
		FrontendIP:    "127.0.0.1",
		FrontendPort:  17240,
		UIPort:        18240,
		Namespaces:    []string{"test-namespace"},
		DatabaseFile:  t.TempDir() + "/test.db",
		LogLevel:      "warn",
		SQLitePragmas: map[string]string{},
		EnableUI:      false,
	}

	devServer, err := NewDevServer(opts)
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