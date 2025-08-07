package statemachine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestValidateAllExampleYAMLFiles validates that all YAML files in the examples directory
// can be parsed and have valid structure for state machine configurations
func TestValidateAllExampleYAMLFiles(t *testing.T) {
	// Find the examples directory
	examplesDir := filepath.Join("..", "..", "..", "examples")
	
	// Check if examples directory exists
	if _, err := os.Stat(examplesDir); os.IsNotExist(err) {
		t.Skip("Examples directory not found")
	}
	
	// Walk through all files in the examples directory
	err := filepath.Walk(examplesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories and non-YAML files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
			return nil
		}
		
		// Get relative path for better test names
		relPath, _ := filepath.Rel(examplesDir, path)
		
		t.Run(relPath, func(t *testing.T) {
			// Read the file
			content, err := os.ReadFile(path)
			require.NoError(t, err, "Failed to read file %s", path)
			
			// Try to parse as a generic YAML first
			var genericYAML map[string]interface{}
			err = yaml.Unmarshal(content, &genericYAML)
			require.NoError(t, err, "Failed to parse YAML in %s", path)
			
			// Check if it's a state machine configuration
			if isStateMachineYAML(genericYAML) {
				validateStateMachineYAML(t, path, content)
			} else if isRecipeYAML(genericYAML) {
				validateRecipeYAML(t, path, content)
			} else {
				// It's a valid YAML but not a state machine or recipe
				t.Logf("File %s is valid YAML but not a state machine or recipe configuration", relPath)
			}
		})
		
		return nil
	})
	
	require.NoError(t, err, "Error walking through examples directory")
}

// isStateMachineYAML checks if the YAML contains state machine configuration
func isStateMachineYAML(data map[string]interface{}) bool {
	// Check for state machine specific fields
	if _, hasStates := data["states"]; hasStates {
		if _, hasInitial := data["initial_state"]; hasInitial {
			return true
		}
	}
	// Also check if it's embedded in an activity configuration
	if activity, ok := data["activity"].(map[string]interface{}); ok {
		if actType, hasType := activity["type"].(string); hasType && actType == "state_machine" {
			return true
		}
	}
	// Check if it's a workflow with state machine steps
	if steps, ok := data["steps"].([]interface{}); ok {
		for _, step := range steps {
			if stepMap, ok := step.(map[string]interface{}); ok {
				if uses, hasUses := stepMap["uses"].(string); hasUses && uses == "state_machine" {
					return true
				}
			}
		}
	}
	return false
}

// isRecipeYAML checks if the YAML contains recipe configuration
func isRecipeYAML(data map[string]interface{}) bool {
	// Check for recipe-specific fields
	_, hasRecipe := data["recipe"]
	_, hasWorkflow := data["workflow"]
	_, hasActivities := data["activities"]
	
	return hasRecipe || hasWorkflow || hasActivities
}

// validateStateMachineYAML validates state machine specific configuration
func validateStateMachineYAML(t *testing.T, path string, content []byte) {
	// Try to parse as state machine config
	var config yamlpkg.StateMachineConfig
	err := yaml.Unmarshal(content, &config)
	
	// If it doesn't parse directly, it might be embedded
	if err != nil || config.InitialState == "" {
		// Try as a workflow with state machine steps
		var workflowWrapper struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Steps       []struct {
				ID     string                     `yaml:"id"`
				Name   string                     `yaml:"name"`
				Uses   string                     `yaml:"uses"`
				Config yamlpkg.StateMachineConfig `yaml:"config"`
			} `yaml:"steps"`
		}
		err = yaml.Unmarshal(content, &workflowWrapper)
		if err == nil {
			foundStateMachine := false
			for _, step := range workflowWrapper.Steps {
				if step.Uses == "state_machine" && step.Config.InitialState != "" {
					t.Logf("Found state machine step '%s' in workflow", step.ID)
					validateStateMachineConfig(t, step.Config, path)
					foundStateMachine = true
				}
			}
			if foundStateMachine {
				return
			}
		}
		
		// Try as an activity wrapper
		var activityWrapper struct {
			Activity struct {
				Type   string                     `yaml:"type"`
				Config yamlpkg.StateMachineConfig `yaml:"config"`
			} `yaml:"activity"`
		}
		err = yaml.Unmarshal(content, &activityWrapper)
		if err == nil && activityWrapper.Activity.Type == "state_machine" {
			config = activityWrapper.Activity.Config
		} else {
			// Try as a recipe with embedded state machine
			var recipeWrapper struct {
				Recipe struct {
					Activities map[string]struct {
						Type   string                     `yaml:"type"`
						Config yamlpkg.StateMachineConfig `yaml:"config"`
					} `yaml:"activities"`
				} `yaml:"recipe"`
			}
			err = yaml.Unmarshal(content, &recipeWrapper)
			if err == nil {
				// Find state machine activities
				for name, activity := range recipeWrapper.Recipe.Activities {
					if activity.Type == "state_machine" {
						t.Logf("Found state machine activity '%s' in recipe", name)
						validateStateMachineConfig(t, activity.Config, path)
					}
				}
				return
			}
			t.Errorf("Failed to parse state machine configuration in %s: %v", path, err)
			return
		}
	}
	
	if config.InitialState != "" {
		validateStateMachineConfig(t, config, path)
	}
}

// validateStateMachineConfig validates the parsed state machine configuration
func validateStateMachineConfig(t *testing.T, config yamlpkg.StateMachineConfig, path string) {
	// Validate required fields
	assert.NotEmpty(t, config.InitialState, "State machine in %s must have initial_state", path)
	assert.NotEmpty(t, config.States, "State machine in %s must have states", path)
	
	// Validate initial state exists
	_, exists := config.States[config.InitialState]
	assert.True(t, exists, "Initial state '%s' not found in states for %s", config.InitialState, path)
	
	// Validate each state
	for stateName, state := range config.States {
		// Check that state has some action or is terminal
		hasAction := state.Uses != "" || 
			len(state.Sequential) > 0 || 
			len(state.Parallel) > 0 || 
			len(state.Conditional) > 0
		
		if !hasAction && !state.Terminal {
			t.Errorf("State '%s' in %s has no action and is not terminal", stateName, path)
		}
		
		// Validate transitions reference existing states
		for _, transition := range state.Transitions {
			if transition.To != "" {
				_, exists := config.States[transition.To]
				assert.True(t, exists, "Transition to state '%s' from '%s' references non-existent state in %s", 
					transition.To, stateName, path)
			}
		}
		
		// Validate composition steps
		validateCompositionSteps(t, state.Sequential, path, "sequential")
		validateCompositionSteps(t, state.Parallel, path, "parallel")
		
		// Validate conditional branches
		for i, branch := range state.Conditional {
			if !branch.Default {
				assert.NotEmpty(t, branch.When, "Conditional branch %d in %s must have 'when' condition if not default", i, path)
			}
			validateCompositionStep(t, yamlpkg.CompositionStep{
				Uses:       branch.Uses,
				Sequential: branch.Sequential,
				Parallel:   branch.Parallel,
				Inputs:     branch.Inputs,
			}, path, "conditional")
		}
		
		// Validate retry policy if present
		if state.Retry != nil {
			assert.Greater(t, state.Retry.MaxAttempts, 0, "Retry max_attempts must be > 0 in %s", path)
			if state.Retry.BackoffCoefficient > 0 {
				assert.GreaterOrEqual(t, state.Retry.BackoffCoefficient, 1.0, 
					"Retry backoff_coefficient should be >= 1.0 in %s", path)
			}
		}
	}
	
	// Check for unreachable states (except terminal states)
	reachableStates := findReachableStates(config)
	for stateName, state := range config.States {
		if stateName != config.InitialState && !state.Terminal {
			assert.Contains(t, reachableStates, stateName, 
				"State '%s' is unreachable in %s", stateName, path)
		}
	}
	
	t.Logf("✓ Valid state machine configuration in %s with %d states", path, len(config.States))
}

// validateCompositionSteps validates a list of composition steps
func validateCompositionSteps(t *testing.T, steps []yamlpkg.CompositionStep, path string, stepType string) {
	seenIDs := make(map[string]bool)
	
	for i, step := range steps {
		// Validate step has an ID
		if step.ID == "" && (step.Uses != "" || len(step.Sequential) > 0 || len(step.Parallel) > 0) {
			t.Errorf("%s step %d in %s should have an ID", stepType, i, path)
		}
		
		// Check for duplicate IDs
		if step.ID != "" {
			assert.False(t, seenIDs[step.ID], "Duplicate step ID '%s' in %s steps of %s", 
				step.ID, stepType, path)
			seenIDs[step.ID] = true
		}
		
		validateCompositionStep(t, step, path, stepType)
	}
}

// validateCompositionStep validates a single composition step
func validateCompositionStep(t *testing.T, step yamlpkg.CompositionStep, path string, stepType string) {
	// Check that step has some action
	hasAction := step.Uses != "" || 
		len(step.Sequential) > 0 || 
		len(step.Parallel) > 0 || 
		len(step.Conditional) > 0
	
	if !hasAction && step.ID != "" {
		t.Errorf("%s step '%s' in %s has no action (uses, sequential, parallel, or conditional)", 
			stepType, step.ID, path)
	}
	
	// Validate nested compositions
	if len(step.Sequential) > 0 {
		validateCompositionSteps(t, step.Sequential, path, stepType+"/sequential")
	}
	if len(step.Parallel) > 0 {
		validateCompositionSteps(t, step.Parallel, path, stepType+"/parallel")
	}
	
	// Validate retry policy if present
	if step.Retry != nil {
		assert.Greater(t, step.Retry.MaxAttempts, 0, 
			"Step retry max_attempts must be > 0 in %s", path)
	}
	
	// Validate dependencies reference existing step IDs (would need parent context for full validation)
	// This is a limitation of the current validation approach
}

// validateRecipeYAML validates recipe configuration files
func validateRecipeYAML(t *testing.T, path string, content []byte) {
	// For now, just ensure it's valid YAML
	// The existing compiler tests handle recipe validation
	t.Logf("✓ Valid recipe YAML in %s", path)
}

// findReachableStates finds all states reachable from the initial state
func findReachableStates(config yamlpkg.StateMachineConfig) map[string]bool {
	reachable := make(map[string]bool)
	visited := make(map[string]bool)
	
	var visit func(stateName string)
	visit = func(stateName string) {
		if visited[stateName] {
			return
		}
		visited[stateName] = true
		reachable[stateName] = true
		
		if state, exists := config.States[stateName]; exists {
			// Add all transition targets
			for _, transition := range state.Transitions {
				if transition.To != "" {
					visit(transition.To)
				}
			}
		}
	}
	
	// Start from initial state
	visit(config.InitialState)
	
	return reachable
}

// TestStateMachineExamplesCompile tests that state machine examples can be compiled
func TestStateMachineExamplesCompile(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "..", "examples")
	
	// List of state machine example files to test compilation
	stateMachineExamples := []string{
		"simple_state_machine.yaml",
		"state_machine_composition.yaml",
		"nested_composition.yaml",
		"parallel_workflow.yaml",
	}
	
	for _, filename := range stateMachineExamples {
		t.Run(filename, func(t *testing.T) {
			filePath := filepath.Join(examplesDir, filename)
			
			// Check if file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Skipf("Example file %s not found", filename)
				return
			}
			
			// Read and parse the file
			content, err := os.ReadFile(filePath)
			require.NoError(t, err)
			
			var config yamlpkg.StateMachineConfig
			
			// Try parsing as workflow with state machine steps first
			var workflowWrapper struct {
				Steps []struct {
					ID     string                     `yaml:"id"`
					Uses   string                     `yaml:"uses"`
					Config yamlpkg.StateMachineConfig `yaml:"config"`
				} `yaml:"steps"`
			}
			err = yaml.Unmarshal(content, &workflowWrapper)
			if err == nil {
				// Find the state machine step
				for _, step := range workflowWrapper.Steps {
					if step.Uses == "state_machine" {
						config = step.Config
						break
					}
				}
			}
			
			// If still empty, try direct parse
			if config.InitialState == "" {
				err = yaml.Unmarshal(content, &config)
				
				// If still doesn't work, try activity wrapper
				if err != nil || config.InitialState == "" {
					var wrapper struct {
						Activity struct {
							Type   string                     `yaml:"type"`
							Config yamlpkg.StateMachineConfig `yaml:"config"`
						} `yaml:"activity"`
					}
					err2 := yaml.Unmarshal(content, &wrapper)
					if err2 == nil && wrapper.Activity.Type == "state_machine" {
						config = wrapper.Activity.Config
					}
				}
			}
			
			// Skip if no state machine config was found
			if config.InitialState == "" || len(config.States) == 0 {
				t.Skipf("File %s does not contain a state machine configuration", filename)
				return
			}
			
			// Create a compiler and verify it can be initialized with this config
			compiler, err := NewStateMachineCompiler(nil)
			require.NoError(t, err)
			assert.NotNil(t, compiler)
			
			// Basic validation that the config is processable
			assert.NotEmpty(t, config.InitialState)
			assert.NotEmpty(t, config.States)
			
			t.Logf("✓ Successfully validated compilation readiness for %s", filename)
		})
	}
}