#!/bin/bash

echo "Testing YAML Workflow Functionality"
echo "==================================="
echo ""

echo "1. Building the project..."
moon run build

echo ""
echo "2. Starting Temporal server (in background)..."
echo "   Run: moon run start"
echo ""

echo "3. Validating research workflow..."
echo "   Command: ono workflow validate --project ./example/research_project/"
echo ""

echo "4. Creating workflow from project..."
echo "   Command: ono workflow create --project ./example/research_project/"
echo ""

echo "5. Running the research workflow..."
echo "   Command: ono workflow run research_report_workflow --input topic=\"Temporal Workflows\" --input max_sources=5"
echo ""

echo "6. Listing workflows..."
echo "   Command: ono workflow list"
echo ""

echo "Example workflow YAML files created in:"
echo "  - ./example/research_project/ (multi-file project)"
echo "  - ./example/simple_workflow.yaml (single file)"
echo ""

echo "The implementation includes:"
echo "✓ YAML parser for workflow definitions"
echo "✓ Workflow compiler to convert YAML to Temporal workflows"
echo "✓ Activity registry and mock implementations"
echo "✓ CLI commands for workflow management"
echo "✓ Template engine for variable interpolation"
echo "✓ Support for sequential and parallel workflows"
echo "✓ Mock implementations for testing"
echo ""

echo "To run the full example:"
echo "1. Start the server: moon run start"
echo "2. In another terminal, run: go run . workflow create --project ./example/research_project/"
echo "3. Then run: go run . workflow run research_report_workflow --input topic=\"AI Agents\""