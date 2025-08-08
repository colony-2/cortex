package statemachine

import (
	"strings"
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// TestInvalidScopeReferences tests that accessing out-of-scope variables produces appropriate errors
func TestInvalidScopeReferences(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	executor := &mockActivityExecutor{
		results: map[string]map[string]interface{}{
			"activity1": {"result": "value1"},
			"activity2": {"result": "value2"},
		},
	}

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		compiler, err := NewStateMachineCompiler(executor)
		require.NoError(t, err)

		stateCtx := &yamlpkg.StateContext{
			CurrentState: "test",
			Inputs: map[string]interface{}{
				"global_input": "global_value",
			},
			StateOutputs: make(map[string]map[string]interface{}),
			StepOutputs:  make(map[string]interface{}),
		}

		rootScope := NewScopedContext(nil, stateCtx, "root")

		// Test: Nested scope trying to access parent scope steps should fail
		sequentialSteps := []yamlpkg.CompositionStep{
			{
				ID:   "outer_step",
				Uses: "activity1",
			},
			{
				ID: "nested_block",
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "inner_step",
						Uses: "activity2",
						Inputs: map[string]interface{}{
							// This should fail - trying to access outer_step from nested scope
							"invalid_ref": "{{ .Steps.outer_step.result }}",
						},
					},
				},
			},
		}

		outputs, err := compiler.executeSequentialScoped(ctx, sequentialSteps, rootScope)
		require.NoError(t, err) // Execution succeeds but template won't resolve

		// Verify the nested block executed
		assert.Contains(t, outputs, "nested_block")
		nestedOutputs, ok := outputs["nested_block"].(map[string]interface{})
		require.True(t, ok, "nested_block should have outputs")
		
		// Check that the invalid reference didn't resolve to the expected value
		if innerStepOutput, ok := nestedOutputs["inner_step"].(map[string]interface{}); ok {
			if inputData, ok := innerStepOutput["data"].(map[string]interface{}); ok {
				// The template should resolve to <invalid-ref> placeholder
				invalidRef := inputData["invalid_ref"]
				assert.True(t, invalidRef == "<invalid-ref>" || invalidRef == "", 
					"Invalid reference should not resolve to actual value, got: %v", invalidRef)
			}
		}

		return nil
	})

	require.NoError(t, env.GetWorkflowError())
}

// TestCELEvaluationWithInvalidReferences tests CEL expressions that reference out-of-scope variables
func TestCELEvaluationWithInvalidReferences(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)

	stateCtx := &yamlpkg.StateContext{
		CurrentState: "test",
		Inputs:       make(map[string]interface{}),
		StateOutputs: make(map[string]map[string]interface{}),
	}

	// Create parent scope with a step output
	parentScope := NewScopedContext(nil, stateCtx, "parent")
	parentScope.SetStepOutput("parent_step", map[string]interface{}{"value": 10})

	// Create nested scope (doesn't inherit parent's step outputs)
	nestedScope := NewScopedContext(parentScope, stateCtx, "nested")
	nestedScope.SetStepOutput("nested_step", map[string]interface{}{"value": 20})

	// Test 1: Nested scope trying to access parent step should fail
	_, err = compiler.evaluateCELScoped(".Steps.parent_step.value > 0", nil, nestedScope)
	assert.Error(t, err, "Should fail when accessing parent scope step from nested scope")
	assert.Contains(t, err.Error(), "CEL expression")

	// Test 2: Parent scope should not see nested scope steps
	_, err = compiler.evaluateCELScoped(".Steps.nested_step.value > 0", nil, parentScope)
	assert.Error(t, err, "Should fail when accessing nested scope step from parent scope")

	// Test 3: Accessing non-existent step in current scope should fail
	_, err = compiler.evaluateCELScoped(".Steps.nonexistent.value > 0", nil, nestedScope)
	assert.Error(t, err, "Should fail when accessing non-existent step")
}

// TestParallelBranchIsolation tests that parallel branches cannot see each other's steps
func TestParallelBranchIsolation(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	executor := &mockActivityExecutor{}

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		compiler, err := NewStateMachineCompiler(executor)
		require.NoError(t, err)

		stateCtx := &yamlpkg.StateContext{
			CurrentState: "test",
			Inputs:       make(map[string]interface{}),
			StateOutputs: make(map[string]map[string]interface{}),
			StepOutputs:  make(map[string]interface{}),
		}

		rootScope := NewScopedContext(nil, stateCtx, "root")

		// Test: Parallel branches trying to reference each other should fail
		parallelSteps := []yamlpkg.CompositionStep{
			{
				ID: "branch1",
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "branch1_step1",
						Uses: "activity",
					},
					{
						ID:   "branch1_step2",
						Uses: "activity",
						Inputs: map[string]interface{}{
							// Valid: can see own branch's step
							"valid_ref": "{{ .Steps.branch1_step1.output }}",
							// Invalid: cannot see other branch's step
							"invalid_ref": "{{ .Steps.branch2_step1.output }}",
						},
					},
				},
			},
			{
				ID: "branch2",
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "branch2_step1",
						Uses: "activity",
					},
					{
						ID:   "branch2_step2",
						Uses: "activity",
						Inputs: map[string]interface{}{
							// Invalid: cannot see other branch's step
							"invalid_ref": "{{ .Steps.branch1_step1.output }}",
						},
					},
				},
			},
		}

		outputs, err := compiler.executeParallelScoped(ctx, parallelSteps, rootScope)
		require.NoError(t, err)

		// Verify both branches executed
		assert.Contains(t, outputs, "branch1")
		assert.Contains(t, outputs, "branch2")

		// Check that cross-branch references didn't resolve
		if branch1Outputs, ok := outputs["branch1"].(map[string]interface{}); ok {
			if branch1Step2, ok := branch1Outputs["branch1_step2"].(map[string]interface{}); ok {
				if branch1Step2Data, ok := branch1Step2["data"].(map[string]interface{}); ok {
					// The invalid reference to branch2_step1 should not have resolved
					invalidRef := branch1Step2Data["invalid_ref"]
					assert.True(t, invalidRef == "<invalid-ref>" || invalidRef == "",
						"Cross-branch reference should not resolve, got: %v", invalidRef)
				}
			}
		}

		if branch2Outputs, ok := outputs["branch2"].(map[string]interface{}); ok {
			if branch2Step2, ok := branch2Outputs["branch2_step2"].(map[string]interface{}); ok {
				if branch2Step2Data, ok := branch2Step2["data"].(map[string]interface{}); ok {
					// The invalid reference to branch1_step1 should not have resolved
					invalidRef := branch2Step2Data["invalid_ref"]
					assert.True(t, invalidRef == "<invalid-ref>" || invalidRef == "",
						"Cross-branch reference should not resolve, got: %v", invalidRef)
				}
			}
		}

		return nil
	})

	require.NoError(t, env.GetWorkflowError())
}

// TestConditionalBranchIsolation tests that conditional branches maintain scope isolation
func TestConditionalBranchIsolation(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Create mock executor with defined results
	executor := &mockActivityExecutor{
		results: map[string]map[string]interface{}{
			"activity1": {"result": "branch1_result"},
			"activity2": {"result": "branch2_result"},
		},
	}
	
	var result interface{}
	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		compiler, err := NewStateMachineCompiler(executor)
		require.NoError(t, err)

		stateCtx := &yamlpkg.StateContext{
			CurrentState: "test",
			Inputs: map[string]interface{}{
				"condition": true,
			},
			StateOutputs: make(map[string]map[string]interface{}),
			StepOutputs:  make(map[string]interface{}),
		}

		rootScope := NewScopedContext(nil, stateCtx, "root")
		rootScope.SetStepOutput("root_step", map[string]interface{}{"value": "should_not_be_accessible"})

		// Test: Conditional branch trying to access parent scope step in CEL
		conditionalBranches := []yamlpkg.ConditionalBranch{
			{
				// This condition should fail because .Steps.root_step is not in nested scope
				When: ".Steps.root_step.value == 'should_not_be_accessible'",
				Uses: "activity1",
			},
			{
				// This should execute as fallback
				Default: true,
				Uses:    "activity2",
			},
		}

		// Create nested scope for conditional execution
		nestedScope := NewScopedContext(rootScope, stateCtx, "conditional")
		
		// The first branch condition should fail to evaluate (no such key error)
		result, err = compiler.executeConditionalScoped(ctx, conditionalBranches, nestedScope)
		
		// This should fail with an error about the missing key
		require.Error(t, err)
		assert.Contains(t, err.Error(), "root_step", "Error should mention the missing step")
		
		// Now test with a valid condition that can be evaluated
		validBranches := []yamlpkg.ConditionalBranch{
			{
				// This condition uses valid inputs
				When: ".Inputs.condition == true",
				Uses: "activity1",
			},
			{
				Default: true,
				Uses:    "activity2",
			},
		}
		
		result, err = compiler.executeConditionalScoped(ctx, validBranches, nestedScope)
		require.NoError(t, err)
		
		// Should have executed activity1 since condition is true
		if resultMap, ok := result.(map[string]interface{}); ok {
			assert.Equal(t, "branch1_result", resultMap["result"])
		}
		
		return nil
	})
	
	err := env.GetWorkflowError()
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// TestTemplateResolutionWithInvalidPaths tests template resolution with invalid variable paths
func TestTemplateResolutionWithInvalidPaths(t *testing.T) {
	stateCtx := &yamlpkg.StateContext{
		Inputs: map[string]interface{}{
			"valid_input": "test_value",
		},
		StateOutputs: map[string]map[string]interface{}{},
	}

	scope := NewScopedContext(nil, stateCtx, "test")
	scope.SetStepOutput("local_step", map[string]interface{}{"data": "local_data"})

	resolver := NewScopedTemplateResolver()

	testCases := []struct {
		name        string
		template    string
		shouldError bool
		expected    string
	}{
		{
			name:        "Valid local step reference",
			template:    "{{ .Steps.local_step.data }}",
			shouldError: false,
			expected:    "local_data",
		},
		{
			name:        "Invalid step reference",
			template:    "{{ .Steps.nonexistent_step.data }}",
			shouldError: false, // Template execution returns empty/error string
			expected:    "",    // Should not resolve to a valid value
		},
		{
			name:        "Valid input reference",
			template:    "{{ .Inputs.valid_input }}",
			shouldError: false,
			expected:    "test_value",
		},
		{
			name:        "Invalid nested path",
			template:    "{{ .Steps.local_step.nonexistent.deeply.nested }}",
			shouldError: false, // Template handles gracefully
			expected:    "",    // Should not resolve
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inputs := map[string]interface{}{
				"test": tc.template,
			}

			resolved, err := resolver.ResolveInputsScoped(inputs, scope)
			
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				resolvedValue := resolved["test"].(string)
				
				if tc.expected != "" {
					assert.Equal(t, tc.expected, resolvedValue)
				} else {
					// For invalid references, check that it didn't resolve to something unexpected
					assert.True(t, resolvedValue == "<invalid-ref>" || resolvedValue == "" || strings.Contains(resolvedValue, "<no value>"),
						"Invalid reference should not resolve, got: %v", resolvedValue)
				}
			}
		})
	}
}

// TestDeeplyNestedScopeIsolation tests that deeply nested scopes maintain proper isolation
func TestDeeplyNestedScopeIsolation(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	executor := &mockActivityExecutor{}

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		compiler, err := NewStateMachineCompiler(executor)
		require.NoError(t, err)

		stateCtx := &yamlpkg.StateContext{
			CurrentState: "test",
			Inputs:       make(map[string]interface{}),
			StateOutputs: make(map[string]map[string]interface{}),
			StepOutputs:  make(map[string]interface{}),
		}

		rootScope := NewScopedContext(nil, stateCtx, "root")

		// Create a deeply nested structure where inner scopes try to access outer scopes
		steps := []yamlpkg.CompositionStep{
			{
				ID:   "level1_step",
				Uses: "activity",
			},
			{
				ID: "level1_nested",
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "level2_step",
						Uses: "activity",
					},
					{
						ID: "level2_nested",
						Sequential: []yamlpkg.CompositionStep{
							{
								ID:   "level3_step",
								Uses: "activity",
								Inputs: map[string]interface{}{
									// Should NOT see level1_step (grandparent scope)
									"invalid_grandparent": "{{ .Steps.level1_step.output }}",
									// Should NOT see level2_step (parent scope)
									"invalid_parent": "{{ .Steps.level2_step.output }}",
								},
							},
						},
					},
				},
			},
		}

		outputs, err := compiler.executeSequentialScoped(ctx, steps, rootScope)
		require.NoError(t, err)

		// Navigate to the deeply nested output
		if level1Nested, ok := outputs["level1_nested"].(map[string]interface{}); ok {
			if level2Nested, ok := level1Nested["level2_nested"].(map[string]interface{}); ok {
				if level3Step, ok := level2Nested["level3_step"].(map[string]interface{}); ok {
					if level3Data, ok := level3Step["data"].(map[string]interface{}); ok {
						// Verify that the invalid references didn't resolve
						invalidGrandparent := level3Data["invalid_grandparent"]
						assert.True(t, invalidGrandparent == "<invalid-ref>" || invalidGrandparent == "",
							"Grandparent reference should not resolve, got: %v", invalidGrandparent)
						
						invalidParent := level3Data["invalid_parent"]
						assert.True(t, invalidParent == "<invalid-ref>" || invalidParent == "",
							"Parent reference should not resolve, got: %v", invalidParent)
					}
				}
			}
		}

		return nil
	})

	require.NoError(t, env.GetWorkflowError())
}

// TestScopeEncapsulationWithErrors tests that scope errors are properly reported
func TestScopeEncapsulationWithErrors(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	require.NoError(t, err)

	stateCtx := &yamlpkg.StateContext{
		CurrentState: "test",
		Inputs:       make(map[string]interface{}),
		StateOutputs: make(map[string]map[string]interface{}),
	}

	// Create nested scopes
	rootScope := NewScopedContext(nil, stateCtx, "root")
	rootScope.SetStepOutput("root_only", map[string]interface{}{"data": "root"})

	childScope := NewScopedContext(rootScope, stateCtx, "child")
	childScope.SetStepOutput("child_only", map[string]interface{}{"data": "child"})

	// Test various invalid access patterns
	testCases := []struct {
		name       string
		expression string
		scope      *ScopedContext
		shouldFail bool
	}{
		{
			name:       "Child accessing its own step",
			expression: ".Steps.child_only.data == 'child'",
			scope:      childScope,
			shouldFail: false,
		},
		{
			name:       "Child accessing parent step",
			expression: ".Steps.root_only.data == 'root'",
			scope:      childScope,
			shouldFail: true,
		},
		{
			name:       "Parent accessing child step",
			expression: ".Steps.child_only.data == 'child'",
			scope:      rootScope,
			shouldFail: true,
		},
		{
			name:       "Accessing completely non-existent step",
			expression: ".Steps.never_created.value > 0",
			scope:      childScope,
			shouldFail: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := compiler.evaluateCELScoped(tc.expression, nil, tc.scope)
			
			if tc.shouldFail {
				assert.Error(t, err, "Expression should fail: %s", tc.expression)
			} else {
				assert.NoError(t, err, "Expression should succeed: %s", tc.expression)
				assert.True(t, result)
			}
		})
	}
}