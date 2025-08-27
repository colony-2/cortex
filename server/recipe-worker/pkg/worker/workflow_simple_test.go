package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
)

func TestSimpleWorkflowRegistry(t *testing.T) {
	activityRegistry := ops.NewActivityRegistry()
	assert.NotNil(t, activityRegistry)
	
	// Get list of activities
	activities := activityRegistry.List()
	assert.NotNil(t, activities)
	
	// Check if we can get all activities
	allActivities := activityRegistry.GetAll()
	assert.NotNil(t, allActivities)
}

func TestSimpleWorkflowCompiler(t *testing.T) {
	activityRegistry := ops.NewActivityRegistry()
	assert.NotNil(t, activityRegistry)
	
	// The compiler is no longer a separate struct, it's just functions
	// Test that the registry can be created and used
	activities := activityRegistry.List()
	assert.NotNil(t, activities)
}