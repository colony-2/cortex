package shared

import (
	"errors"
	"testing"
	"time"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStateTransitionLogic tests state machine transition evaluation
func TestStateTransitionLogic(t *testing.T) {
	tests := []struct {
		name         string
		currentState string
		transitions  []yamlpkg.Transition
		outputs      map[string]interface{}
		expected     string
	}{
		{
			name:         "simple transition based on score",
			currentState: "review",
			transitions: []yamlpkg.Transition{
				{To: "approved", When: "score >= 80"},
				{To: "rejected", When: "score < 80"},
			},
			outputs: map[string]interface{}{
				"score": 85,
			},
			expected: "approved",
		},
		{
			name:         "first matching transition wins",
			currentState: "processing",
			transitions: []yamlpkg.Transition{
				{To: "complete", When: "success == true"},
				{To: "failed", When: "error != nil"},
				{To: "retry", When: "attempts < 3"},
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
			transitions: []yamlpkg.Transition{
				{To: "timeout", When: "elapsed > 60"},
				{To: "continue"},  // No condition - always matches
			},
			outputs: map[string]interface{}{
				"elapsed": 30,
			},
			expected: "continue",
		},
		{
			name:         "complex condition with multiple fields",
			currentState: "analysis",
			transitions: []yamlpkg.Transition{
				{To: "escalate", When: "severity == 'high' && confidence > 0.9"},
				{To: "investigate", When: "severity == 'medium' || confidence < 0.5"},
				{To: "archive", When: "severity == 'low'"},
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
			transitions: []yamlpkg.Transition{
				{To: "next", When: "value > 100"},
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
func evaluateTransitions(transitions []yamlpkg.Transition, outputs map[string]interface{}) string {
	for _, transition := range transitions {
		if transition.When == "" {
			// No condition means always transition
			return transition.To
		}
		
		// Simplified CEL evaluation for testing
		// In production, this would use the actual CEL evaluator
		if evaluateSimpleCondition(transition.When, outputs) {
			return transition.To
		}
	}
	return ""
}

// evaluateSimpleCondition is a simplified condition evaluator for testing
func evaluateSimpleCondition(condition string, data map[string]interface{}) bool {
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
		policy      *yamlpkg.RetryPolicy
		attempt     int
		lastError   error
		shouldRetry bool
	}{
		{
			name: "retry within max attempts",
			policy: &yamlpkg.RetryPolicy{
				MaxAttempts:        3,
				InitialInterval:    "1s",
				BackoffCoefficient: 2.0,
			},
			attempt:     2,
			lastError:   errors.New("temporary error"),
			shouldRetry: true,
		},
		{
			name: "max attempts reached",
			policy: &yamlpkg.RetryPolicy{
				MaxAttempts:     3,
				InitialInterval: "1s",
			},
			attempt:     3,
			lastError:   errors.New("error"),
			shouldRetry: false,
		},
		{
			name: "no retry policy",
			policy:      nil,
			attempt:     1,
			lastError:   errors.New("error"),
			shouldRetry: false,
		},
		{
			name: "first attempt failure",
			policy: &yamlpkg.RetryPolicy{
				MaxAttempts:     5,
				InitialInterval: "500ms",
			},
			attempt:     1,
			lastError:   errors.New("first failure"),
			shouldRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRetry(tt.policy, tt.attempt, tt.lastError)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

// shouldRetry determines if an operation should be retried
func shouldRetry(policy *yamlpkg.RetryPolicy, attempt int, lastError error) bool {
	if policy == nil || lastError == nil {
		return false
	}
	
	return attempt < policy.MaxAttempts
}

// TestBackoffCalculation tests exponential backoff calculation
func TestBackoffCalculation(t *testing.T) {
	tests := []struct {
		name               string
		policy             *yamlpkg.RetryPolicy
		attempt            int
		expectedMinDelay   time.Duration
		expectedMaxDelay   time.Duration
	}{
		{
			name: "simple exponential backoff",
			policy: &yamlpkg.RetryPolicy{
				InitialInterval:    "1s",
				BackoffCoefficient: 2.0,
				MaxInterval:        "30s",
			},
			attempt:          3,
			expectedMinDelay: 4 * time.Second,  // 1s * 2^2
			expectedMaxDelay: 4 * time.Second,
		},
		{
			name: "backoff with max interval cap",
			policy: &yamlpkg.RetryPolicy{
				InitialInterval:    "1s",
				BackoffCoefficient: 2.0,
				MaxInterval:        "5s",
			},
			attempt:          5,
			expectedMinDelay: 5 * time.Second,  // Would be 16s but capped at 5s
			expectedMaxDelay: 5 * time.Second,
		},
		{
			name: "no backoff coefficient",
			policy: &yamlpkg.RetryPolicy{
				InitialInterval: "2s",
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
func calculateBackoff(policy *yamlpkg.RetryPolicy, attempt int) time.Duration {
	// Parse initial interval
	initial, _ := time.ParseDuration(policy.InitialInterval)
	
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
	if policy.MaxInterval != "" {
		maxInterval, _ := time.ParseDuration(policy.MaxInterval)
		if maxInterval > 0 && delay > maxInterval {
			delay = maxInterval
		}
	}
	
	return delay
}

// TestStateMachineExecution tests complete state machine execution flow
func TestStateMachineExecution(t *testing.T) {
	// Define a state machine for document review workflow
	stateMachine := &yamlpkg.StateMap{
		Initial: "draft",
		States: map[string]yamlpkg.State{
			"draft": {
				Op: "edit_document",
				Transitions: []yamlpkg.Transition{
					{To: "review", When: "completed == true"},
					{To: "draft"},  // Stay in draft if not completed
				},
			},
			"review": {
				Op: "review_document",
				Transitions: []yamlpkg.Transition{
					{To: "approved", When: "score >= 80"},
					{To: "revision", When: "score < 80"},
				},
				Retry: &yamlpkg.RetryPolicy{
					MaxAttempts:     2,
					InitialInterval: "1s",
				},
			},
			"revision": {
				Op: "revise_document",
				Transitions: []yamlpkg.Transition{
					{To: "review"},  // Always go back to review
				},
			},
			"approved": {
				Op: "publish_document",
				// Terminal state - no transitions
			},
		},
	}

	// Test execution flow
	tests := []struct {
		name          string
		startState    string
		mockOutputs   map[string]map[string]interface{}
		expectedPath  []string
		expectedFinal string
	}{
		{
			name:       "successful approval flow",
			startState: "draft",
			mockOutputs: map[string]map[string]interface{}{
				"draft": {
					"completed": true,
				},
				"review": {
					"score": 85,
				},
			},
			expectedPath:  []string{"draft", "review", "approved"},
			expectedFinal: "approved",
		},
		{
			name:       "revision required flow",
			startState: "draft",
			mockOutputs: map[string]map[string]interface{}{
				"draft": {
					"completed": true,
				},
				"review": {
					"score": 65,
				},
				"revision": {
					"revised": true,
				},
			},
			expectedPath:  []string{"draft", "review", "revision", "review"},
			expectedFinal: "review",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate state machine execution
			path := []string{}
			currentState := tt.startState
			
			for i := 0; i < len(tt.expectedPath) && i < 10; i++ { // Limit iterations to prevent infinite loops
				path = append(path, currentState)
				
				// Get state definition
				state, exists := stateMachine.States[currentState]
				if !exists {
					break
				}
				
				// Get mock outputs for this state
				outputs := tt.mockOutputs[currentState]
				if outputs == nil {
					break
				}
				
				// Evaluate transitions
				nextState := evaluateTransitions(state.Transitions, outputs)
				if nextState == "" {
					break
				}
				
				currentState = nextState
			}
			
			// Verify execution path
			require.Equal(t, len(tt.expectedPath), len(path), "Path length mismatch")
			for i, state := range tt.expectedPath {
				assert.Equal(t, state, path[i], "State mismatch at position %d", i)
			}
		})
	}
}