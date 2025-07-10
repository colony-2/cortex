#!/bin/bash

echo "Running Gemini Integration Test"
echo "==============================="
echo ""

# Check if API key exists
if [ ! -f ~/gemini.key ]; then
    echo "Error: Gemini API key not found at ~/gemini.key"
    echo "Please create the file with your Gemini API key"
    exit 1
fi

echo "Found Gemini API key"
echo ""

# Run the integration test
echo "Running integration tests..."
go test -v -tags=integration ./pkg/activities/llm -run TestGemini

echo ""
echo "To use Gemini in the workflow:"
echo "1. Update the example workflow to use provider: gemini"
echo "2. Set useMock to false in the activity executor"
echo "3. Run: go run . workflow run research_report_workflow --input topic=\"Your Topic\""