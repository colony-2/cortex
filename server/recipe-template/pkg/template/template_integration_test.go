package template

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_SequenceWithTemplates(t *testing.T) {
	// This test simulates a realistic sequence execution scenario
	// Set initial inputs
	inputs := map[string]interface{}{
		"api_url":     "https://api.example.com",
		"api_key":     "secret123",
		"user_id":     "user-456",
		"max_retries": 3,
	}
	ctx, err := NewResolutionContext("sequence", "data-pipeline", inputs, contextual.JobContext{})
	require.NoError(t, err)

	// Simulate first node: fetch_data
	fetchOutputs := map[string]interface{}{
		"status": 200,
		"body": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": 1, "name": "Item 1"},
				map[string]interface{}{"id": 2, "name": "Item 2"},
			},
			"total": 2,
		},
	}
	ctx.AddSequenceNode("fetch_data", fetchOutputs)

	// Test template resolution for next node inputs
	validateInputTemplate := map[string]interface{}{
		"data":           "{{ sequence.fetch_data.outputs.body }}",
		"expected_count": "{{ sequence.fetch_data.outputs.body.total }}",
		"user":           "{{ inputs.user_id }}",
	}

	resolvedInputs, err := ctx.resolveValue(validateInputTemplate)
	require.NoError(t, err)

	inputsMap := resolvedInputs.(map[string]interface{})
	assert.Equal(t, fetchOutputs["body"], inputsMap["data"])
	assert.Equal(t, int64(2), inputsMap["expected_count"])
	assert.Equal(t, "user-456", inputsMap["user"])

	// Simulate second node: validate
	validateOutputs := map[string]interface{}{
		"valid":             true,
		"validation_errors": []interface{}{},
	}
	ctx.AddSequenceNode("validate", validateOutputs)

	// Simulate third node: transform
	transformOutputs := map[string]interface{}{
		"processed_items": []interface{}{
			map[string]interface{}{"id": 1, "name": "ITEM 1", "processed": true},
			map[string]interface{}{"id": 2, "name": "ITEM 2", "processed": true},
		},
		"metadata": map[string]interface{}{
			"processing_time_ms": 125,
			"success":            true,
		},
	}
	ctx.AddSequenceNode("transform", transformOutputs)

	// Test output mapping templates
	outputTemplates := map[string]interface{}{
		"items":         "{{ sequence.transform.outputs.processed_items }}",
		"is_valid":      "{{ sequence.validate.outputs.valid }}",
		"total_count":   "{{ sequence.fetch_data.outputs.body.total }}",
		"processing_ms": "{{ sequence.transform.outputs.metadata.processing_time_ms }}",
	}

	finalOutputs, err := ctx.resolveValue(outputTemplates)
	require.NoError(t, err)

	outputsMap := finalOutputs.(map[string]interface{})
	assert.Equal(t, transformOutputs["processed_items"], outputsMap["items"])
	assert.Equal(t, true, outputsMap["is_valid"])
	assert.Equal(t, int64(2), outputsMap["total_count"])
	assert.Equal(t, int64(125), outputsMap["processing_ms"])
}

func TestIntegration_StateMachineWithNestedSequence(t *testing.T) {
	// Create state machine context
	inputs := map[string]interface{}{
		"order_id":    "order-789",
		"customer_id": "cust-123",
		"amount":      99.99,
	}
	smCtx, err := NewResolutionContext("state_machine", "order-workflow", inputs, contextual.JobContext{})
	require.NoError(t, err)

	// Execute validate state
	smCtx.AddStateOutput("validate_order", map[string]interface{}{
		"valid": true,
		"validation_details": map[string]interface{}{
			"credit_check":    "passed",
			"inventory_check": "passed",
		},
	})

	// Create process state context (nested sequence)
	processCtx, err := smCtx.NewChildContext("state", "process_order", smCtx.TemplateData.Inputs)
	require.NoError(t, err)

	// Test that process state can access validate state outputs
	template := "{{ states.validate_order.outputs.validation_details.credit_check }}"
	result, err := processCtx.resolveTemplate(template)
	require.NoError(t, err)
	assert.Equal(t, "passed", result)

	// Execute nodes within process state sequence
	processCtx.AddSequenceNode("reserve_inventory", map[string]interface{}{
		"reservation_id": "res-001",
		"items_reserved": 2,
	})

	processCtx.AddSequenceNode("charge_payment", map[string]interface{}{
		"transaction_id": "txn-456",
		"status":         "success",
		"charged_amount": 99.99,
	})

	processCtx.AddSequenceNode("generate_invoice", map[string]interface{}{
		"invoice_id": "inv-789",
		"pdf_url":    "https://invoices.example.com/inv-789.pdf",
	})

	// Test process state output mapping with access to sequence nodes
	processOutputTemplate := map[string]interface{}{
		"transaction_id": "{{ sequence.charge_payment.outputs.transaction_id }}",
		"invoice_id":     "{{ sequence.generate_invoice.outputs.invoice_id }}",
		"reservation_id": "{{ sequence.reserve_inventory.outputs.reservation_id }}",
	}

	processOutputs, err := processCtx.resolveValue(processOutputTemplate)
	require.NoError(t, err)

	processOutputsMap := processOutputs.(map[string]interface{})
	assert.Equal(t, "txn-456", processOutputsMap["transaction_id"])
	assert.Equal(t, "inv-789", processOutputsMap["invoice_id"])
	assert.Equal(t, "res-001", processOutputsMap["reservation_id"])

	// Add process state outputs to parent context
	smCtx.AddStateOutput("process_order", processOutputsMap)

	// Test transition evaluation with sequence node access
	transitionExpr := "sequence.charge_payment.outputs.status == \"success\""
	// For transition evaluation, we need to temporarily add sequence to context
	tempCtx := &ResolutionContext{
		TemplateData: smCtx.TemplateData,
		CELEnv:       processCtx.CELEnv,
	}
	tempCtx.TemplateData.Sequence = processCtx.TemplateData.Sequence

	shouldTransition, err := tempCtx.EvaluateCEL(transitionExpr)
	require.NoError(t, err)
	assert.True(t, shouldTransition)

	// Execute complete state
	completeCtx, err := smCtx.NewChildContext("state", "complete", smCtx.TemplateData.Inputs)
	require.NoError(t, err)

	// Complete state can access all previous states
	completeTemplate := map[string]interface{}{
		"order_id":       "{{ inputs.order_id }}",
		"transaction_id": "{{ states.process_order.outputs.transaction_id }}",
		"invoice_id":     "{{ states.process_order.outputs.invoice_id }}",
		"status":         "completed",
	}

	completeOutputs, err := completeCtx.resolveValue(completeTemplate)
	require.NoError(t, err)

	completeOutputsMap := completeOutputs.(map[string]interface{})
	assert.Equal(t, "order-789", completeOutputsMap["order_id"])
	assert.Equal(t, "txn-456", completeOutputsMap["transaction_id"])
	assert.Equal(t, "inv-789", completeOutputsMap["invoice_id"])
	assert.Equal(t, "completed", completeOutputsMap["status"])
}

func TestIntegration_RetryScenario(t *testing.T) {
	// Test retry tracking with runs
	inputs := map[string]interface{}{
		"endpoint":    "https://flaky-api.example.com",
		"max_retries": 3,
	}
	ctx, err := NewResolutionContext("sequence", "retry-workflow", inputs, contextual.JobContext{})
	require.NoError(t, err)

	// First attempt fails
	ctx.AddSequenceNode("api_call", map[string]interface{}{
		"status":  500,
		"error":   "Internal Server Error",
		"attempt": 1,
	})

	// Second attempt fails
	ctx.AddSequenceNode("api_call", map[string]interface{}{
		"status":  503,
		"error":   "Service Unavailable",
		"attempt": 2,
	})

	// Third attempt succeeds
	ctx.AddSequenceNode("api_call", map[string]interface{}{
		"status":  200,
		"body":    map[string]interface{}{"result": "success"},
		"attempt": 3,
	})

	// Check that we can access runs
	node := ctx.TemplateData.Sequence["api_call"]
	outMap := convertRawToMap(node.Outputs)
	assert.Equal(t, 200, outMap["status"])
	assert.Len(t, node.Runs, 2) // Two previous attempts
	assert.Equal(t, 500, convertRawToMap(node.Runs[0].Outputs)["status"])
	assert.Equal(t, 503, convertRawToMap(node.Runs[1].Outputs)["status"])

	// Test template that references current output
	// Note: Accessing specific runs would need custom CEL functions
	result, err := ctx.resolveTemplate("{{ sequence.api_call.outputs.status }}")
	require.NoError(t, err)
	assert.Equal(t, int64(200), result)
}

func TestIntegration_ComplexCELExpressions(t *testing.T) {
	inputs := map[string]interface{}{
		"threshold":  100,
		"multiplier": 2,
	}
	ctx, err := NewResolutionContext("state_machine", "decision-workflow", inputs, contextual.JobContext{})
	require.NoError(t, err)

	ctx.AddSequenceNode("calculate", map[string]interface{}{
		"base_value":     50,
		"adjusted_value": 150,
		"metrics": map[string]interface{}{
			"count":   10,
			"average": 15.5,
		},
	})

	// Test complex CEL expressions
	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "complex arithmetic",
			expression: "sequence.calculate.outputs.adjusted_value > inputs.threshold",
			expected:   true,
		},
		{
			name:       "nested field access",
			expression: "double(sequence.calculate.outputs.metrics.count) * sequence.calculate.outputs.metrics.average > 100.0",
			expected:   true,
		},
		{
			name:       "combined conditions",
			expression: "sequence.calculate.outputs.base_value * inputs.multiplier == 100 && sequence.calculate.outputs.adjusted_value > 0",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.EvaluateCEL(tt.expression)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test CEL expression
	template := `{{ sequence.calculate.outputs.base_value * 3 }}`
	result, err := ctx.resolveTemplate(template)
	require.NoError(t, err)
	assert.Equal(t, int64(150), result)
}
