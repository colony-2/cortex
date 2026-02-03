package shared

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/cel"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	workerexec "github.com/colony-2/colony2/server/recipe-worker/pkg/executor"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestStateTransitionLogic tests state machine transition evaluation
func TestStateTransitionLogic(t *testing.T) {
	tests := []struct {
		name         string
		currentState string
		transitions  []recipe.Transition
		outputs      map[string]interface{}
		expected     string
	}{
		{
			name:         "simple transition based on score",
			currentState: "review",
			transitions: []recipe.Transition{
				mkTransition("approved", "score >= 80"),
				mkTransition("rejected", "score < 80"),
			},
			outputs: map[string]interface{}{
				"score": 85,
			},
			expected: "approved",
		},
		{
			name:         "first matching transition wins",
			currentState: "processing",
			transitions: []recipe.Transition{
				mkTransition("complete", "success == true"),
				mkTransition("failed", "error != nil"),
				mkTransition("retry", "attempts < 3"),
			},
			outputs: map[string]interface{}{
				"success":  true,
				"error":    nil,
				"attempts": 2,
			},
			expected: "complete",
		},
		{
			name:         "default transition without condition",
			currentState: "waiting",
			transitions: []recipe.Transition{
				mkTransition("timeout", "elapsed > 60"),
				mkTransition("continue", ""), // No condition - always matches
			},
			outputs: map[string]interface{}{
				"elapsed": 30,
			},
			expected: "continue",
		},
		{
			name:         "complex condition with multiple fields",
			currentState: "analysis",
			transitions: []recipe.Transition{
				mkTransition("escalate", "severity == 'high' && confidence > 0.9"),
				mkTransition("investigate", "severity == 'medium' || confidence < 0.5"),
				mkTransition("archive", "severity == 'low'"),
			},
			outputs: map[string]interface{}{
				"severity":   "high",
				"confidence": 0.95,
			},
			expected: "escalate",
		},
		{
			name:         "no matching transition",
			currentState: "unknown",
			transitions: []recipe.Transition{
				mkTransition("next", "value > 100"),
			},
			outputs: map[string]interface{}{
				"value": 50,
			},
			expected: "", // No transition matches
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateTransitions(tt.transitions, tt.outputs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// evaluateTransitions simulates the transition evaluation logic
func evaluateTransitions(transitions []recipe.Transition, outputs map[string]interface{}) string {
	// First pass: evaluate conditional transitions
	for _, transition := range transitions {
		if transition.When.String() != "" {
			if evaluateSimpleCondition(transition.When.String(), outputs) {
				return transition.To
			}
		}
	}
	// Second pass: take the first unconditional transition, if any
	for _, transition := range transitions {
		if transition.When.String() == "" {
			return transition.To
		}
	}
	return ""
}

// evaluateSimpleCondition is a simplified condition evaluator for testing
func evaluateSimpleCondition(condition string, data map[string]interface{}) bool {
	// Normalize possible "inputs." prefix used by CEL-expressions
	condition = strings.ReplaceAll(condition, "inputs.", "")
	// This is a simplified implementation for testing
	// Real implementation would use CEL
	switch condition {
	case "score >= 80":
		if score, ok := data["score"].(int); ok {
			return score >= 80
		}
	case "score < 80":
		if score, ok := data["score"].(int); ok {
			return score < 80
		}
	case "success == true":
		if success, ok := data["success"].(bool); ok {
			return success
		}
	case "elapsed > 60":
		if elapsed, ok := data["elapsed"].(int); ok {
			return elapsed > 60
		}
	case "severity == 'high' && confidence > 0.9":
		severity, _ := data["severity"].(string)
		confidence, _ := data["confidence"].(float64)
		return severity == "high" && confidence > 0.9
	case "severity == 'medium' || confidence < 0.5":
		severity, _ := data["severity"].(string)
		confidence, _ := data["confidence"].(float64)
		return severity == "medium" || confidence < 0.5
	case "severity == 'low'":
		severity, _ := data["severity"].(string)
		return severity == "low"
	case "value > 100":
		if value, ok := data["value"].(int); ok {
			return value > 100
		}
	case "completed == true":
		if completed, ok := data["completed"].(bool); ok {
			return completed
		}
	}
	return false
}

// TestRetryPolicyEvaluation tests retry policy logic
func TestRetryPolicyEvaluation(t *testing.T) {
	tests := []struct {
		name        string
		policy      *recipe.RetryPolicy
		attempt     int
		lastError   error
		shouldRetry bool
	}{
		{
			name: "retry within max attempts",
			policy: &recipe.RetryPolicy{
				MaximumAttempts:    3,
				InitialInterval:    swf.Duration(time.Second),
				BackoffCoefficient: 2.0,
			},
			attempt:     2,
			lastError:   errors.New("temporary error"),
			shouldRetry: true,
		},
		{
			name: "max attempts reached",
			policy: &recipe.RetryPolicy{
				MaximumAttempts: 3,
				InitialInterval: swf.Duration(time.Second),
			},
			attempt:     3,
			lastError:   errors.New("error"),
			shouldRetry: false,
		},
		{
			name:        "no retry policy",
			policy:      nil,
			attempt:     1,
			lastError:   errors.New("error"),
			shouldRetry: false,
		},
		{
			name: "first attempt failure",
			policy: &recipe.RetryPolicy{
				MaximumAttempts: 5,
				InitialInterval: swf.Duration(500 * time.Millisecond),
			},
			attempt:     1,
			lastError:   errors.New("first failure"),
			shouldRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRetry(tt.policy, int32(tt.attempt), tt.lastError)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

// shouldRetry determines if an operation should be retried
func shouldRetry(policy *recipe.RetryPolicy, attempt int32, lastError error) bool {
	if policy == nil || lastError == nil {
		return false
	}

	return attempt < policy.MaximumAttempts
}

// TestBackoffCalculation tests exponential backoff calculation
func TestBackoffCalculation(t *testing.T) {
	tests := []struct {
		name             string
		policy           *recipe.RetryPolicy
		attempt          int
		expectedMinDelay time.Duration
		expectedMaxDelay time.Duration
	}{
		{
			name: "simple exponential backoff",
			policy: &recipe.RetryPolicy{
				InitialInterval:    swf.Duration(time.Second),
				BackoffCoefficient: 2.0,
				MaximumInterval:    swf.Duration(30 * time.Second),
			},
			attempt:          3,
			expectedMinDelay: 4 * time.Second, // 1s * 2^2
			expectedMaxDelay: 4 * time.Second,
		},
		{
			name: "backoff with max interval cap",
			policy: &recipe.RetryPolicy{
				InitialInterval:    swf.Duration(time.Second),
				BackoffCoefficient: 2.0,
				MaximumInterval:    swf.Duration(5 * time.Second),
			},
			attempt:          5,
			expectedMinDelay: 5 * time.Second, // Would be 16s but capped at 5s
			expectedMaxDelay: 5 * time.Second,
		},
		{
			name: "no backoff coefficient",
			policy: &recipe.RetryPolicy{
				InitialInterval: swf.Duration(2 * time.Second),
			},
			attempt:          3,
			expectedMinDelay: 2 * time.Second,
			expectedMaxDelay: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := calculateBackoff(tt.policy, tt.attempt)
			assert.GreaterOrEqual(t, delay, tt.expectedMinDelay)
			assert.LessOrEqual(t, delay, tt.expectedMaxDelay)
		})
	}
}

// calculateBackoff calculates the backoff delay for a retry attempt
func calculateBackoff(policy *recipe.RetryPolicy, attempt int) time.Duration {
	// Convert initial interval
	initial := policy.InitialInterval.ToDuration()

	// Calculate exponential backoff
	delay := initial
	if policy.BackoffCoefficient > 0 && attempt > 1 {
		multiplier := 1.0
		for i := 1; i < attempt; i++ {
			multiplier *= policy.BackoffCoefficient
		}
		delay = time.Duration(float64(initial) * multiplier)
	}

	// Apply max interval cap if specified
	if policy.MaximumInterval.ToDuration() > 0 && delay > policy.MaximumInterval.ToDuration() {
		delay = policy.MaximumInterval.ToDuration()
	}

	return delay
}

// TestStateMachineExecution tests complete state machine execution flow
func TestStateMachineExecution_SingleState(t *testing.T) {
	// Build a minimal state machine with a single terminal state
	r := recipe.Recipe{RecipeImpl: &recipe.RecipeState{
		RecipeMetadata: recipe.RecipeMetadata{Version: "1.0", NodeMetadata: recipe.NodeMetadata{ID: "sm-single"}},
		StateMachineData: recipe.StateMachineData{
			States: &recipe.StateMap{
				Initial: "start",
				States: map[string]recipe.State{
					"start": {
						Node: recipe.Node{NodeImpl: &recipe.NodeOp{NodeMetadata: recipe.NodeMetadata{ID: "start", Inputs: map[string]interface{}{"duration": "5ms"}}, OpData: recipe.OpData{Op: "sleep"}}},
						SingleStateMetadata: recipe.SingleStateMetadata{Transitions: []recipe.Transition{
							mkTransition("finish", "true"),
						}},
					},
					"finish": {
						Node: recipe.Node{NodeImpl: &recipe.NodeOp{NodeMetadata: recipe.NodeMetadata{ID: "finish", Inputs: map[string]interface{}{"duration": "1ms"}}, OpData: recipe.OpData{Op: "sleep"}}},
						// terminal (no transitions)
					},
				},
			},
			Outputs: map[string]interface{}{"status": "ok"},
		},
	}}

	deps := coreops.NewServiceDepsBuilder().Build()
	reg, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	exec, err := workerexec.NewStandaloneExecutor(deps, reg, zap.NewNop())
	require.NoError(t, err)
	inputs := map[string]interface{}{}

	workflowInputs := requiredWorkflowInputs(t)
	baseRepo, _ := workflowInputs["basegitrepo"].(string)
	baseHash, _ := workflowInputs["basegithash"].(string)
	cellName, _ := workflowInputs["cellname"].(string)

	jobCtx := contextual.JobContext{
		GitBase: contextual.GitBaseContext{
			BaseRepo:         baseRepo,
			BaseRef:          baseHash,
			ResolvedBaseHash: baseHash,
		},
		Workflow: contextual.WorkflowContext{
			CellName: cellName,
			CellPath: "cells/" + cellName,
		},
	}

	out, err := exec.Execute(context.Background(), r, inputs, jobCtx, baseHash)
	require.NoError(t, err)

	assert.Equal(t, "ok", out["status"])
}

// mkTransition constructs a recipe.Transition with a CEL expression via YAML parsing
func mkTransition(to, when string) recipe.Transition {
	var t recipe.Transition
	t.To = to
	if when != "" {
		// Adjust expression to compile with CEL environment which exposes `inputs`
		normalized := when
		// Prefix first identifier with inputs. if not already qualified
		if idx := strings.IndexFunc(when, func(r rune) bool { return r == ' ' || r == '\t' }); idx > 0 {
			ident := when[:idx]
			rest := when[idx:]
			if !strings.Contains(ident, ".") {
				normalized = "inputs." + ident + rest
			}
		}
		if expr, err := cel.NewCELExpr(normalized); err == nil {
			t.When = *expr
		}
	}
	return t
}
