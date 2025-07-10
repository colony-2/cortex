package activities

import (
	"context"
	"fmt"
	"time"
)

// MockActivityHandler provides mock implementations for testing
type MockActivityHandler struct{}

// NewMockActivityHandler creates a new mock activity handler
func NewMockActivityHandler() *MockActivityHandler {
	return &MockActivityHandler{}
}

// ResearchActivity simulates research activity
func (h *MockActivityHandler) ResearchActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	topic, ok := inputs["topic"].(string)
	if !ok {
		return nil, fmt.Errorf("topic input is required")
	}

	maxSources := 10
	if max, ok := inputs["max_sources"].(int); ok {
		maxSources = max
	}

	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	// Return mock research results
	return map[string]interface{}{
		"research_results": map[string]interface{}{
			"sources": []map[string]interface{}{
				{
					"title": fmt.Sprintf("Understanding %s: A Comprehensive Guide", topic),
					"url":   "https://example.com/guide1",
					"credibility_score": 9.5,
				},
				{
					"title": fmt.Sprintf("Latest Developments in %s", topic),
					"url":   "https://example.com/guide2",
					"credibility_score": 8.7,
				},
			},
			"summary": fmt.Sprintf("Research on %s reveals significant developments in the field. Key findings include recent advancements and future trends.", topic),
			"key_points": []string{
				fmt.Sprintf("%s is rapidly evolving", topic),
				"New technologies are emerging",
				"Industry adoption is increasing",
			},
			"total_sources": maxSources,
		},
	}, nil
}

// AnalyzeActivity simulates analysis activity
func (h *MockActivityHandler) AnalyzeActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	data, ok := inputs["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data input is required")
	}

	topic, _ := inputs["topic"].(string)

	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	// Return mock analysis results
	return map[string]interface{}{
		"analysis_results": map[string]interface{}{
			"trends": []string{
				"Increasing adoption rate",
				"Technology maturation",
				"Market consolidation",
			},
			"insights": []map[string]interface{}{
				{
					"finding": fmt.Sprintf("%s shows strong growth potential", topic),
					"confidence": 0.85,
					"supporting_data": data,
				},
			},
			"recommendations": []string{
				"Continue monitoring developments",
				"Consider strategic investments",
				"Build expertise in key areas",
			},
		},
	}, nil
}

// WriteReportActivity simulates report writing activity
func (h *MockActivityHandler) WriteReportActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	research, ok := inputs["research"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("research input is required")
	}

	analysis, ok := inputs["analysis"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("analysis input is required")
	}

	topic, _ := inputs["topic"].(string)

	// Simulate some work
	time.Sleep(200 * time.Millisecond)

	// Generate mock report
	report := fmt.Sprintf(`# %s Report

## Executive Summary

This comprehensive report examines %s, providing detailed insights based on extensive research and analysis.

## Key Findings

Based on our research data:
%v

And our analysis:
%v

## Detailed Analysis

The research reveals several important trends and patterns in the %s landscape. 
Our analysis indicates strong growth potential with increasing adoption rates across various sectors.

## Conclusions

%s represents a significant opportunity for organizations looking to innovate and stay competitive.
The trends identified in this report suggest continued growth and evolution in this space.

## References

1. Understanding %s: A Comprehensive Guide
2. Latest Developments in %s
3. Industry Analysis Reports

---
Generated on: %s
`, topic, topic, research, analysis, topic, topic, topic, topic, time.Now().Format("2006-01-02 15:04:05"))

	return map[string]interface{}{
		"final_report": report,
	}, nil
}

// GenericActivity provides a generic activity handler for any activity type
func (h *MockActivityHandler) GenericActivity(ctx context.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Simulate some work
	time.Sleep(50 * time.Millisecond)

	// Return generic response
	return map[string]interface{}{
		"status": "completed",
		"activity": activityName,
		"processed_inputs": inputs,
		"timestamp": time.Now().Unix(),
	}, nil
}