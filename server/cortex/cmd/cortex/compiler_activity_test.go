package main

import (
	"context"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	opsactivity "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityInputParsing(t *testing.T) {
	// Test that our activity executor properly parses inputs
	// for the command_execution activity

	ctx := context.Background()

	// Create activity registry
	registry := ops.NewActivityRegistry()
	activities := opsactivity.GetAll()
	for _, act := range activities {
		err := registry.RegisterGeneric(act)
		require.NoError(t, err)
	}

	// Get the command_execution activity
	activityReg, exists := registry.Get("command_execution")
	require.True(t, exists, "command_execution should be registered")
	require.NotNil(t, activityReg.Activity)

	// The activity should be able to handle these inputs
	cmdActivity, ok := activityReg.Activity.(*commandop.CommandExecutionActivityWrapper)
	require.True(t, ok, "Should be CommandExecutionActivityWrapper")

	// Create the input structure that the activity expects
	cmdInput := commandop.CommandExecutionInput{
		Run: "echo Hello World",
	}

	// Execute the activity
	output, err := cmdActivity.Execute(ctx, commandop.CommandExecutionConfig{}, cmdInput)
	require.NoError(t, err)

	// Verify the output
	assert.True(t, output.Success)
	assert.Equal(t, "Hello World\n", output.Stdout)
	assert.Equal(t, 0, output.ExitCode)
}

func TestReflectionBasedActivityExecution(t *testing.T) {
	// Test that our reflection-based activity executor in createActivityExecutor
	// properly converts map inputs to the activity's input struct

	ctx := context.Background()

	// Create activity registry
	registry := ops.NewActivityRegistry()
	activities := opsactivity.GetAll()
	for _, act := range activities {
		err := registry.RegisterGeneric(act)
		require.NoError(t, err)
	}

	// Create the activity executor using the new executor package
	activityType := "command_execution"
	logger := createTestLogger()
	activityExecutor := executor.NewActivityExecutor(registry, logger)
	executorFunc := activityExecutor.CreateTemporalActivity(activityType)

	// Test inputs as a map (what comes from the workflow)
	inputs := map[string]interface{}{
		"run": "echo Test Message",
	}

	// Execute through the reflection-based executor
	outputs, err := executorFunc.(func(context.Context, map[string]interface{}) (map[string]interface{}, error))(ctx, inputs)
	require.NoError(t, err)

	// Verify outputs
	assert.Equal(t, "Test Message\n", outputs["stdout"])
	assert.Equal(t, true, outputs["success"])
}

func createTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}
