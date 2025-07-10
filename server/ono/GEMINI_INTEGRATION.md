# Gemini Integration for Ono Workflows

This document describes how to use Google Gemini AI with Ono workflows.

## Setup

1. **API Key**: Place your Gemini API key in `~/gemini.key`
   ```bash
   echo "YOUR_GEMINI_API_KEY" > ~/gemini.key
   ```

2. **Model**: The integration uses `gemini-2.0-flash-exp` by default

## Usage

### 1. In Workflow YAML

Update your activity definition to use Gemini:

```yaml
activities:
  - name: ai_report_activity
    implementation:
      type: ai_prompt
      config:
        provider: gemini              # Use Gemini instead of OpenAI
        model: gemini-2.0-flash-exp   # Gemini model
        temperature: 0.7
        prompt: |
          Your prompt here...
```

### 2. Example Workflows

- `example/gemini_workflow.yaml` - Simple Gemini workflow example
- `example/research_project/` - Can be modified to use Gemini

### 3. Testing

Run the integration tests:
```bash
# Run integration tests (requires API key)
go test -v -tags=integration ./pkg/activities/llm -run TestGemini

# Or use the test script
./run_gemini_test.sh
```

Run the standalone test:
```bash
go run test_gemini_integration.go
```

## Implementation Details

The Gemini integration supports:
- ✅ Text generation with temperature control
- ✅ JSON response mode for structured outputs
- ✅ Template-based prompts with variable substitution
- ✅ Error handling and retries

The implementation uses the Gemini REST API directly without external dependencies.

## Switching Between Mock and Real LLM

By default, workflows use mock implementations. To use real Gemini:

1. Update the activity executor in your workflow runner
2. Ensure the API key is available
3. Set the provider to "gemini" in your activity config

## Example Output

When running with Gemini, you'll see real AI-generated content instead of mock data:

```
Generated Report:
================
# YAML-based Temporal Workflow Orchestration

## Executive Summary
YAML-based Temporal workflow orchestration simplifies complex workflow definitions...

## Key Points
- Declarative workflow definitions using YAML syntax
- Seamless integration with Temporal's durable execution
- Support for multiple LLM providers including Gemini

## Conclusion
This approach democratizes workflow orchestration...
```