package ono

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
)

func TestWorkflowHistoryCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedError  bool
		errorContains  string
	}{
		{
			name:          "valid workflow ID",
			args:          []string{"history", "my-workflow-123"},
			expectedError: false,
		},
		{
			name:          "with run ID",
			args:          []string{"history", "my-workflow-123", "--run-id", "run-123"},
			expectedError: false,
		},
		{
			name:          "with event filter",
			args:          []string{"history", "my-workflow-123", "--filter", "activity"},
			expectedError: false,
		},
		{
			name:          "with after event ID",
			args:          []string{"history", "my-workflow-123", "--after-event", "10"},
			expectedError: false,
		},
		{
			name:          "with JSON format",
			args:          []string{"history", "my-workflow-123", "--format", "json"},
			expectedError: false,
		},
		{
			name:          "with limit",
			args:          []string{"history", "my-workflow-123", "--limit", "50"},
			expectedError: false,
		},
		{
			name:          "missing workflow ID",
			args:          []string{"history"},
			expectedError: true,
			errorContains: "requires at least 1 arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command that mimics the real one
			// In a real test environment, you'd mock the Temporal client
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name      string
		eventType enums.EventType
		filter    string
		expected  bool
	}{
		{
			name:      "activity filter matches activity scheduled",
			eventType: enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
			filter:    "activity",
			expected:  true,
		},
		{
			name:      "activity filter matches activity completed",
			eventType: enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED,
			filter:    "activity",
			expected:  true,
		},
		{
			name:      "activity filter doesn't match workflow event",
			eventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			filter:    "activity",
			expected:  false,
		},
		{
			name:      "timer filter matches timer started",
			eventType: enums.EVENT_TYPE_TIMER_STARTED,
			filter:    "timer",
			expected:  true,
		},
		{
			name:      "signal filter matches signal sent",
			eventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			filter:    "signal",
			expected:  true,
		},
		{
			name:      "workflow filter matches workflow started",
			eventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			filter:    "workflow",
			expected:  true,
		},
		{
			name:      "workflow filter matches workflow completed",
			eventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED,
			filter:    "workflow",
			expected:  true,
		},
		{
			name:      "empty filter matches everything",
			eventType: enums.EVENT_TYPE_ACTIVITY_TASK_STARTED,
			filter:    "",
			expected:  true,
		},
		{
			name:      "unknown filter matches everything",
			eventType: enums.EVENT_TYPE_ACTIVITY_TASK_STARTED,
			filter:    "unknown",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &history.HistoryEvent{
				EventType: tt.eventType,
			}
			result := matchesFilter(event, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEventTypeName(t *testing.T) {
	tests := []struct {
		eventType enums.EventType
		expected  string
	}{
		{
			eventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			expected:  "Workflow Execution Started",
		},
		{
			eventType: enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
			expected:  "Activity Task Scheduled",
		},
		{
			eventType: enums.EVENT_TYPE_TIMER_STARTED,
			expected:  "Timer Started",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := getEventTypeName(tt.eventType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorkflowHistoryFlags(t *testing.T) {
	cmd := workflowHistoryCmd
	
	// Ensure all required flags are registered
	require.NotNil(t, cmd.Flag("namespace"))
	require.NotNil(t, cmd.Flag("run-id"))
	require.NotNil(t, cmd.Flag("after-event"))
	require.NotNil(t, cmd.Flag("filter"))
	require.NotNil(t, cmd.Flag("format"))
	require.NotNil(t, cmd.Flag("limit"))
	require.NotNil(t, cmd.Flag("host"))
	require.NotNil(t, cmd.Flag("port"))
	
	// Check defaults
	assert.Equal(t, "default", cmd.Flag("namespace").DefValue)
	assert.Equal(t, "", cmd.Flag("run-id").DefValue)
	assert.Equal(t, "0", cmd.Flag("after-event").DefValue)
	assert.Equal(t, "", cmd.Flag("filter").DefValue)
	assert.Equal(t, "text", cmd.Flag("format").DefValue)
	assert.Equal(t, "100", cmd.Flag("limit").DefValue)
}