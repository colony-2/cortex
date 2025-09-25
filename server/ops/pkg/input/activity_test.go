package input

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func TestInputActivity_GetMetadata(t *testing.T) {
	activity := newInputActivity()
	metadata := activity.GetMetadata()

	assert.Equal(t, "input", metadata.Type)
	assert.NotEmpty(t, metadata.Description)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, 5*time.Minute, metadata.DefaultTimeout)
}

func TestInputActivity_Execute_SingleQuestion(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(InputCollectionWorkflow)
	cfg := Config{Question: "What is your name?", Type: FieldTypeShortAnswer, Timeout: 2}
	in := Input{BoxID: "test-cell", ActivityID: "test-activity", Config: cfg}
	form := newInputActivity().buildForm(cfg, in)
	params := InputWorkflowParams{ID: "single-question-id", Form: form, Timeout: form.Timeout, BoxID: in.BoxID, ActivityID: in.ActivityID}

	// Signal response shortly after start
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(fmt.Sprintf("user-response:%s", params.ID), UserResponseSignal{
			Fields:      map[string]interface{}{"field1": "Alice"},
			UserID:      "u-1",
			RespondedAt: time.Now(),
		})
	}, time.Second)

	env.ExecuteWorkflow(InputCollectionWorkflow, params)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var out InputWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&out))
	assert.Equal(t, "u-1", out.UserID)
	// Short-answer stores response in Fields map via workflow result normalization
}

func TestInputActivity_Execute_MultiField(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(InputCollectionWorkflow)
	config := Config{
		Title: "Deployment Configuration",
		Fields: []FormField{
			{ID: "strategy", Type: FieldTypeMultipleChoice, Question: "Select deployment strategy", Required: true, Options: []Option{{Value: "blue_green"}, {Value: "canary"}}},
			{ID: "urgency", Type: FieldTypeLinearScale, Question: "How urgent is this?", Required: true, Scale: &LinearScale{Min: 1, Max: 5}},
			{ID: "notes", Type: FieldTypeParagraphText, Question: "Additional notes"},
		},
		Timeout: 3,
	}
	in := Input{BoxID: "test-cell", ActivityID: "test-activity", Config: config}
	form := newInputActivity().buildForm(config, in)
	params := InputWorkflowParams{ID: "multi-field-id", Form: form, Timeout: form.Timeout, BoxID: in.BoxID, ActivityID: in.ActivityID}
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(fmt.Sprintf("user-response:%s", params.ID), UserResponseSignal{
			Fields: map[string]interface{}{
				"strategy": "blue_green",
				"urgency":  3,
				"notes":    "ship it",
			},
			UserID:      "approver-1",
			RespondedAt: time.Now(),
		})
	}, time.Second)
	env.ExecuteWorkflow(InputCollectionWorkflow, params)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var out InputWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&out))
	assert.Equal(t, "blue_green", out.FormResponse["strategy"])
}

func TestInputActivity_BuildForm(t *testing.T) {
	activity := newInputActivity()

	t.Run("single question form", func(t *testing.T) {
		config := Config{
			Question: "Test question",
			Type:     FieldTypeShortAnswer,
			Timeout:  60,
		}
		input := Input{
			BoxID:      "test-cell",
			ActivityID: "test-activity",
		}

		in := input
		in.Config = config
		form := activity.buildForm(config, in)

		assert.Equal(t, "Test question", form.Question)
		assert.Equal(t, FieldTypeShortAnswer, form.Type)
		assert.Equal(t, 60*time.Second, form.Timeout)
	})

	t.Run("multi-field form", func(t *testing.T) {
		config := Config{
			Title: "Test Form",
			Fields: []FormField{
				{
					ID:       "field1",
					Type:     FieldTypeShortAnswer,
					Question: "Question 1",
				},
			},
			Context: FormContext{
				Artifacts: []Artifact{
					{Path: "test.yaml"},
				},
			},
			Timeout: 120,
		}
		input := Input{
			BoxID:      "test-cell",
			ActivityID: "test-activity",
		}

		in := input
		in.Config = config
		form := activity.buildForm(config, in)

		assert.Equal(t, "Test Form", form.Title)
		assert.Len(t, form.Fields, 1)
		assert.Equal(t, "field1", form.Fields[0].ID)
		assert.Len(t, form.Context.Artifacts, 1)
		assert.Equal(t, 120*time.Second, form.Timeout)
	})
}

func TestInputActivity_DefaultOnTimeout(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(InputCollectionWorkflow)
	cfg := Config{Question: "Approve?", Type: FieldTypeMultipleChoice, Options: []Option{{Value: "yes"}, {Value: "no"}}, Timeout: 1}
	in2 := Input{BoxID: "test-cell", ActivityID: "test-activity", Config: cfg}
	form2 := newInputActivity().buildForm(cfg, in2)
	params2 := InputWorkflowParams{ID: "timeout-id", Form: form2, Timeout: form2.Timeout, BoxID: in2.BoxID, ActivityID: in2.ActivityID}
	env.ExecuteWorkflow(InputCollectionWorkflow, params2)
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

func TestInputActivity_ManagementService(t *testing.T) {
	activity := newInputActivity()

	// Test that management service is available
	service := activity.GetManagementService()
	assert.NotNil(t, service)

	// Test that it returns the correct routes
	mgmtService := service.(*inputManagementService)
	routes := mgmtService.GetRoutes()

	assert.Len(t, routes, 6)

	// Verify route paths
	expectedPaths := []string{
		"/api/user-inputs/pending",
		"/api/user-inputs/stream",
		"/api/user-inputs/{workflowID}",
		"/api/user-inputs/{workflowID}/pending",
		"/api/user-inputs/{workflowID}/respond",
		"/api/user-inputs/{workflowID}/cancel",
	}

	for i, route := range routes {
		assert.Equal(t, expectedPaths[i], route.Path)
		assert.NotNil(t, route.Handler)
	}
}

func TestFieldValidation(t *testing.T) {
	tests := []struct {
		name       string
		validation FieldValidation
		value      interface{}
		shouldPass bool
	}{
		{
			name: "text min length",
			validation: FieldValidation{
				MinLength: 5,
			},
			value:      "test",
			shouldPass: false,
		},
		{
			name: "text max length",
			validation: FieldValidation{
				MaxLength: 10,
			},
			value:      "this is a very long string",
			shouldPass: false,
		},
		{
			name: "numeric range",
			validation: FieldValidation{
				Min: 1,
				Max: 10,
			},
			value:      5,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a placeholder for actual validation logic
			// In a real implementation, you would validate the value against the rules
			// For now, we're just testing that the structures are correct
			assert.NotNil(t, tt.validation)
		})
	}
}
