# Proposal: YAML-Based Temporal Workflow Orchestration

## Overview

This proposal outlines an enhancement to our Temporal CLI tool that enables users to define workflows and activities using YAML configuration files, inspired by CrewAI's agent-based approach. The system will translate declarative YAML definitions into executable Temporal workflows, making it easier for users to create complex orchestrations without writing Go code.

## Motivation

- **Accessibility**: Lower the barrier to entry for creating Temporal workflows
- **Rapid Prototyping**: Enable quick iteration on workflow designs
- **Configuration as Code**: Maintain workflow definitions in version control
- **Reusability**: Share and compose workflow components easily

## Example Use Case: Research and Report Generation

We'll implement a research and report generation workflow similar to CrewAI's common example, where:
1. A researcher gathers information on a topic
2. An analyst processes the research
3. A writer creates a comprehensive report

## File Organization

Workflows can be organized in two ways:

### Option 1: Single File (Simple Workflows)
```yaml
# research_workflow.yaml - Everything in one file
workflow:
  name: research_report
  # ... workflow definition

activities:
  # ... activity definitions

agents:
  # ... agent definitions
```

### Option 2: Multi-File (Complex Workflows)
```
research_project/
├── workflow.yaml      # Main workflow definition
├── activities.yaml    # Reusable activity definitions
├── agents.yaml       # Agent/role definitions
└── project.yaml      # Project manifest (references other files)
```

## Proposed YAML Schema

### 1. Project Manifest (`project.yaml`)

```yaml
# project.yaml - Entry point that references all components
name: research_report_project
version: "1.0"
description: Research and report generation workflow

files:
  workflow: ./workflow.yaml
  activities: ./activities.yaml
  agents: ./agents.yaml

# Or for single file:
# file: ./research_workflow.yaml
```

### 2. Workflow Definition (`workflow.yaml`)

```yaml
name: research_report_workflow
description: Research a topic and generate a comprehensive report
version: "1.0"

inputs:
  - name: topic
    type: string
    required: true
    description: The topic to research
  - name: max_sources
    type: int
    default: 10
    description: Maximum number of sources to research

outputs:
  - name: report
    type: string
    description: The final generated report
  - name: sources
    type: array
    description: List of sources used

# Data Flow Specification
workflow:
  type: sequential  # or parallel, state_machine
  retry_policy:
    initial_interval: 1s
    maximum_attempts: 3
    
  steps:
    - id: research
      activity: research_activity
      inputs:
        topic: "{{ .Inputs.topic }}"
        max_sources: "{{ .Inputs.max_sources }}"
      outputs:
        # Maps activity output 'research_results' to step output 'research_data'
        research_data: research_results
        
    - id: analyze
      activity: analyze_activity
      inputs:
        # Data flows from previous step's output
        data: "{{ .Steps.research.outputs.research_data }}"
        topic: "{{ .Inputs.topic }}"
      outputs:
        analysis: analysis_results
        
    - id: write_report
      activity: write_report_activity
      inputs:
        # Can reference any previous step's outputs
        research: "{{ .Steps.research.outputs.research_data }}"
        analysis: "{{ .Steps.analyze.outputs.analysis }}"
        topic: "{{ .Inputs.topic }}"
      outputs:
        report: final_report
        
  # Final workflow outputs reference step outputs
  outputs:
    report: "{{ .Steps.write_report.outputs.report }}"
    sources: "{{ .Steps.research.outputs.research_data.sources }}"
```

## Data Flow Specification

### Data Flow Rules

1. **Input Sources**:
   - `{{ .Inputs.* }}` - Workflow inputs
   - `{{ .Steps.<step_id>.outputs.* }}` - Previous step outputs
   - `{{ .Env.* }}` - Environment variables
   - `{{ .Context.* }}` - Workflow context (e.g., workflow ID, run ID)

2. **Output Mapping**:
   - Each step declares its outputs with mappings
   - Activity outputs are mapped to step outputs
   - Step outputs can be referenced by subsequent steps

3. **Data Types**:
   - `string`, `int`, `float`, `bool`
   - `array`, `object` (JSON-compatible)
   - `file` (special type for file handles)

4. **Parallel Steps Data Flow**:
   ```yaml
   steps:
     - id: parallel_group
       parallel:
         - id: task_a
           activity: activity_a
           outputs:
             result_a: output
         - id: task_b
           activity: activity_b  
           outputs:
             result_b: output
     
     - id: merge_results
       activity: merge_activity
       inputs:
         # Access parallel step outputs
         a: "{{ .Steps.parallel_group.task_a.outputs.result_a }}"
         b: "{{ .Steps.parallel_group.task_b.outputs.result_b }}"
   ```

### 3. Activities Definition (`activities.yaml`)

```yaml
activities:
  - name: research_activity
    description: Research information about a given topic
    timeout: 5m
    retry:
      maximum_attempts: 3
      non_retryable_errors:
        - "RATE_LIMIT_ERROR"
    inputs:
      - name: topic
        type: string
        required: true
      - name: max_sources
        type: int
        default: 10
    outputs:
      - name: research_results
        type: object
        schema:
          sources: array
          summary: string
          key_points: array
    implementation:
      type: http  # or grpc, script, function
      config:
        method: POST
        url: "${RESEARCH_API_URL}/search"
        headers:
          Authorization: "Bearer ${RESEARCH_API_KEY}"
        body:
          query: "{{ .topic }}"
          limit: "{{ .max_sources }}"
          
  - name: analyze_activity
    description: Analyze research data and extract insights
    timeout: 3m
    inputs:
      - name: data
        type: object
      - name: topic
        type: string
    outputs:
      - name: analysis_results
        type: object
    implementation:
      type: function
      config:
        handler: analyzers.ProcessResearch
        runtime: go
        
  - name: write_report_activity
    description: Generate a comprehensive report
    timeout: 10m
    inputs:
      - name: research
        type: object
      - name: analysis
        type: object
      - name: topic
        type: string
    outputs:
      - name: final_report
        type: string
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        temperature: 0.7
        prompt: |
          Generate a comprehensive report on "{{ .topic }}" based on the following:
          
          Research Data:
          {{ .research | json }}
          
          Analysis:
          {{ .analysis | json }}
          
          Format the report with:
          1. Executive Summary
          2. Key Findings
          3. Detailed Analysis
          4. Conclusions
          5. References
          
  # Example: LLM activity with structured output
  - name: extract_entities_activity
    description: Extract structured entities from text
    timeout: 2m
    inputs:
      - name: text
        type: string
      - name: entity_types
        type: array
        items: string
    outputs:
      - name: entities
        type: object
        schema:
          $ref: "#/definitions/ExtractedEntities"
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        temperature: 0.1
        response_format: json  # Forces JSON output
        structured_output:
          schema:
            type: object
            properties:
              entities:
                type: array
                items:
                  type: object
                  properties:
                    text:
                      type: string
                      description: The entity text
                    type:
                      type: string
                      enum: ["person", "organization", "location", "date", "email", "phone"]
                    confidence:
                      type: number
                      minimum: 0
                      maximum: 1
                    context:
                      type: string
                      description: Surrounding context
                  required: ["text", "type", "confidence"]
              summary:
                type: string
              metadata:
                type: object
                properties:
                  total_entities:
                    type: integer
                  processing_notes:
                    type: string
            required: ["entities", "summary"]
        prompt: |
          Extract all entities of types {{ .entity_types | join ", " }} from the following text.
          
          Text: {{ .text }}
          
          Return a JSON object with the extracted entities and a brief summary.

# Schema definitions for reusable types
definitions:
  ExtractedEntities:
    type: object
    properties:
      entities:
        type: array
        items:
          $ref: "#/definitions/Entity"
      summary:
        type: string
      metadata:
        type: object
  
  Entity:
    type: object
    properties:
      text:
        type: string
      type:
        type: string
      confidence:
        type: number
    required: ["text", "type"]
```

### 4. Agents/Roles Configuration (`agents.yaml`)

```yaml
agents:
  researcher:
    role: Senior Research Analyst
    capabilities:
      - web_search
      - data_extraction
      - source_validation
    goals:
      - Find comprehensive, accurate information
      - Identify credible sources
      - Extract key insights
    constraints:
      - Only use verified sources
      - Cite all references
      - Avoid biased content
      
  analyst:
    role: Data Analyst
    capabilities:
      - data_processing
      - pattern_recognition
      - statistical_analysis
    goals:
      - Identify trends and patterns
      - Generate actionable insights
      - Validate research findings
      
  writer:
    role: Technical Writer
    capabilities:
      - content_generation
      - formatting
      - editing
    goals:
      - Create clear, concise content
      - Ensure proper structure
      - Maintain consistent tone
```

## Implementation Architecture

### 1. CLI Enhancement

```bash
# New commands for project-based workflows
ono workflow create --project ./research_project/
ono workflow create --file ./simple_workflow.yaml

# Or with explicit project file
ono workflow create --project-file ./research_project/project.yaml

# Running workflows
ono workflow run research_report --input topic="AI Agents"
ono workflow list
ono workflow describe research_report

# Validate configurations
ono workflow validate --project ./research_project/

# Start server with default persistence (./ono.db)
ono start
# Or specify custom database location
ono start --db-filename /path/to/custom.db
# Use in-memory only (not recommended for workflows)
ono start --in-memory
```

### 2. Core Components

```go
// pkg/yaml/parser.go
type WorkflowDefinition struct {
    Name        string
    Description string
    Version     string
    Inputs      []InputDefinition
    Outputs     []OutputDefinition
    Workflow    WorkflowSpec
}

// pkg/compiler/compiler.go
type WorkflowCompiler interface {
    Compile(def WorkflowDefinition) (temporal.Workflow, error)
    CompileActivity(def ActivityDefinition) (temporal.Activity, error)
}

// pkg/runtime/executor.go
type YAMLWorkflowExecutor struct {
    temporalClient client.Client
    compiler       WorkflowCompiler
    registry       ActivityRegistry
}
```

### 3. Activity Implementation Types

1. **HTTP Activities**: Make REST API calls
2. **gRPC Activities**: Call gRPC services
3. **Script Activities**: Execute scripts (Python, JS, etc.)
4. **Function Activities**: Call pre-registered Go functions
5. **AI Prompt Activities**: Integration with LLMs

### 4. Structured Output for LLM Activities

LLM activities support structured outputs through JSON Schema validation across multiple LLM providers:

#### Supported LLM Providers

1. **Google Gemini** - via `github.com/googleapis/go-genai`
2. **Anthropic Claude** - via `github.com/anthropics/anthropic-sdk-go`
3. **OpenAI GPT** - via `github.com/openai/openai-go`

All providers support structured outputs with JSON Schema validation.

#### Provider Configuration Examples

```yaml
# OpenAI Example
- name: openai_structured_activity
  implementation:
    type: ai_prompt
    config:
      provider: openai
      model: gpt-4-turbo-preview
      api_key: ${OPENAI_API_KEY}
      response_format: json_schema
      structured_output:
        name: research_output
        strict: true
        schema:
          # ... JSON Schema definition

# Anthropic Claude Example  
- name: claude_structured_activity
  implementation:
    type: ai_prompt
    config:
      provider: anthropic
      model: claude-3-opus-20240229
      api_key: ${ANTHROPIC_API_KEY}
      max_tokens: 4096
      structured_output:
        schema:
          # ... JSON Schema definition

# Google Gemini Example
- name: gemini_structured_activity
  implementation:
    type: ai_prompt
    config:
      provider: gemini
      model: gemini-1.5-pro
      api_key: ${GEMINI_API_KEY}
      response_mime_type: application/json
      structured_output:
        schema:
          # ... JSON Schema definition
```

#### Example: Complex structured output for research activity
```yaml
- name: research_with_structure
  implementation:
    type: ai_prompt
    config:
      provider: openai  # or anthropic, gemini
      model: gpt-4
      response_format: json
      structured_output:
        schema:
          type: object
          properties:
            research_summary:
              type: object
              properties:
                title:
                  type: string
                abstract:
                  type: string
                  maxLength: 500
                key_findings:
                  type: array
                  minItems: 3
                  maxItems: 10
                  items:
                    type: object
                    properties:
                      finding:
                        type: string
                      importance:
                        type: string
                        enum: ["critical", "high", "medium", "low"]
                      evidence:
                        type: array
                        items:
                          type: string
                    required: ["finding", "importance"]
                sources:
                  type: array
                  items:
                    type: object
                    properties:
                      title:
                        type: string
                      url:
                        type: string
                        format: uri
                      credibility_score:
                        type: number
                        minimum: 0
                        maximum: 10
                      relevant_quotes:
                        type: array
                        items:
                          type: string
                    required: ["title", "url", "credibility_score"]
                recommendations:
                  type: array
                  items:
                    type: string
          required: ["research_summary"]
      prompt: |
        Research "{{ .topic }}" and provide structured findings.
```

Benefits of structured outputs:
- **Type Safety**: Outputs are validated against schema
- **Predictable Integration**: Downstream activities can rely on structure
- **Error Handling**: Invalid LLM responses are caught early
- **Documentation**: Schema serves as documentation

### 5. Template Engine

Use Go templates for variable interpolation:
- `{{ .Inputs.variableName }}` - Access workflow inputs
- `{{ .Steps.stepId.outputs.outputName }}` - Access step outputs
- `{{ .Env.VARIABLE_NAME }}` - Access environment variables
- `{{ .value | json }}` - JSON encode values
- `{{ .array | join ", " }}` - Join array elements

## Benefits

1. **No Code Required**: Users can create workflows without writing Go
2. **Reusable Components**: Activities can be shared across workflows
3. **Version Control Friendly**: YAML files are easy to diff and review
4. **Gradual Complexity**: Start simple, add advanced features as needed
5. **Integration Ready**: Easy to integrate with existing systems

## Example Execution Flow

```bash
# 1. Start the Ono server (uses ./ono.db by default for persistence)
$ ono start
Starting Temporal development server...
Database: ./ono.db
Frontend address: 127.0.0.1:7233
UI address: http://127.0.0.1:8233
Namespace: default
Temporal development server started successfully

# 2. In another terminal, create workflow from project directory
$ ono workflow create --project ./research_project/
Loading project: research_report_project
Validating workflow definition...
Validating activities...
Validating agents...
Workflow 'research_report_workflow' created successfully

# 3. Run the workflow
$ ono workflow run research_report_workflow \
    --input topic="Temporal Workflows" \
    --input max_sources=5
    
Starting workflow: research_report_workflow-1234
Status: Running
Step 1/3: research_activity [Running...]
  > Output: research_data = {sources: 5, summary: "...", key_points: [...]}
Step 2/3: analyze_activity [Running...]  
  > Input: data = {sources: 5, summary: "...", key_points: [...]}
  > Output: analysis = {trends: [...], insights: [...]}
Step 3/3: write_report_activity [Running...]
  > Input: research = {...}, analysis = {...}, topic = "Temporal Workflows"
  > Output: report = "# Temporal Workflows Report\n\n## Executive Summary..."
Workflow completed successfully!

Output saved to: ./output/report_20250709_1234.md

# 4. View workflow history with data flow (works because of persistence)
$ ono workflow history research_report_workflow-1234 --show-data

# 5. List all workflows (persisted across restarts)
$ ono workflow list
NAME                      STATUS      STARTED              COMPLETED
research_report_workflow  Completed   2025-07-09 13:45:00  2025-07-09 13:47:23
```

## Data Flow Example

Here's how data flows through the workflow:

```yaml
# Initial Input
{
  "topic": "Temporal Workflows",
  "max_sources": 5
}

# Step 1: research_activity
Input:  { topic: "Temporal Workflows", max_sources: 5 }
Output: { research_results: { sources: [...], summary: "...", key_points: [...] } }
        ↓ (mapped to research_data)

# Step 2: analyze_activity  
Input:  { data: <research_data from Step 1>, topic: "Temporal Workflows" }
Output: { analysis_results: { trends: [...], insights: [...] } }
        ↓ (mapped to analysis)

# Step 3: write_report_activity
Input:  { 
  research: <research_data from Step 1>,
  analysis: <analysis from Step 2>,
  topic: "Temporal Workflows"
}
Output: { final_report: "# Temporal Workflows Report..." }
        ↓ (mapped to report)

# Final Workflow Output
{
  "report": "# Temporal Workflows Report...",
  "sources": [<sources array from research_data>]
}
```

## Structured LLM Output Example

Here's a complete example using structured outputs:

```yaml
# Workflow using LLM with structured outputs
steps:
  - id: extract_requirements
    activity: llm_extract_requirements
    inputs:
      document: "{{ .Inputs.requirements_doc }}"
    outputs:
      requirements: structured_data
      
  - id: generate_test_cases  
    activity: llm_generate_tests
    inputs:
      # Structured data from previous LLM can be directly used
      requirements: "{{ .Steps.extract_requirements.outputs.requirements }}"
    outputs:
      test_cases: test_suite

# Activity definition with structured output
activities:
  - name: llm_extract_requirements
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        response_format: json
        structured_output:
          schema:
            type: object
            properties:
              functional_requirements:
                type: array
                items:
                  type: object
                  properties:
                    id: { type: string, pattern: "^FR-\\d{3}$" }
                    description: { type: string }
                    priority: { type: string, enum: ["P0", "P1", "P2"] }
                    acceptance_criteria:
                      type: array
                      items: { type: string }
              non_functional_requirements:
                type: array
                items:
                  type: object
                  properties:
                    category: { type: string }
                    requirement: { type: string }
                    metric: { type: string }
            required: ["functional_requirements"]
        prompt: |
          Extract requirements from: {{ .document }}
          
          Categorize into functional and non-functional requirements.
          Use the ID format FR-001, FR-002, etc.
```

The structured output ensures downstream activities receive predictable, typed data that can be validated and processed reliably.

## Migration Path

For users who outgrow YAML configurations:
1. Generate Go code from YAML definitions
2. Extend generated code with custom logic
3. Gradually migrate to full Go implementation

## Implementation Requirements

### LLM Provider Integration

The system must support structured outputs for all three major LLM providers:

1. **OpenAI Integration**
   - SDK: `github.com/openai/openai-go`
   - Structured output via `response_format: { type: "json_schema", json_schema: {...} }`
   - Models: GPT-4, GPT-4 Turbo, GPT-3.5 Turbo

2. **Anthropic Integration**
   - SDK: `github.com/anthropics/anthropic-sdk-go`
   - Structured output via tool use
   - Models: Claude 4 Opus, Claude 4 Sonnet, Claude 4 Haiku

3. **Google Gemini Integration**
   - SDK: `github.com/googleapis/go-genai`
   - Structured output via ` ResponseSchema: &genai.Schema{..}`
   - Models: Gemini 2.5 Pro, Gemini 2.5 Flash

### Provider-Agnostic Activity Example

```yaml
activities:
  - name: analyze_sentiment
    description: Analyze sentiment across different LLM providers
    implementation:
      type: ai_prompt
      config:
        # Provider can be set via environment or per-activity
        provider: ${LLM_PROVIDER}  # openai, anthropic, or gemini
        model: ${LLM_MODEL}
        structured_output:
          schema:
            type: object
            properties:
              sentiment:
                type: string
                enum: ["positive", "negative", "neutral", "mixed"]
              confidence:
                type: number
                minimum: 0
                maximum: 1
              aspects:
                type: array
                items:
                  type: object
                  properties:
                    aspect:
                      type: string
                    sentiment:
                      type: string
                      enum: ["positive", "negative", "neutral"]
                    keywords:
                      type: array
                      items:
                        type: string
            required: ["sentiment", "confidence", "aspects"]
        prompt: |
          Analyze the sentiment of the following text:
          {{ .text }}
```

## Next Steps

1. Implement YAML parser and validator
2. Build workflow compiler
3. Create activity registry and implementations
4. **Integrate LLM providers** (OpenAI, Anthropic, Gemini) with structured output support
5. Add CLI commands for workflow management
6. Implement example workflows
7. Write comprehensive documentation

This approach provides a CrewAI-like experience while leveraging Temporal's powerful orchestration capabilities and supporting multiple LLM providers with type-safe structured outputs.