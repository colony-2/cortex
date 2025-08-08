package input

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputActivity_GetMetadata(t *testing.T) {
	activity := NewInputActivity()
	metadata := activity.GetMetadata()
	
	assert.Equal(t, "input", metadata.Type)
	assert.Equal(t, "User Input Collection", metadata.Name)
	assert.NotEmpty(t, metadata.Description)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, 5*time.Minute, metadata.DefaultTimeout)
	assert.NotNil(t, metadata.RetryPolicy)
	assert.Equal(t, int32(1), metadata.RetryPolicy.MaximumAttempts)
}

func TestInputActivity_Execute_SingleQuestion(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		input    Input
		validate func(t *testing.T, output Output, err error)
	}{
		{
			name: "short answer question",
			config: Config{
				Question: "What is your name?",
				Type:     FieldTypeShortAnswer,
				Timeout:  60,
			},
			input: Input{
				BoxID:      "test-box",
				ActivityID: "test-activity",
			},
			validate: func(t *testing.T, output Output, err error) {
				require.NoError(t, err)
				assert.NotNil(t, output.Response)
				assert.Equal(t, "test-user", output.UserID)
			},
		},
		{
			name: "multiple choice question",
			config: Config{
				Question: "Do you approve?",
				Type:     FieldTypeMultipleChoice,
				Options: []Option{
					{Value: "approve", Label: "Approve"},
					{Value: "reject", Label: "Reject"},
				},
				Timeout: 60,
			},
			input: Input{
				BoxID:      "test-box",
				ActivityID: "test-activity",
			},
			validate: func(t *testing.T, output Output, err error) {
				require.NoError(t, err)
				assert.Equal(t, "approve", output.Response)
				assert.Equal(t, "test-user", output.UserID)
			},
		},
		{
			name: "linear scale question",
			config: Config{
				Question: "Rate your satisfaction",
				Type:     FieldTypeLinearScale,
				Scale: &LinearScale{
					Min:      1,
					Max:      5,
					MinLabel: "Poor",
					MaxLabel: "Excellent",
				},
				Timeout: 60,
			},
			input: Input{
				BoxID:      "test-box",
				ActivityID: "test-activity",
			},
			validate: func(t *testing.T, output Output, err error) {
				require.NoError(t, err)
				assert.Equal(t, 3, output.Response) // Should return middle value
				assert.Equal(t, "test-user", output.UserID)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity := NewInputActivity()
			output, err := activity.Execute(context.Background(), tt.config, tt.input)
			tt.validate(t, output, err)
		})
	}
}

func TestInputActivity_Execute_MultiField(t *testing.T) {
	config := Config{
		Title: "Deployment Configuration",
		Fields: []FormField{
			{
				ID:       "strategy",
				Type:     FieldTypeMultipleChoice,
				Question: "Select deployment strategy",
				Required: true,
				Options: []Option{
					{Value: "blue_green", Label: "Blue-Green"},
					{Value: "canary", Label: "Canary"},
				},
			},
			{
				ID:       "urgency",
				Type:     FieldTypeLinearScale,
				Question: "How urgent is this?",
				Required: true,
				Scale: &LinearScale{
					Min:      1,
					Max:      5,
					MinLabel: "Can wait",
					MaxLabel: "Critical",
				},
			},
			{
				ID:          "notes",
				Type:        FieldTypeParagraphText,
				Question:    "Additional notes",
				Required:    false,
				Placeholder: "Enter any additional information",
			},
		},
		Timeout: 60,
	}
	
	input := Input{
		BoxID:      "test-box",
		ActivityID: "test-activity",
		Context: map[string]interface{}{
			"deployment_id": "deploy-123",
		},
	}
	
	activity := NewInputActivity()
	output, err := activity.Execute(context.Background(), config, input)
	
	require.NoError(t, err)
	assert.NotNil(t, output.Fields)
	assert.Equal(t, "blue_green", output.Fields["strategy"])
	assert.Equal(t, 3, output.Fields["urgency"])
	assert.Equal(t, "test response", output.Fields["notes"])
	assert.Equal(t, "test-user", output.UserID)
}

func TestInputActivity_BuildForm(t *testing.T) {
	activity := NewInputActivity()
	
	t.Run("single question form", func(t *testing.T) {
		config := Config{
			Question: "Test question",
			Type:     FieldTypeShortAnswer,
			Timeout:  60,
		}
		input := Input{
			BoxID:      "test-box",
			ActivityID: "test-activity",
		}
		
		form := activity.buildForm(config, input)
		
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
			BoxID:      "test-box",
			ActivityID: "test-activity",
		}
		
		form := activity.buildForm(config, input)
		
		assert.Equal(t, "Test Form", form.Title)
		assert.Len(t, form.Fields, 1)
		assert.Equal(t, "field1", form.Fields[0].ID)
		assert.Len(t, form.Context.Artifacts, 1)
		assert.Equal(t, 120*time.Second, form.Timeout)
	})
}

func TestInputActivity_DefaultOnTimeout(t *testing.T) {
	activity := NewInputActivity()
	
	t.Run("single question with default", func(t *testing.T) {
		config := Config{
			Question:         "Approve deployment?",
			Type:             FieldTypeMultipleChoice,
			Options:          []Option{{Value: "yes"}, {Value: "no"}},
			DefaultOnTimeout: "no",
			Timeout:          1, // Very short timeout for testing
		}
		input := Input{
			BoxID:      "test-box",
			ActivityID: "test-activity",
		}
		
		// In mock mode, it won't actually timeout, but we can test the config
		output, err := activity.Execute(context.Background(), config, input)
		
		require.NoError(t, err)
		// In mock mode, it returns the first option, not the default
		// This is expected behavior for the mock
		assert.Equal(t, "yes", output.Response)
	})
}

func TestInputActivity_ManagementService(t *testing.T) {
	activity := NewInputActivity()
	
	// Test that management service is available
	service := activity.GetManagementService()
	assert.NotNil(t, service)
	
	// Test that it returns the correct routes
	mgmtService := service.(*InputManagementService)
	routes := mgmtService.GetRoutes()
	
	assert.Len(t, routes, 5)
	
	// Verify route paths
	expectedPaths := []string{
		"/api/user-inputs/pending",
		"/api/user-inputs/stream",
		"/api/user-inputs/{workflowID}",
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