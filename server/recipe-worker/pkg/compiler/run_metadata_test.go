package compiler

import (
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func metadataReceiverWorkflow(ctx workflow.Context) (RecipeRunMetadataSignal, error) {
	metadata, err := waitForRecipeRunMetadata(ctx)
	if err != nil {
		return RecipeRunMetadataSignal{}, err
	}
	return *metadata, nil
}

func runMetadataWorkflowTest(t *testing.T, signals []RecipeRunMetadataSignal, before func(index int, env *testsuite.TestWorkflowEnvironment)) RecipeRunMetadataSignal {
	t.Helper()

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(metadataReceiverWorkflow)

	for i, sig := range signals {
		idx := i
		payload := sig
		env.RegisterDelayedCallback(func() {
			if before != nil {
				before(idx, env)
			}
			env.SignalWorkflow(recipeRunMetadataSignalName, payload)
		}, time.Millisecond*time.Duration(idx+1))
	}

	env.ExecuteWorkflow(metadataReceiverWorkflow)

	require.NoError(t, env.GetWorkflowError())

	var result RecipeRunMetadataSignal
	require.NoError(t, env.GetWorkflowResult(&result))
	return result
}

func TestWaitForRecipeRunMetadataBlocksUntilSignal(t *testing.T) {
	signals := []RecipeRunMetadataSignal{{Reason: "ready"}}

	var checked bool
	metadata := runMetadataWorkflowTest(t, signals, func(_ int, env *testsuite.TestWorkflowEnvironment) {
		checked = true
		require.False(t, env.IsWorkflowCompleted(), "workflow should block before receiving metadata")
	})

	require.True(t, checked, "callback should run before signal is delivered")
	assert.NotEmpty(t, metadata.TargetRunID)
	assert.Equal(t, "ready", metadata.Reason)
}

func TestWaitForRecipeRunMetadataIgnoresMismatchedRunIDs(t *testing.T) {
	signals := []RecipeRunMetadataSignal{
		{TargetRunID: "old-run", Reason: "stale"},
		{Reason: "fresh"},
	}

	var firstChecked bool
	metadata := runMetadataWorkflowTest(t, signals, func(index int, env *testsuite.TestWorkflowEnvironment) {
		if index == 0 {
			firstChecked = true
			require.False(t, env.IsWorkflowCompleted(), "workflow must ignore mismatched run signal")
		}
	})

	require.True(t, firstChecked, "mismatched signal should be observed in callback")
	assert.Equal(t, "fresh", metadata.Reason)
	assert.NotEqual(t, "old-run", metadata.TargetRunID)
}

func TestWaitForRecipeRunMetadataUsesLatestMatchingPayload(t *testing.T) {
	signals := []RecipeRunMetadataSignal{
		{Reason: "first", Resume: &RecipeRunMetadataResume{ExecutionPath: []RecipeRunMetadataSegment{{InvocationHash: "a", EventID: 1}}}},
		{Reason: "second", Resume: &RecipeRunMetadataResume{ExecutionPath: []RecipeRunMetadataSegment{{InvocationHash: "b", EventID: 2}}}},
	}

	metadata := runMetadataWorkflowTest(t, signals, nil)

	assert.Equal(t, "second", metadata.Reason)
	if assert.NotNil(t, metadata.Resume) && assert.Len(t, metadata.Resume.ExecutionPath, 1) {
		assert.Equal(t, "b", metadata.Resume.ExecutionPath[0].InvocationHash)
		assert.EqualValues(t, 2, metadata.Resume.ExecutionPath[0].EventID)
	}
}

func TestExecuteRecipeWaitsForMetadataSignal(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)
	registry.EnableActivitiesInWorker(env)

	recipeDef := recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{NodeMetadata: recipe.NodeMetadata{ID: "metadata-root"}},
			SequenceData:   recipe.SequenceData{Sequence: []recipe.Node{}},
		},
	}

	workflowFn := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, recipeDef, inputs)
	}

	env.RegisterWorkflowWithOptions(workflowFn, workflow.RegisterOptions{Name: "metadata-execute"})

	inputs := map[string]interface{}{
		"basegitrepo": "https://example.com/repo.git",
		"basegithash": "abc123",
		"ticketid":    "TEST-1",
		"cellname":    "cells/cell-a",
	}

	env.RegisterDelayedCallback(func() {
		require.False(t, env.IsWorkflowCompleted(), "recipe execution should block before metadata arrival")
		env.SignalWorkflow(recipeRunMetadataSignalName, RecipeRunMetadataSignal{Reason: "go"})
	}, time.Millisecond)

	env.ExecuteWorkflow("metadata-execute", inputs)

	require.NoError(t, env.GetWorkflowError())
}
