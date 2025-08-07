# LLM Activity Mode Enhancement Specification

## Overview
Enhance the LLM activity to support two operational modes: **Simple Mode** (default) and **Persona Mode** (advanced configuration).

## Operating Modes

### Simple Mode
- Default behavior
- Direct LLM interaction without persona configuration
- Existing functionality remains unchanged
- No additional configuration required

### Persona Mode
- Configurable persona-based interaction
- Defines role, capabilities, goals, and constraints
- Enables specialized behavior patterns
- Structured prompt engineering through configuration

## Configuration Schema

### Activity Configuration Structure
```yaml
type: llm
config:
  mode: persona  # Options: "simple" (default) | "persona"
  model: gpt-4  # LLM model selection
  
  # Persona configuration (only when mode: persona)
  persona:
    role: string  # Required: The role/identity of the LLM
    
    capabilities:  # Optional: List of capabilities
      - string
    
    goals:  # Optional: List of objectives
      - string
    
    constraints:  # Optional: List of limitations/rules
      - string
    
    context:  # Optional: Additional context or instructions
      - string
```

## Example Configurations

### Research Analyst Persona
```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  persona:
    role: Senior Research Analyst
    capabilities:
      - web_search
      - data_extraction
      - source_validation
      - trend_analysis
    goals:
      - Find comprehensive, accurate information
      - Identify credible sources
      - Extract key insights
      - Provide evidence-based analysis
    constraints:
      - Only use verified sources
      - Cite all references
      - Avoid biased content
      - Maintain objectivity
```

### Technical Writer Persona
```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  persona:
    role: Technical Documentation Specialist
    capabilities:
      - technical_writing
      - api_documentation
      - code_examples
      - diagram_creation
    goals:
      - Create clear, concise documentation
      - Ensure technical accuracy
      - Maintain consistent style
      - Optimize for developer experience
    constraints:
      - Follow documentation standards
      - Use appropriate technical terminology
      - Include practical examples
      - Avoid unnecessary complexity
```

### Customer Support Agent Persona
```yaml
type: llm
config:
  mode: persona
  model: gpt-3.5-turbo
  persona:
    role: Customer Support Specialist
    capabilities:
      - issue_resolution
      - empathetic_communication
      - knowledge_base_access
      - ticket_escalation
    goals:
      - Resolve customer issues quickly
      - Provide helpful, friendly assistance
      - Maintain customer satisfaction
      - Document solutions for future reference
    constraints:
      - Use professional, friendly tone
      - Follow support protocols
      - Protect customer privacy
      - Escalate complex issues appropriately
```

## Implementation Requirements

### 1. Configuration Parsing
- Detect `mode` field in config (default to "simple" if not present)
- When mode is "persona", validate persona configuration
- Required field: `role`
- Optional fields: `capabilities`, `goals`, `constraints`, `context`
- Auto-detect provider from model name or explicit provider field
- Select appropriate file handling strategy based on provider capabilities

### 2. Prompt Input and Construction

#### Input Sources
The LLM activity receives prompts from two primary sources:

1. **Direct Input**: User-provided prompt passed as activity input
   - Comes from upstream activities or initial workflow input
   - Stored in activity's input data structure
   - Example: `input.prompt` or `input.message`

2. **Template-based Input**: Prompt constructed from templates
   - Uses workflow variables and context
   - May combine multiple data sources
   - Example: Template with placeholders like `{user_query}`, `{context}`

#### Prompt Assembly Process

**Simple Mode:**
```
Final Prompt = User Input Prompt
```
- Direct pass-through of input prompt to LLM
- No modification or enhancement
- System prompt (if any) comes from default LLM configuration

**Persona Mode:**
```
Final Prompt = System Prompt (Persona) + User Input Prompt
```

The complete prompt structure:

1. **System Prompt** (Generated from persona configuration):
```
You are a {role}.

Your capabilities include:
{capabilities_list}

Your goals are to:
{goals_list}

You must adhere to these constraints:
{constraints_list}

{additional_context}
```

2. **User Prompt** (From input):
```
{input.prompt or input.message}
```

3. **Combined Message Structure** (sent to LLM):
```json
{
  "messages": [
    {
      "role": "system",
      "content": "<generated persona prompt>"
    },
    {
      "role": "user", 
      "content": "<input prompt from activity input>"
    }
  ]
}
```

#### Input Field Mapping
```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  input_field: prompt  # Optional: specify which input field contains the prompt (default: "prompt")
  persona:
    role: Senior Research Analyst
    # ... rest of persona config
```

#### Example Flow

**Input to Activity:**
```json
{
  "prompt": "What are the latest trends in renewable energy?",
  "context": {
    "region": "North America",
    "timeframe": "2024"
  }
}
```

**Simple Mode Processing:**
- Sends directly: "What are the latest trends in renewable energy?"

**Persona Mode Processing:**
- System: "You are a Senior Research Analyst. Your capabilities include: web_search, data_extraction..."
- User: "What are the latest trends in renewable energy?"

#### Dynamic Prompt Enhancement
In persona mode, the activity can optionally enhance the user prompt based on persona configuration:

```yaml
persona:
  role: Research Analyst
  prompt_enhancement:
    prefix: "Please provide a comprehensive analysis:"
    suffix: "Include citations for all sources."
    context_injection: true  # Inject available context into prompt
```

Results in:
```
System: [Persona configuration as system prompt]
User: "Please provide a comprehensive analysis: {original_prompt} Include citations for all sources. Context: {injected_context}"
```

### 3. File Inclusion in Prompts

#### Overview
The LLM activity supports including arbitrary files in the prompt context using the native file handling capabilities of each LLM provider. This ensures optimal performance, token efficiency, and proper handling of different file types (text, images, PDFs, etc.). The activity automatically adapts to each provider's API for file inclusion.

#### Provider-Specific File Handling

Different LLM providers support files in different ways:

1. **OpenAI (GPT-4, GPT-4V)**: 
   - Supports file uploads via Assistants API or base64 encoding for vision models
   - Images can be provided as URLs or base64
   - Text files can be attached to messages

2. **Anthropic (Claude)**:
   - Supports direct file content in messages
   - Images via base64 encoding
   - PDF support with native parsing

3. **Google (Gemini)**:
   - File uploads via Files API
   - Native multimodal support
   - Direct file URIs in prompts

4. **Local/Custom Models**:
   - Fallback to text injection
   - Custom preprocessing as needed

#### Configuration
```yaml
type: llm
config:
  mode: persona  # or simple
  model: gpt-4
  
  # File inclusion configuration
  context:
    # Static file paths
    artifacts:
      - path: "src/main.go"
        label: "Main application code"  # Optional label for context
      - path: "README.md"
    
    # Dynamic artifacts from previous activity outputs
    artifacts_from_output: "file_generation.output_files"  # Reference to previous activity
    
    # Glob patterns for file matching
    artifacts_glob:
      - pattern: "src/**/*.go"
        exclude: ["**/test/**", "**/vendor/**"]  # Optional exclusions
      - pattern: "docs/*.md"
    
    # Directory listing
    artifacts_directory:
      path: "config/"
      recursive: true
      extensions: [".yaml", ".json"]
    
    # Combined resolution with automatic deduplication
    artifacts_resolution:
      sources:
        - type: "activity_output"
          activity: "code_generator"
          field: "generated_files"
        - type: "glob"
          patterns: ["*.yaml", "*.json"]
        - type: "directory"
          path: "output/"
          recursive: true
          filter: "*.txt"
        - type: "static"
          paths: ["CHANGELOG.md", "VERSION"]
  
  persona:
    role: Code Reviewer
    # ... rest of persona config
```

#### Artifact Resolution Methods

1. **Static Paths** - Hardcoded file paths
   ```yaml
   artifacts:
     - path: "config.yaml"
     - path: "src/main.go"
       label: "Main entry point"  # Optional descriptive label
   ```

2. **Activity Output Reference** - Files from previous activity
   ```yaml
   artifacts_from_output: "previous_activity.output_field"
   ```

3. **Glob Patterns** - Pattern matching with optional exclusions
   ```yaml
   artifacts_glob:
     - pattern: "**/*.go"
       exclude: ["**/test/**", "**/backup/**"]
   ```

4. **Directory Listing** - All files in directory
   ```yaml
   artifacts_directory:
     path: "output/"
     recursive: true
     extensions: [".json", ".yaml"]
   ```

5. **Combined Resolution** - Multiple sources with automatic deduplication
   ```yaml
   artifacts_resolution:
     sources:
       - type: "activity_output"
         activity: "scanner"
         field: "found_files"
       - type: "glob"
         patterns: ["src/**/*.py"]
       - type: "static"
         paths: ["requirements.txt"]
   ```

#### Provider-Adaptive File Injection

The LLM activity automatically selects the optimal file injection method based on the provider:

**OpenAI Models:**
```json
{
  "model": "gpt-4-vision-preview",
  "messages": [
    {
      "role": "system",
      "content": "[Persona configuration if applicable]"
    },
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "{input.prompt}"
        },
        {
          "type": "image_url",
          "image_url": {
            "url": "data:image/png;base64,{base64_image_data}"
          }
        },
        {
          "type": "text",
          "text": "File: src/main.go\n{file_content}"
        }
      ]
    }
  ]
}
```

**Anthropic Claude:**
```json
{
  "model": "claude-3-opus",
  "messages": [
    {
      "role": "system",
      "content": "[Persona configuration if applicable]"
    },
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "{input.prompt}"
        },
        {
          "type": "image",
          "source": {
            "type": "base64",
            "media_type": "image/png",
            "data": "{base64_image_data}"
          }
        },
        {
          "type": "document",
          "source": {
            "type": "base64",
            "media_type": "application/pdf",
            "data": "{base64_pdf_data}"
          }
        }
      ]
    }
  ]
}
```

**Google Gemini:**
```json
{
  "model": "gemini-pro-vision",
  "contents": [
    {
      "role": "user",
      "parts": [
        {
          "text": "{input.prompt}"
        },
        {
          "fileData": {
            "mimeType": "image/png",
            "fileUri": "gs://bucket/image.png"
          }
        },
        {
          "text": "File: src/main.go\n{file_content}"
        }
      ]
    }
  ],
  "systemInstruction": {
    "parts": [
      {
        "text": "[Persona configuration if applicable]"
      }
    ]
  }
}
```

**Fallback Text Injection (for unsupported providers):**
```
System: [Persona configuration if applicable]

User: {input.prompt}

--- File Context ---
File: src/main.go
```
{file_content}
```

File: README.md
```
{file_content}
```
```

#### Example Configurations

**Code Review Persona with File Context:**
```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  
  context:
    artifacts_glob:
      - pattern: "src/**/*.go"
        exclude: ["**/test/**"]
    artifacts:
      - path: "go.mod"
        label: "Dependencies"
  
  persona:
    role: Senior Go Developer and Code Reviewer
    capabilities:
      - code_analysis
      - security_review
      - performance_optimization
      - best_practices_enforcement
    goals:
      - Identify potential bugs and security issues
      - Suggest performance improvements
      - Ensure code follows Go best practices
      - Provide actionable feedback
    constraints:
      - Focus on practical, implementable suggestions
      - Prioritize critical issues over style preferences
      - Consider backward compatibility
```

**Documentation Generator with Dynamic Files:**
```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  
  context:
    # Get files generated by previous activity
    artifacts_from_output: "code_generator.created_files"
    # Also include existing docs for context
    artifacts_glob:
      - pattern: "docs/**/*.md"
  
  persona:
    role: Technical Documentation Writer
    capabilities:
      - api_documentation
      - code_examples
      - markdown_formatting
    goals:
      - Create clear, comprehensive documentation
      - Include practical examples
      - Maintain consistent style
    constraints:
      - Follow project documentation standards
      - Keep examples simple and focused
```

**Data Analysis with Multiple File Sources:**
```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  
  context:
    artifacts_resolution:
      sources:
        - type: "glob"
          patterns: ["data/*.csv", "data/*.json"]
        - type: "activity_output"
          activity: "data_processor"
          field: "processed_files"
        - type: "static"
          paths: ["schema.json", "data_dictionary.md"]
  
  persona:
    role: Data Analyst
    capabilities:
      - data_analysis
      - pattern_recognition
      - statistical_analysis
    goals:
      - Identify trends and anomalies
      - Provide data-driven insights
      - Suggest data quality improvements
```

#### File Size and Limits

```yaml
type: llm
config:
  mode: persona
  model: gpt-4
  
  context:
    artifacts:
      - path: "large_file.txt"
    
    # File handling configuration
    file_limits:
      max_file_size: 1048576  # 1MB per file
      max_total_size: 5242880  # 5MB total
      max_file_count: 20       # Maximum 20 files
      truncate_large_files: true  # Truncate instead of failing
      truncation_message: "... [truncated - file exceeds size limit] ..."
```

#### File Type Detection and Handling

```yaml
type: llm
config:
  mode: persona
  model: gpt-4-vision-preview  # or claude-3-opus, gemini-pro-vision
  
  context:
    artifacts:
      - path: "architecture.png"
        type: "image"  # Optional: auto-detected from extension
      - path: "report.pdf"
        type: "document"
      - path: "src/main.go"
        type: "text"  # Default type
    
    # Provider-specific handling
    file_handling:
      provider_mode: "native"  # Options: "native" (default) | "text_fallback" | "hybrid"
      image_handling: "base64"  # Options: "base64" | "url" | "upload"
      pdf_handling: "native"     # Options: "native" | "text_extraction" | "ocr"
      max_image_size: 20971520   # 20MB limit for images
      optimize_images: true      # Resize/compress if needed
```

#### Provider Capability Matrix

| Provider | Text Files | Images | PDFs | Audio | Video | Native API |
|----------|------------|--------|------|-------|-------|------------|
| OpenAI GPT-4 | ✅ | ❌ | ❌ | ❌ | ❌ | Assistants API |
| OpenAI GPT-4V | ✅ | ✅ | ❌ | ❌ | ❌ | Vision API |
| Anthropic Claude 3 | ✅ | ✅ | ✅ | ❌ | ❌ | Messages API |
| Google Gemini Pro | ✅ | ✅ | ✅ | ✅ | ✅ | Files API |
| Local Models | ✅ | ⚠️ | ⚠️ | ❌ | ❌ | Text injection |

⚠️ = Requires preprocessing/conversion

#### Runtime File Resolution and Provider Adaptation

```go
type FileHandler interface {
    PrepareFiles(ctx context.Context, artifacts []Artifact, provider string) ([]PreparedFile, error)
    SupportsNativeFiles(provider string) bool
    GetSupportedTypes(provider string) []string
}

type PreparedFile struct {
    Original  Artifact
    Type      FileType      // text, image, document, audio, video
    Format    string        // How to send: base64, url, upload, text
    Content   interface{}   // Prepared content based on format
    Metadata  map[string]interface{}
}

func (a *LLMActivity) resolveArtifacts(ctx context.Context, input ActivityInput) ([]Artifact, error) {
    var artifacts []Artifact
    config := a.Config.Context
    
    // Static artifacts
    for _, artifact := range config.Artifacts {
        artifacts = append(artifacts, artifact)
    }
    
    // Get artifacts from previous activity output
    if ref := config.ArtifactsFromOutput; ref != "" {
        parts := strings.Split(ref, ".")
        if len(parts) == 2 {
            if output, ok := input.PreviousOutputs[parts[0]]; ok {
                if files, ok := output[parts[1]].([]string); ok {
                    for _, file := range files {
                        artifacts = append(artifacts, Artifact{
                            Path: file,
                        })
                    }
                }
            }
        }
    }
    
    // Apply glob patterns
    for _, glob := range config.ArtifactsGlob {
        matches, _ := filepath.Glob(glob.Pattern)
        for _, match := range matches {
            // Check exclusions
            excluded := false
            for _, exclude := range glob.Exclude {
                if matched, _ := filepath.Match(exclude, match); matched {
                    excluded = true
                    break
                }
            }
            if !excluded {
                artifacts = append(artifacts, Artifact{
                    Path: match,
                })
            }
        }
    }
    
    // Directory listing
    if dir := config.ArtifactsDirectory; dir != nil {
        // Walk directory and collect matching files
        filepath.Walk(dir.Path, func(path string, info os.FileInfo, err error) error {
            if err != nil {
                return nil // Skip errors
            }
            if !dir.Recursive && filepath.Dir(path) != dir.Path {
                return filepath.SkipDir
            }
            for _, ext := range dir.Extensions {
                if strings.HasSuffix(path, ext) {
                    artifacts = append(artifacts, Artifact{
                        Path: path,
                    })
                    break
                }
            }
            return nil
        })
    }
    
    // Combined resolution
    if res := config.ArtifactsResolution; res != nil {
        for _, source := range res.Sources {
            // Process each source type
            // ... (similar to above methods)
        }
    }
    
    // Always deduplicate artifacts
    artifacts = deduplicateArtifacts(artifacts)
    
    // Apply file limits
    artifacts = applyFileLimits(artifacts, config.FileLimits)
    
    return artifacts, nil
}

// Provider-specific message building
func (a *LLMActivity) buildProviderMessage(prompt string, artifacts []Artifact, provider string) (interface{}, error) {
    handler := a.getFileHandler(provider)
    preparedFiles, err := handler.PrepareFiles(context.Background(), artifacts, provider)
    if err != nil {
        return nil, err
    }
    
    switch provider {
    case "openai":
        return a.buildOpenAIMessage(prompt, preparedFiles)
    case "anthropic":
        return a.buildAnthropicMessage(prompt, preparedFiles)
    case "google":
        return a.buildGeminiMessage(prompt, preparedFiles)
    default:
        // Fallback to text injection
        return a.buildTextFallbackMessage(prompt, artifacts)
    }
}

func (a *LLMActivity) buildOpenAIMessage(prompt string, files []PreparedFile) (interface{}, error) {
    content := []interface{}{
        map[string]string{"type": "text", "text": prompt},
    }
    
    for _, file := range files {
        switch file.Type {
        case FileTypeImage:
            content = append(content, map[string]interface{}{
                "type": "image_url",
                "image_url": map[string]string{
                    "url": file.Content.(string), // base64 or URL
                },
            })
        case FileTypeText, FileTypeDocument:
            // OpenAI doesn't support native PDFs, extract text
            content = append(content, map[string]string{
                "type": "text",
                "text": fmt.Sprintf("File: %s\n%s", file.Original.Path, file.Content),
            })
        }
    }
    
    return map[string]interface{}{
        "role": "user",
        "content": content,
    }, nil
}

func (a *LLMActivity) buildAnthropicMessage(prompt string, files []PreparedFile) (interface{}, error) {
    content := []interface{}{
        map[string]string{"type": "text", "text": prompt},
    }
    
    for _, file := range files {
        switch file.Type {
        case FileTypeImage:
            content = append(content, map[string]interface{}{
                "type": "image",
                "source": map[string]string{
                    "type": "base64",
                    "media_type": file.Metadata["mime_type"].(string),
                    "data": file.Content.(string),
                },
            })
        case FileTypeDocument:
            // Claude supports native PDF parsing
            if file.Metadata["mime_type"] == "application/pdf" {
                content = append(content, map[string]interface{}{
                    "type": "document",
                    "source": map[string]string{
                        "type": "base64",
                        "media_type": "application/pdf",
                        "data": file.Content.(string),
                    },
                })
            } else {
                // Fallback to text
                content = append(content, map[string]string{
                    "type": "text",
                    "text": fmt.Sprintf("File: %s\n%s", file.Original.Path, file.Content),
                })
            }
        case FileTypeText:
            content = append(content, map[string]string{
                "type": "text",
                "text": fmt.Sprintf("File: %s\n%s", file.Original.Path, file.Content),
            })
        }
    }
    
    return map[string]interface{}{
        "role": "user",
        "content": content,
    }, nil
}

func (a *LLMActivity) buildGeminiMessage(prompt string, files []PreparedFile) (interface{}, error) {
    parts := []interface{}{
        map[string]string{"text": prompt},
    }
    
    for _, file := range files {
        switch file.Format {
        case "upload":
            // Gemini uses file URIs after upload
            parts = append(parts, map[string]interface{}{
                "fileData": map[string]string{
                    "mimeType": file.Metadata["mime_type"].(string),
                    "fileUri": file.Content.(string),
                },
            })
        case "text":
            parts = append(parts, map[string]string{
                "text": fmt.Sprintf("File: %s\n%s", file.Original.Path, file.Content),
            })
        }
    }
    
    return map[string]interface{}{
        "role": "user",
        "parts": parts,
    }, nil
}

// Fallback for providers without native file support
func (a *LLMActivity) buildTextFallbackMessage(prompt string, artifacts []Artifact) (string, error) {
    var builder strings.Builder
    builder.WriteString(prompt)
    
    if len(artifacts) > 0 {
        builder.WriteString("\n\n--- File Context ---\n")
        
        for _, artifact := range artifacts {
            content, err := a.readFileContent(artifact.Path)
            if err != nil {
                continue
            }
            
            if artifact.Label != "" {
                builder.WriteString(fmt.Sprintf("\nFile: %s (%s)\n", artifact.Path, artifact.Label))
            } else {
                builder.WriteString(fmt.Sprintf("\nFile: %s\n", artifact.Path))
            }
            
            builder.WriteString("```\n")
            builder.WriteString(content)
            builder.WriteString("\n```\n")
        }
    }
    
    return builder.String(), nil
}
```

### 4. Backward Compatibility
- Simple mode must maintain current behavior
- Existing configurations without `mode` field default to simple mode
- No breaking changes to existing activities

### 5. Validation Rules
- `role`: Required string, non-empty
- `capabilities`: Optional array of strings
- `goals`: Optional array of strings  
- `constraints`: Optional array of strings
- `context`: Optional array of strings
- All arrays must contain at least one item if specified

### 6. Error Handling
- Invalid mode value: Default to simple mode with warning
- Missing role in persona mode: Return validation error
- Empty arrays: Ignore the field
- Malformed persona config: Fall back to simple mode with error message
- Unsupported file types for provider: Auto-convert or fallback to text
- File upload failures: Retry with fallback methods
- Provider API errors: Graceful degradation to text injection

## Testing Requirements

### Unit Tests
1. Simple mode preserves existing behavior
2. Persona mode correctly constructs prompts
3. Configuration validation works correctly
4. Default values are applied properly
5. Error cases are handled gracefully
6. Provider detection works correctly
7. File type detection and conversion works
8. Native API calls are properly formatted per provider

### Integration Tests
1. Simple mode activities execute unchanged
2. Persona mode activities apply persona configuration
3. Mode switching works correctly
4. Invalid configurations fall back safely
5. Files are correctly included using native provider APIs
6. Fallback mechanisms work when native APIs fail
7. Different file types are handled appropriately per provider

## Migration Path

### Phase 1: Implementation
- Add mode detection logic
- Implement persona prompt construction
- Maintain backward compatibility

### Phase 2: Documentation
- Update activity documentation
- Add persona mode examples
- Create best practices guide

### Phase 3: Enhancement
- Add persona templates library
- Implement persona validation rules
- Add dynamic capability detection

## Success Criteria
- [ ] Simple mode works identically to current implementation
- [ ] Persona mode successfully applies all configuration fields
- [ ] All existing activities continue to function
- [ ] Configuration validation prevents invalid personas
- [ ] Clear error messages for configuration issues
- [ ] Comprehensive test coverage for both modes
- [ ] Files are included using native provider APIs when available
- [ ] Automatic fallback to text injection when native APIs unavailable
- [ ] Provider-specific formatting is correctly applied
- [ ] File type detection and handling works across all providers