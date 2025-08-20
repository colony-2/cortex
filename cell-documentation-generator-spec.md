# Cell Documentation Generator Recipe Specification

## Overview
A recipe that analyzes a cell directory using the git file collector, identifies the primary programming language, generates VIBETHIS.md documentation via LLM with native file handling, and creates API documentation using language-specific tools.

## Recipe Structure

```yaml
name: cell_documentation_generator
description: Generates comprehensive documentation for a cell by analyzing its source files
version: "2.0"

input_schema:
  cell_directory:
    type: string
    required: true
    description: "Path to the cell directory to document"
  
  max_file_size:
    type: number
    default: 100000
    description: "Maximum size of individual files to include (bytes)"
  
  max_total_size:
    type: number
    default: 5000000
    description: "Maximum total size of all files (bytes)"
  
  include_untracked:
    type: boolean
    default: false
    description: "Include untracked git files"
  
  include_staged:
    type: boolean
    default: true
    description: "Include staged git files"

sequence:
  # Step 1: Collect source files using git file collector
  - id: collect_files
    op: git_file_collector
    inputs:
      context_dir: "{{ .inputs.cell_directory }}"
      max_file_size: "{{ .inputs.max_file_size }}"
      max_total_size: "{{ .inputs.max_total_size }}"
      include_staged: "{{ .inputs.include_staged }}"
      include_untracked: "{{ .inputs.include_untracked }}"
      use_gitignore: true
      exclude_binary: true
      auto_detect_type: true
      include_metadata: true
    outputs:
      files: "{{ .outputs.files }}"
      file_count: "{{ .outputs.file_count }}"
      total_size: "{{ .outputs.total_size }}"
      repository: "{{ .outputs.repository }}"
      statistics: "{{ .outputs.statistics }}"

  # Step 2: Generate API documentation using dedicated api_generator op
  - id: generate_api_docs
    op: api_generator
    inputs:
      source_dir: "{{ .inputs.cell_directory }}"
      output_dir: "{{ .inputs.cell_directory }}/vibethis.api"
      detail_level: "standard"
      include_private: false
      output_formats: ["json", "html", "markdown"]
    outputs:
      primary_language: "{{ .outputs.primary_language }}"
      language_stats: "{{ .outputs.language_stats }}"
      documentation_paths: "{{ .outputs.documentation_paths }}"
      api_summary: "{{ .outputs.api_summary }}"

  # Step 3: Generate VIBETHIS.md using LLM with native file handling and write tool
  - id: generate_vibethis
    op: llm_inference
    inputs:
      provider: "OpenAI"
      model: "gpt-4"
      files: "{{ .collect_files.outputs.files }}"  # Native file handling
      file_handling: "native"  # Use native file handling
      max_tokens: 3000
      temperature: 0.3
      system_prompt: |
        You are a technical documentation expert creating documentation for LLM consumption.
        You have access to the source files of a cell/module.
        Generate clear, concise VIBETHIS.md documentation.
        
        Focus on:
        - Clear functionality description
        - Key interfaces and APIs
        - Usage examples
        - No repetition or unnecessary text
        - Direct, actionable information
      prompt: |
        Analyze the provided source files and generate a VIBETHIS.md documentation file.
        
        Repository info: {{ .collect_files.outputs.repository }}
        Statistics: {{ .collect_files.outputs.statistics }}
        Primary language: {{ .generate_api_docs.outputs.primary_language }}
        Language breakdown: {{ .generate_api_docs.outputs.language_stats }}
        
        API Summary:
        {{ .generate_api_docs.outputs.api_summary }}
        
        Create documentation with these sections:
        1. Overview (2-3 sentences)
        2. Architecture (key components and their relationships)
        3. Key Interfaces (main APIs/functions with signatures from the API summary)
        4. Usage Examples (practical code examples)
        5. Configuration (if applicable)
        
        Use the write_file tool to save the documentation as VIBETHIS.md.
      tools:
        - name: write_file
          description: "Write the VIBETHIS.md documentation file"
          parameters:
            type: object
            properties:
              path:
                type: string
                description: "File path (should be VIBETHIS.md)"
              content:
                type: string
                description: "Complete documentation content"
            required: ["path", "content"]
      execute_tools: true
      tool_working_dir: "{{ .inputs.cell_directory }}"
    outputs:
      response: "{{ .outputs.response }}"
      files_written: "{{ .outputs.files_written }}"

  # Step 4: Final summary
  - id: summarize
    op: command_execution
    inputs:
      run: |
        echo "Documentation generation complete!"
        echo ""
        echo "Generated files:"
        echo "- VIBETHIS.md (cell overview)"
        ls -la VIBETHIS.md 2>/dev/null || echo "  [Not found]"
        echo ""
        echo "- vibethis.api/ (API documentation)"
        ls -la vibethis.api/ 2>/dev/null || echo "  [Not found]"
        echo ""
        echo "Statistics:"
        echo "- Files analyzed: {{ .collect_files.outputs.file_count }}"
        echo "- Total size: {{ .collect_files.outputs.total_size }} bytes"
        echo "- Primary language: {{ .generate_api_docs.outputs.primary_language }}"
        echo ""
        echo "Language breakdown:"
        echo '{{ .generate_api_docs.outputs.language_stats }}' | jq -r 'to_entries | .[] | "  - \(.key): \(.value.percentage)% (\(.value.file_count) files, \(.value.code_lines) lines)"'
      working_directory: "{{ .inputs.cell_directory }}"
    outputs:
      summary: "{{ .outputs.stdout }}"
```

## Implementation Notes

### Key Design Decisions

1. **Git File Collector Without Patterns**: The `git_file_collector` op is used without specifying file patterns, allowing it to collect all relevant source files based on git status and its own logic.

2. **Dedicated API Generator Op**: API documentation is generated using a separate `api_generator` op that uses proper language-specific tools rather than grep/find hacks.

3. **LLM with Native File Handling**: The LLM inference uses native file handling to process the collected files directly rather than inlining content in prompts.

4. **Tool-based Writing**: The LLM uses the write_file tool to directly create documentation files.

### Op Dependencies

This recipe requires these ops to be available:
- `git_file_collector` - From server/git/pkg/gitcollector
- `api_generator` - New op as specified in api-generator-op-spec.md
- `llm_inference` - Enhanced version from server/ops/pkg/llm
- `command_execution` - Standard command execution op

## Testing

Create `cell_documentation_generator.test.yaml`:

```yaml
tests:
  - name: "document_go_cell"
    inputs:
      cell_directory: "/test/sample_go_cell"
      max_file_size: 50000
    want:
      summary: "Documentation generation complete!"
    wantErr: false

  - name: "document_javascript_cell"
    inputs:
      cell_directory: "/test/sample_js_cell"
      include_untracked: true
    want:
      summary: "Documentation generation complete!"
    wantErr: false

  - name: "invalid_directory"
    inputs:
      cell_directory: "/nonexistent/path"
    wantErr: true
    wantErrContains: "directory not found"
```

## Benefits

1. **Clean Architecture**: Separates concerns with dedicated ops for file collection, API generation, and documentation
2. **Proper Tools**: Uses language-specific documentation tools (go doc, javadoc, etc.) instead of regex hacks
3. **LLM-Optimized**: VIBETHIS.md formatted specifically for LLM consumption with native file handling
4. **Git-Aware**: Leverages git file collector to intelligently select relevant files
5. **Structured Output**: API generator produces consistent JSON/structured data across languages
6. **No Arbitrary Features**: Avoids hardcoded file extension lists, letting git file collector handle file selection intelligently