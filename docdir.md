# Cell Documentation Generator Recipe Specification

## Overview
A recipe that analyzes a cell directory using the git file collector and generates VIBETHIS.md documentation via LLM with native file handling

## Recipe Structure

```yaml
name: doccell
input_schema:
  directory:
    type: string
    required: true
    description: "Path to the cell directory to document"

sequence:
  # Step 1: Collect source files using git file collector
  - id: file_collector
    op: git_file_collector
    inputs:
      context_dir: "{{ .inputs.directory }}"

  # Step 2: Generate VIBETHIS.md using LLM with native file handling and write tool
  - id: generate_doc
    op: llm_inference
    inputs:
      provider: "Gemini"
      model: "gemini-2.5-flash"
      files: "{{ .file_collector.outputs.files }}"  # Native file handling
      file_handling: "native"  # Use native file handling
      system_prompt: |
        You are a technical documentation expert creating documentation for LLM consumption.
        You have access to the source files of a cell.
        Generate clear, concise VIBETHIS.md documentation.
        
        Focus on:
        - Clear functionality description
        - Key interfaces and APIs
        - Usage examples
        - No repetition or unnecessary text
        - Direct, actionable information
      prompt: |
Analyze the provided source code in the project directory and generate a VIBETHIS.md documentation file (or replace existing).

Create documentation with these sections:
1. Overview (2-3 sentences)
2. Architecture (key components and their relationships)
3. Key Interfaces (main APIs/functions with signatures from the API summary)
4. Usage Examples (practical code examples)
5. Configuration (if applicable)

Focus on:
- Clear functionality description
- Key interfaces and APIs
- Usage examples
- No repetition or unnecessary text
- Direct, actionable information
  
Additionally, produce a VIBETHIS.md at the root directory that summarizes all the lower level VIBETHIS.md files.
        
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

```

## Implementation Notes

### Key Design Decisions

1. **Git File Collector Without Patterns**: The `git_file_collector` op is used without specifying file patterns, allowing it to collect all relevant source files based on git status and its own logic.

2. **LLM with Native File Handling**: The LLM inference uses native file handling to process the collected files directly rather than inlining content in prompts.

3**Tool-based Writing**: The LLM uses the write_file tool to directly create documentation files.

### Op Dependencies

This recipe requires these ops to be available:
- `git_file_collector` - From server/git/pkg/gitcollector
- `llm_inference` - Enhanced version from server/ops/pkg/llm

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
3. **LLM-Optimized**: VIBETHIS.md formatted specifically for LLM consumption with native file handling
4. **Git-Aware**: Leverages git file collector to intelligently select relevant files
6. **No Arbitrary Features**: Avoids hardcoded file extension lists, letting git file collector handle file selection intelligently