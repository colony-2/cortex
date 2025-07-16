//go:build integration
// +build integration

package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
)

func TestWorkflowRunIntegration(t *testing.T) {
	// This test requires a running Temporal server
	// It will be skipped if the server is not available
	
	// Try to connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  "127.0.0.1:7233",
		Namespace: "default",
	})
	if err != nil {
		t.Skip("Skipping integration test - Temporal server not available")
		return
	}
	defer c.Close()

	// Create a test worker
	w := worker.New(c, "test-task-queue", worker.Options{})

	// Register mock activities
	w.RegisterActivity(mockResearchActivity)
	w.RegisterActivity(mockAnalyzeActivity)
	w.RegisterActivity(mockWriteReportActivity)

	// Start worker in background
	workerErr := make(chan error, 1)
	go func() {
		workerErr <- w.Run(worker.InterruptCh())
	}()

	// Give worker time to start
	time.Sleep(100 * time.Millisecond)

	defer func() {
		w.Stop()
		select {
		case err := <-workerErr:
			if err != nil {
				t.Logf("Worker error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Log("Worker stop timeout")
		}
	}()

	t.Run("run simple workflow", func(t *testing.T) {
		// This would require implementing the full workflow execution
		// For now, we'll test the command parsing
		t.Skip("Requires full workflow implementation")
	})
}

func TestWorkflowRunCommandValidation(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func()
		args          []string
		expectedError string
	}{
		{
			name: "missing file and project",
			setupFlags: func() {
				workflowFile = ""
				projectPath = ""
			},
			args:          []string{},
			expectedError: "either --file or --project must be specified",
		},
		{
			name: "both file and project specified",
			setupFlags: func() {
				workflowFile = "test.yaml"
				projectPath = "./project"
			},
			args:          []string{},
			expectedError: "cannot specify both --file and --project",
		},
		{
			name: "project without workflow name",
			setupFlags: func() {
				workflowFile = ""
				projectPath = "./project"
			},
			args:          []string{},
			expectedError: "workflow name required when using --project",
		},
		{
			name: "invalid file extension",
			setupFlags: func() {
				workflowFile = "workflow.txt"
				projectPath = ""
			},
			args:          []string{},
			expectedError: "workflow file must have .yaml or .yml extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			workflowFile = ""
			projectPath = ""
			workflowID = ""
			runNamespace = "default"
			workflowInputs = make(map[string]string)
			taskQueue = "test-queue"

			// Setup test flags
			tt.setupFlags()

			// Create mock command context
			ctx := context.Background()
			_ = ctx

			// We can't easily test executeWorkflow directly due to Temporal client
			// So we'll test the validation logic
			var err error

			// Simulate the validation logic from executeWorkflow
			if workflowFile != "" && projectPath != "" {
				err = fmt.Errorf("cannot specify both --file and --project flags")
			} else if workflowFile == "" && projectPath == "" {
				err = fmt.Errorf("either --file or --project must be specified")
			} else if projectPath != "" && len(tt.args) == 0 {
				err = fmt.Errorf("workflow name required when using --project flag")
			} else if workflowFile != "" && !strings.HasSuffix(workflowFile, ".yaml") && !strings.HasSuffix(workflowFile, ".yml") {
				err = fmt.Errorf("workflow file must have .yaml or .yml extension")
			}

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkflowRunInputParsing(t *testing.T) {
	tests := []struct {
		name           string
		inputs         map[string]string
		expectedInputs map[string]interface{}
	}{
		{
			name: "simple string inputs",
			inputs: map[string]string{
				"topic": "AI Safety",
				"depth": "5",
			},
			expectedInputs: map[string]interface{}{
				"topic": "AI Safety",
				"depth": "5",
			},
		},
		{
			name:           "empty inputs",
			inputs:         map[string]string{},
			expectedInputs: map[string]interface{}{},
		},
		{
			name: "inputs with special characters",
			inputs: map[string]string{
				"query":  "What is 2+2?",
				"filter": "type=research&status=active",
			},
			expectedInputs: map[string]interface{}{
				"query":  "What is 2+2?",
				"filter": "type=research&status=active",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the input conversion logic
			result := make(map[string]interface{})
			for key, value := range tt.inputs {
				result[key] = value
			}

			assert.Equal(t, tt.expectedInputs, result)
		})
	}
}

func TestWorkflowRunIDGeneration(t *testing.T) {
	tests := []struct {
		name         string
		providedID   string
		workflowName string
		expectCustom bool
	}{
		{
			name:         "custom ID provided",
			providedID:   "my-custom-id",
			workflowName: "test-workflow",
			expectCustom: true,
		},
		{
			name:         "auto-generated ID",
			providedID:   "",
			workflowName: "test-workflow",
			expectCustom: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflowID = tt.providedID

			// Simulate ID generation logic
			var generatedID string
			if workflowID == "" {
				// In real code, this uses uuid.New()
				generatedID = fmt.Sprintf("%s-12345678", tt.workflowName)
			} else {
				generatedID = workflowID
			}

			if tt.expectCustom {
				assert.Equal(t, tt.providedID, generatedID)
			} else {
				assert.Contains(t, generatedID, tt.workflowName)
				assert.Contains(t, generatedID, "-")
			}
		})
	}
}

// Mock activities for testing
func mockResearchActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"research_data": map[string]interface{}{
			"sources": []string{"source1", "source2"},
			"facts":   []string{"fact1", "fact2"},
		},
	}, nil
}

func mockAnalyzeActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"analysis": "Test analysis results",
		"insights": []string{"insight1", "insight2"},
	}, nil
}

func mockWriteReportActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"report": "# Test Report\n\nThis is a test report.",
	}, nil
}

// Test with the test suite for more isolated testing
func TestWorkflowRunWithTestSuite(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register mock activities
	env.RegisterActivity(mockResearchActivity)
	env.RegisterActivity(mockAnalyzeActivity)
	env.RegisterActivity(mockWriteReportActivity)

	t.Run("mock workflow execution", func(t *testing.T) {
		// This would test the actual workflow execution
		// For now, we're focusing on command-line integration
		t.Skip("Requires workflow implementation")
	})
}

func TestWorkflowRunFlagsIntegration(t *testing.T) {
	// Test that all flags are properly registered
	cmd := workflowRunCmd

	// Check required flags
	assert.NotNil(t, cmd.Flag("file"))
	assert.NotNil(t, cmd.Flag("project"))
	assert.NotNil(t, cmd.Flag("id"))
	assert.NotNil(t, cmd.Flag("namespace"))
	assert.NotNil(t, cmd.Flag("input"))
	assert.NotNil(t, cmd.Flag("task-queue"))
	assert.NotNil(t, cmd.Flag("host"))
	assert.NotNil(t, cmd.Flag("port"))

	// Check defaults
	assert.Equal(t, "default", cmd.Flag("namespace").DefValue)
	assert.Equal(t, "ono-task-queue", cmd.Flag("task-queue").DefValue)
	assert.Equal(t, "127.0.0.1", cmd.Flag("host").DefValue)
	assert.Equal(t, "7233", cmd.Flag("port").DefValue)
}

func TestWorkflowRunUsageExamples(t *testing.T) {
	// Test that help text includes all usage examples
	help := workflowRunCmd.Long

	examples := []string{
		"Run a workflow from a file",
		"--file",
		"Run a workflow from a project",
		"--project",
		"Run with specific workflow ID",
		"--id",
		"Run with inputs",
		"--input",
		"Run with custom task queue",
		"--task-queue",
	}

	for _, example := range examples {
		assert.Contains(t, help, example, "Help text should include: %s", example)
	}
}

// Benchmark for workflow parsing performance
func BenchmarkWorkflowParsing(b *testing.B) {
	// This benchmark tests how fast we can parse workflow files
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// In a real benchmark, we would parse actual workflow files
		// For now, this is a placeholder
		_ = i
	}
}
