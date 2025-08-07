package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
)

func TestSimpleWorkflowRegistry(t *testing.T) {
	activityRegistry := compiler.NewActivityRegistry()
	
	// Register an activity
	activityRegistry.RegisterActivity("test-activity")
	
	// Check if activity is registered
	hasActivity := activityRegistry.HasActivity("test-activity")
	require.True(t, hasActivity)
	
	// Check non-existent activity
	hasNonExistent := activityRegistry.HasActivity("non-existent")
	assert.False(t, hasNonExistent)
}

func TestSimpleWorkflowCompiler(t *testing.T) {
	activityRegistry := compiler.NewActivityRegistry()
	activityRegistry.RegisterActivity("test-activity")
	
	compiler := compiler.NewCompiler(activityRegistry)
	assert.NotNil(t, compiler)
}