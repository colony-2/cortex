// +build ignore

// This is a standalone test program to demonstrate Gemini integration
// Run with: go run test_gemini_integration.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"vibethis/ono/pkg/activities/llm"
	recipecore "github.com/vibethis/server/recipe-core"
)

func main() {
	fmt.Println("Testing Gemini Integration")
	fmt.Println("=========================")
	fmt.Println()

	// Load API key
	apiKey, err := llm.LoadGeminiAPIKey()
	if err != nil {
		log.Fatalf("Failed to load Gemini API key: %v", err)
	}
	fmt.Println("✓ Loaded Gemini API key")

	// Create LLM executor
	executor := llm.NewExecutor()
	provider := llm.NewGeminiProvider(apiKey, "gemini-2.0-flash-exp")
	executor.RegisterProvider("gemini", provider)
	fmt.Println("✓ Created Gemini provider")

	// Define an activity
	activityDef := &recipecore.ActivityDefinition{
		Name:        "test_gemini_activity",
		Description: "Test Gemini report generation",
		Timeout:     2 * time.Minute,
		Outputs: []recipecore.OutputDefinition{
			{Name: "final_report", Type: "string"},
		},
		Implementation: recipecore.ActivityImplementation{
			Type: "ai_prompt",
			Config: map[string]interface{}{
				"provider":    "gemini",
				"model":       "gemini-2.0-flash-exp",
				"temperature": 0.7,
				"prompt": `Generate a brief technical report about "{{ .topic }}". 
				
Include:
1. Executive Summary (2 sentences)
2. Key Points (3 bullet points)
3. Conclusion (1 sentence)

Keep it under 200 words.`,
			},
		},
	}

	// Test inputs
	inputs := map[string]interface{}{
		"topic": "YAML-based Temporal Workflow Orchestration",
	}

	fmt.Println()
	fmt.Printf("Generating report about: %s\n", inputs["topic"])
	fmt.Println("Calling Gemini API...")

	// Execute the activity
	ctx := context.Background()
	start := time.Now()
	result, err := executor.ExecuteAIPromptActivity(ctx, activityDef, inputs)
	if err != nil {
		log.Fatalf("Failed to execute activity: %v", err)
	}
	duration := time.Since(start)

	fmt.Printf("✓ Response received in %.2f seconds\n", duration.Seconds())
	fmt.Println()

	// Display the report
	if report, ok := result["final_report"].(string); ok {
		fmt.Println("Generated Report:")
		fmt.Println("================")
		fmt.Println(report)
	} else {
		fmt.Println("Error: No report in response")
		fmt.Printf("Response: %+v\n", result)
	}
}