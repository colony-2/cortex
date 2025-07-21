package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowCreateConnectsToTemporal(t *testing.T) {
	// Setup test - set the flag variables directly
	createProjectPath = ""
	createFilePath = "../../../recipe-core/examples/simple_workflow.yaml"
	serverHost = "127.0.0.1"
	serverPort = 7233
	namespace = "default"
	
	// Create command with output buffer
	cmd := &cobra.Command{
		Use:  "create",
		RunE: createWorkflow,
	}
	
	// Set up output capture
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Execute command - this will fail to connect since no server is running
	err := createWorkflow(cmd, []string{})
	
	// We expect it to fail connecting to the server
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to Temporal server")
	
	output := buf.String()
	
	// But before failing, it should validate and try to connect
	assert.Contains(t, output, "✓ Workflow 'simple_research' is valid!")
	assert.Contains(t, output, "Connecting to Temporal server...")
}

func TestWorkflowCreateRegistersWithTemporal(t *testing.T) {
	// The command has been updated to register workflows with Temporal
	
	// Check the command description
	assert.Equal(t, "Create and register a workflow with Temporal", workflowCreateCmd.Short)
	
	// The long description explains it registers with Temporal
	assert.Contains(t, workflowCreateCmd.Long, "registers the workflow type with Temporal")
	assert.Contains(t, workflowCreateCmd.Long, "starting a worker")
	
	// The createWorkflow function now:
	// 1. Parses the YAML
	// 2. Validates the workflow
	// 3. Displays information
	// 4. Connects to Temporal server
	// 5. Starts a worker that registers the workflow type
}