# Ono: YAML-Based Workflow Orchestration with LLM Integration

## What is Ono?

Ono is a powerful workflow orchestration system that allows you to define complex workflows using simple YAML files. Built on top of Temporal, it abstracts away the complexity of distributed systems while providing enterprise-grade reliability and scalability. Think of it as "Infrastructure as Code" for workflows - you describe what you want to happen, and Ono handles the execution details.

## Core Concepts for LLM Understanding

### Recipes
A **recipe** is a reusable workflow definition written in YAML. Recipes describe:
- What inputs the workflow needs
- What steps to execute (activities)
- How data flows between steps
- What outputs are produced

### Jobs
A **job** is a running instance of a recipe. When you execute a recipe with specific inputs, Ono creates a job that tracks the execution progress, handles retries, and manages the state.

### Activities
**Activities** are the individual units of work within a workflow. They can:
- Call REST APIs or gRPC services
- Execute scripts or functions
- **Integrate with LLMs** (OpenAI, Anthropic Claude, Google Gemini)
- Transform data between steps

## Key Features for LLM Integration

### 1. Native LLM Support
Ono provides first-class support for LLM activities with structured outputs:

```yaml
- name: analyze_text_activity
  implementation:
    type: ai_prompt
    config:
      provider: gemini  # or openai, anthropic
      model: gemini-1.5-pro
      structured_output:
        schema:
          type: object
          properties:
            sentiment:
              type: string
              enum: ["positive", "negative", "neutral"]
            key_topics:
              type: array
              items:
                type: string
```

### 2. Data Flow Between Steps
Workflows can chain LLM calls with other activities, using outputs from one step as inputs to another:

```yaml
steps:
  - id: research
    activity: web_search_activity
    outputs:
      results: search_data
      
  - id: analyze
    activity: llm_analyze_activity
    inputs:
      data: "{{ .Steps.research.outputs.results }}"
    outputs:
      analysis: insights
      
  - id: generate_report
    activity: llm_write_activity
    inputs:
      research: "{{ .Steps.research.outputs.results }}"
      analysis: "{{ .Steps.analyze.outputs.analysis }}"
```

### 3. Automatic Retries and Error Handling
Ono handles transient failures (like API rate limits) automatically, with configurable retry policies per activity.

## How to Use Ono

### Quick Start

1. **Start the Ono server** (includes Temporal):
```bash
ono start
```

2. **Create a recipe** in `~/.ono/recipes/`:
```yaml
# my_analysis.yaml
name: document_analyzer
version: "1.0"
inputs:
  - name: document_url
    type: string
    required: true
    
workflow:
  steps:
    - id: fetch_document
      activity: http_fetch
      inputs:
        url: "{{ .Inputs.document_url }}"
        
    - id: analyze_content
      activity: llm_analyze
      inputs:
        content: "{{ .Steps.fetch_document.outputs.content }}"
```

3. **Run the recipe**:
```bash
ono recipe run document_analyzer --input '{"document_url": "https://example.com/doc.pdf"}'
```

### Recipe Structure

#### Single-File Recipe
For simple workflows, everything goes in one YAML file:
```yaml
name: simple_recipe
version: "1.0"

inputs:
  - name: topic
    type: string

workflow:
  steps:
    - id: generate
      activity: generate_content
      
activities:
  - name: generate_content
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        prompt: "Write about {{ .topic }}"
```

#### Multi-File Recipe
For complex workflows, use a directory structure:
```
my_recipe/
├── recipe.yaml      # Manifest
├── workflow.yaml    # Workflow definition
├── activities.yaml  # Activity definitions
└── agents.yaml      # Agent/role definitions
```

## CLI Commands

### Recipe Management
- `ono recipe list` - Show all available recipes
- `ono recipe describe <name>` - View recipe details
- `ono recipe run <name>` - Execute a recipe
- `ono recipe history <name>` - View execution history

### Job Management
- `ono job describe <recipe> <job-id>` - View job details
- `ono job restart <recipe> <job-id>` - Restart failed job
- `ono job cancel <recipe> <job-id>` - Cancel running job

## Advanced Features

### 1. Parallel Execution
Execute multiple activities simultaneously:
```yaml
steps:
  - id: parallel_analysis
    parallel:
      - id: sentiment
        activity: analyze_sentiment
      - id: entities
        activity: extract_entities
      - id: summary
        activity: generate_summary
```

### 2. Conditional Logic
Use activity outputs to control workflow flow:
```yaml
steps:
  - id: check_quality
    activity: quality_check
    
  - id: enhance_if_needed
    activity: enhance_content
    when: "{{ .Steps.check_quality.outputs.score < 0.8 }}"
```

### 3. Custom Activity Types
Extend Ono with custom activity implementations:
- HTTP/REST API calls
- gRPC service integration
- Script execution (Python, JavaScript, etc.)
- Direct function calls
- LLM prompt activities

## Best Practices for LLM Workflows

1. **Use Structured Outputs**: Define JSON schemas for LLM responses to ensure predictable data formats
2. **Chain Activities**: Break complex tasks into smaller, focused LLM calls
3. **Implement Validation**: Add validation activities after LLM calls to verify output quality
4. **Handle Failures Gracefully**: Configure appropriate retry policies and fallback strategies
5. **Monitor Costs**: Track token usage and implement budget controls

## Example: Research and Report Generation

Here's a complete example of a research workflow:

```yaml
name: research_report
version: "1.0"

inputs:
  - name: topic
    type: string
    required: true
  - name: max_sources
    type: int
    default: 5

workflow:
  steps:
    - id: research
      activity: web_research
      inputs:
        query: "{{ .Inputs.topic }} latest developments"
        limit: "{{ .Inputs.max_sources }}"
        
    - id: analyze
      activity: analyze_sources
      inputs:
        sources: "{{ .Steps.research.outputs.results }}"
        
    - id: generate_report
      activity: write_report
      inputs:
        topic: "{{ .Inputs.topic }}"
        research: "{{ .Steps.research.outputs.results }}"
        analysis: "{{ .Steps.analyze.outputs.insights }}"

activities:
  - name: analyze_sources
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        structured_output:
          schema:
            type: object
            properties:
              insights:
                type: array
                items:
                  type: object
                  properties:
                    finding:
                      type: string
                    confidence:
                      type: number
              trends:
                type: array
                items:
                  type: string
```

## Architecture Benefits

1. **Reliability**: Built on Temporal, providing automatic retries, state persistence, and failure recovery
2. **Scalability**: Workflows can run distributed across multiple workers
3. **Observability**: Full execution history and debugging capabilities
4. **Flexibility**: Mix different activity types in a single workflow
5. **Reusability**: Share recipes across teams and projects

## Integration Points

Ono integrates with:
- **LLM Providers**: OpenAI, Anthropic, Google Gemini
- **APIs**: Any REST or gRPC service
- **Databases**: Through activity implementations
- **File Systems**: For data processing workflows
- **Message Queues**: For event-driven workflows

## Getting Started Checklist

1. ✅ Install Ono CLI
2. ✅ Start Ono server with `ono start`
3. ✅ Create your first recipe YAML file
4. ✅ Run the recipe with `ono recipe run`
5. ✅ Monitor execution with `ono job describe`
6. ✅ Iterate and improve your workflows

Ono transforms complex orchestration challenges into simple YAML definitions, making it easy to build reliable, scalable workflows that leverage the power of LLMs and other services.