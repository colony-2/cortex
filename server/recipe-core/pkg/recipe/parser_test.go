package recipe

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

func TestNewParser(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)
	
	assert.NotNil(t, parser)
	assert.NotNil(t, parser.logger)
}

func TestParseRecipe(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    func(t *testing.T, dir string)
		expectedError bool
		validate      func(t *testing.T, recipe *Recipe)
	}{
		{
			name: "valid recipe with all files",
			setupFiles: func(t *testing.T, dir string) {
				// Create recipe.yaml
				recipeYAML := `recipe:
  name: test-recipe
  version: 1.0.0
  description: Test recipe
  files:
    workflow: workflow.yaml
    activities: activities.yaml
    agents: agents.yaml`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "recipe.yaml"), []byte(recipeYAML), 0644))

				// Create workflow.yaml
				workflowYAML := `name: test-workflow
description: Test workflow
version: 1.0.0
inputs:
  - name: input1
    type: string
    required: true
outputs:
  - name: output1
    type: string
workflow:
  type: sequential
  steps:
    - id: step1
      activity: process-data`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(workflowYAML), 0644))

				// Create activities.yaml
				activitiesYAML := `activities:
  - name: process-data
    description: Process data activity
    timeout: 30s
    inputs:
      - name: data
        type: string
        required: true
    outputs:
      - name: result
        type: string
    implementation:
      type: function
      config:
        handler: processData`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "activities.yaml"), []byte(activitiesYAML), 0644))

				// Create empty agents.yaml
				agentsYAML := `agents: []`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "agents.yaml"), []byte(agentsYAML), 0644))
			},
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "test-recipe", recipe.Name)
				assert.Equal(t, "1.0.0", recipe.Version)
				assert.Equal(t, "Test recipe", recipe.Description)
				assert.NotNil(t, recipe.Workflow)
				assert.Equal(t, "test-workflow", recipe.Workflow.Name)
				assert.Len(t, recipe.Activities, 1)
				assert.Equal(t, "process-data", recipe.Activities[0].Name)
			},
		},
		{
			name: "missing recipe.yaml",
			setupFiles: func(t *testing.T, dir string) {
				// Don't create any files
			},
			expectedError: true,
		},
		{
			name: "invalid recipe.yaml format",
			setupFiles: func(t *testing.T, dir string) {
				invalidYAML := `invalid yaml content {`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "recipe.yaml"), []byte(invalidYAML), 0644))
			},
			expectedError: true,
		},
		{
			name: "missing workflow file",
			setupFiles: func(t *testing.T, dir string) {
				recipeYAML := `recipe:
  name: test-recipe
  version: 1.0.0
  description: Test recipe
  files:
    workflow: workflow.yaml`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "recipe.yaml"), []byte(recipeYAML), 0644))
				// Don't create workflow.yaml
			},
			expectedError: true,
		},
		{
			name: "recipe without optional files",
			setupFiles: func(t *testing.T, dir string) {
				// Create recipe.yaml without activities or agents
				recipeYAML := `recipe:
  name: minimal-recipe
  version: 1.0.0
  description: Minimal recipe
  files:
    workflow: workflow.yaml`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "recipe.yaml"), []byte(recipeYAML), 0644))

				// Create workflow.yaml
				workflowYAML := `name: minimal-workflow
description: Minimal workflow
version: 1.0.0
workflow:
  type: sequential
  steps:
    - id: step1
      activity: built-in-activity`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(workflowYAML), 0644))
			},
			expectedError: false,
			validate: func(t *testing.T, recipe *Recipe) {
				assert.Equal(t, "minimal-recipe", recipe.Name)
				assert.NotNil(t, recipe.Workflow)
				assert.Empty(t, recipe.Activities)
				assert.Empty(t, recipe.Agents)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir := t.TempDir()
			
			// Setup test files
			tt.setupFiles(t, tempDir)
			
			// Create parser
			logger := zaptest.NewLogger(t)
			parser := NewParser(logger)
			
			// Parse recipe
			recipe, err := parser.ParseRecipe(tempDir)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, recipe)
				assert.Equal(t, tempDir, recipe.BasePath)
				assert.Equal(t, filepath.Join(tempDir, "recipe.yaml"), recipe.ManifestPath)
				assert.NotEqual(t, "", recipe.Hash)
				assert.False(t, recipe.LastModified.IsZero())
				
				if tt.validate != nil {
					tt.validate(t, recipe)
				}
			}
		})
	}
}

func TestParseWorkflowFile(t *testing.T) {
	tests := []struct {
		name          string
		workflowYAML  string
		expectedError bool
		validate      func(t *testing.T, wf *yamlpkg.WorkflowDefinition)
	}{
		{
			name: "valid workflow",
			workflowYAML: `name: test-workflow
description: Test workflow
version: 1.0.0
inputs:
  - name: message
    type: string
    required: true
    description: Input message
outputs:
  - name: result
    type: string
    description: Processing result
workflow:
  type: sequential
  steps:
    - id: process
      activity: process-message
      input:
        message: "{{ .inputs.message }}"`,
			expectedError: false,
			validate: func(t *testing.T, wf *yamlpkg.WorkflowDefinition) {
				assert.Equal(t, "test-workflow", wf.Name)
				assert.Equal(t, "Test workflow", wf.Description)
				assert.Equal(t, "1.0.0", wf.Version)
				assert.Len(t, wf.Inputs, 1)
				assert.Equal(t, "message", wf.Inputs[0].Name)
				assert.True(t, wf.Inputs[0].Required)
				assert.Len(t, wf.Outputs, 1)
				assert.Equal(t, "result", wf.Outputs[0].Name)
				assert.Equal(t, "sequential", wf.Workflow.Type)
				assert.Len(t, wf.Workflow.Steps, 1)
			},
		},
		{
			name: "workflow with parallel steps",
			workflowYAML: `name: parallel-workflow
description: Workflow with parallel steps
version: 1.0.0
workflow:
  type: sequential
  steps:
    - id: parallel-group
      parallel:
        - id: task1
          activity: activity1
        - id: task2
          activity: activity2`,
			expectedError: false,
			validate: func(t *testing.T, wf *yamlpkg.WorkflowDefinition) {
				assert.Len(t, wf.Workflow.Steps, 1)
				assert.Len(t, wf.Workflow.Steps[0].Parallel, 2)
			},
		},
		{
			name:          "invalid yaml",
			workflowYAML:  `invalid: yaml: content:`,
			expectedError: true,
		},
		{
			name: "missing required fields",
			workflowYAML: `description: Missing name
workflow:
  steps: []`,
			expectedError: false, // Parser doesn't validate required fields
			validate: func(t *testing.T, wf *yamlpkg.WorkflowDefinition) {
				assert.Equal(t, "", wf.Name)
				assert.Equal(t, "Missing name", wf.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			workflowPath := filepath.Join(tempDir, "workflow.yaml")
			require.NoError(t, os.WriteFile(workflowPath, []byte(tt.workflowYAML), 0644))

			logger := zaptest.NewLogger(t)
			parser := NewParser(logger)

			wf, err := parser.parseWorkflowFile(workflowPath)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, wf)
				if tt.validate != nil {
					tt.validate(t, wf)
				}
			}
		})
	}
}

func TestParseActivitiesFile(t *testing.T) {
	tests := []struct {
		name           string
		activitiesYAML string
		expectedError  bool
		validate       func(t *testing.T, activities []ActivityDefinition)
	}{
		{
			name: "valid activities",
			activitiesYAML: `activities:
  - name: send-email
    description: Send an email
    timeout: 30s
    inputs:
      - name: to
        type: string
        required: true
      - name: subject
        type: string
        required: true
      - name: body
        type: string
        required: true
    outputs:
      - name: messageId
        type: string
    implementation:
      type: function
      config:
        function: sendEmail
  - name: process-data
    description: Process data
    timeout: 1m
    implementation:
      type: container
      config:
        image: processor:latest`,
			expectedError: false,
			validate: func(t *testing.T, activities []ActivityDefinition) {
				require.Len(t, activities, 2)
				
				// First activity
				assert.Equal(t, "send-email", activities[0].Name)
				assert.Equal(t, "function", activities[0].Implementation.Type)
				assert.Equal(t, 30*time.Second, activities[0].Timeout)
				assert.Len(t, activities[0].Inputs, 3)
				assert.Len(t, activities[0].Outputs, 1)
				
				// Second activity
				assert.Equal(t, "process-data", activities[1].Name)
				assert.Equal(t, 60*time.Second, activities[1].Timeout)
				assert.Equal(t, "container", activities[1].Implementation.Type)
			},
		},
		{
			name: "empty activities",
			activitiesYAML: `activities: []`,
			expectedError: false,
			validate: func(t *testing.T, activities []ActivityDefinition) {
				assert.Empty(t, activities)
			},
		},
		{
			name:           "invalid yaml",
			activitiesYAML: `activities: [invalid`,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			activitiesPath := filepath.Join(tempDir, "activities.yaml")
			require.NoError(t, os.WriteFile(activitiesPath, []byte(tt.activitiesYAML), 0644))

			logger := zaptest.NewLogger(t)
			parser := NewParser(logger)

			activities, err := parser.parseActivities(activitiesPath)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, activities)
				}
			}
		})
	}
}

func TestValidateRecipe(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewParser(logger)

	tests := []struct {
		name          string
		recipe        *Recipe
		expectedError bool
		errorContains string
	}{
		{
			name: "valid recipe",
			recipe: &Recipe{
				Name:    "valid-recipe",
				Version: "1.0.0",
				Workflow: &yamlpkg.WorkflowDefinition{
					Name: "valid-workflow",
				},
			},
			expectedError: false,
		},
		{
			name: "missing name",
			recipe: &Recipe{
				Version: "1.0.0",
				Workflow: &yamlpkg.WorkflowDefinition{
					Name: "workflow",
				},
			},
			expectedError: true,
			errorContains: "recipe name is required",
		},
		{
			name: "missing version",
			recipe: &Recipe{
				Name: "recipe",
				Workflow: &yamlpkg.WorkflowDefinition{
					Name: "workflow",
				},
			},
			expectedError: true,
			errorContains: "recipe version is required",
		},
		{
			name: "missing workflow",
			recipe: &Recipe{
				Name:    "recipe",
				Version: "1.0.0",
			},
			expectedError: true,
			errorContains: "workflow definition is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateRecipe(tt.recipe)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}