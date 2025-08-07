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

#### Clean Architecture Summary

The file handling system follows clean architecture principles:

1. **Activity Layer (Central)**: 
   - Resolves all file paths from configuration
   - Reads file contents into memory
   - Creates provider-agnostic `ResolvedFile` objects
   - Handles deduplication and size limits

2. **Provider Facade (Interface)**:
   - Clean `AddFileContext()` method for adding files
   - Provider decides internally how to handle files
   - Encapsulates all provider-specific logic

3. **Provider Implementation (Internal)**:
   - Each provider handles files according to its native capabilities
   - OpenAI: Images via Vision API, text injection for documents
   - Anthropic: Native image and PDF support
   - Gemini: Upload to Files API for all types
   - Fallback providers: Simple text injection

This design ensures:
- **Single Responsibility**: File resolution is separate from provider-specific handling
- **Open/Closed**: Easy to add new providers without changing core logic
- **Interface Segregation**: Clean, minimal provider interface
- **Dependency Inversion**: Activity depends on abstraction, not concrete providers

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

#### Architecture: Separation of Concerns

The file handling system follows a clean separation of concerns:

1. **Activity Layer**: Resolves file paths and collects artifacts
2. **Provider Facade**: Abstract interface for all LLM providers
3. **Provider Implementation**: Provider-specific file handling logic

```go
// LLM Provider Facade - Clean interface for all providers
type LLMProvider interface {
    // Core methods
    SendMessage(ctx context.Context, messages []Message, config Config) (Response, error)
    
    // File context methods
    AddFileContext(ctx context.Context, files []ResolvedFile) error
    AddDirectoryContext(ctx context.Context, path string, recursive bool) error
    ClearContext() error
    
    // Capabilities
    SupportsFileType(fileType string) bool
    GetMaxFileSize() int64
    GetSupportedMimeTypes() []string
}

// Resolved file from activity layer - provider agnostic
type ResolvedFile struct {
    Path      string
    Label     string  // Optional descriptive label
    Content   []byte  // Raw file content
    MimeType  string  // Detected MIME type
    Size      int64
}

// Activity layer - centralizes all file resolution logic
func (a *LLMActivity) resolveArtifacts(ctx context.Context, input ActivityInput) ([]ResolvedFile, error) {
    paths := []string{}
    labels := map[string]string{}
    config := a.Config.Context
    
    // Static artifacts
    for _, artifact := range config.Artifacts {
        paths = append(paths, artifact.Path)
        if artifact.Label != "" {
            labels[artifact.Path] = artifact.Label
        }
    }
    
    // Get artifacts from previous activity output
    if ref := config.ArtifactsFromOutput; ref != "" {
        parts := strings.Split(ref, ".")
        if len(parts) == 2 {
            if output, ok := input.PreviousOutputs[parts[0]]; ok {
                if files, ok := output[parts[1]].([]string); ok {
                    paths = append(paths, files...)
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
                paths = append(paths, match)
            }
        }
    }
    
    // Directory listing
    if dir := config.ArtifactsDirectory; dir != nil {
        dirPaths, err := a.listDirectory(dir.Path, dir.Recursive, dir.Extensions)
        if err == nil {
            paths = append(paths, dirPaths...)
        }
    }
    
    // Combined resolution
    if res := config.ArtifactsResolution; res != nil {
        for _, source := range res.Sources {
            sourcePaths := a.resolveSource(source, input)
            paths = append(paths, sourcePaths...)
        }
    }
    
    // Deduplicate paths
    paths = deduplicatePaths(paths)
    
    // Read files and create ResolvedFile objects
    resolvedFiles := []ResolvedFile{}
    for _, path := range paths {
        content, err := os.ReadFile(path)
        if err != nil {
            continue // Skip files that can't be read
        }
        
        mimeType := detectMimeType(path, content)
        
        resolved := ResolvedFile{
            Path:     path,
            Label:    labels[path],
            Content:  content,
            MimeType: mimeType,
            Size:     int64(len(content)),
        }
        
        // Apply size limits
        if config.FileLimits != nil {
            if resolved.Size > config.FileLimits.MaxFileSize {
                if config.FileLimits.TruncateLargeFiles {
                    resolved.Content = truncateContent(resolved.Content, config.FileLimits.MaxFileSize)
                } else {
                    continue // Skip file
                }
            }
        }
        
        resolvedFiles = append(resolvedFiles, resolved)
    }
    
    // Apply total limits
    if config.FileLimits != nil {
        resolvedFiles = applyTotalLimits(resolvedFiles, config.FileLimits)
    }
    
    return resolvedFiles, nil
}

// Main activity execution
func (a *LLMActivity) Execute(ctx context.Context, input ActivityInput) (ActivityOutput, error) {
    // Step 1: Resolve all files centrally
    resolvedFiles, err := a.resolveArtifacts(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve artifacts: %w", err)
    }
    
    // Step 2: Get the appropriate provider
    provider := a.getProvider()
    
    // Step 3: Add file context to provider (provider handles internally)
    if len(resolvedFiles) > 0 {
        if err := provider.AddFileContext(ctx, resolvedFiles); err != nil {
            // Log warning but continue - provider should handle gracefully
            log.Printf("Warning: failed to add file context: %v", err)
        }
    }
    
    // Step 4: Build messages (persona or simple mode)
    messages := a.buildMessages(input)
    
    // Step 5: Send to provider
    response, err := provider.SendMessage(ctx, messages, a.Config)
    if err != nil {
        return nil, err
    }
    
    // Step 6: Clear context if needed
    defer provider.ClearContext()
    
    return ActivityOutput{
        "response": response.Content,
        "usage": response.Usage,
    }, nil
}

// Provider Implementations - Each handles files internally

// OpenAI Provider Implementation
type OpenAIProvider struct {
    client      *openai.Client
    fileContext []ResolvedFile
}

func (p *OpenAIProvider) AddFileContext(ctx context.Context, files []ResolvedFile) error {
    // Validate files are supported
    for _, file := range files {
        if !p.SupportsFileType(file.MimeType) {
            // Convert or skip based on configuration
            log.Printf("OpenAI: unsupported file type %s, will inject as text", file.MimeType)
        }
    }
    p.fileContext = files
    return nil
}

func (p *OpenAIProvider) SendMessage(ctx context.Context, messages []Message, config Config) (Response, error) {
    // Build OpenAI-specific message format internally
    openAIMessages := []openai.Message{}
    
    for _, msg := range messages {
        if msg.Role == "user" && len(p.fileContext) > 0 {
            // Add files using OpenAI's format
            content := []interface{}{
                map[string]string{"type": "text", "text": msg.Content},
            }
            
            for _, file := range p.fileContext {
                if strings.HasPrefix(file.MimeType, "image/") {
                    // Use vision API for images
                    content = append(content, map[string]interface{}{
                        "type": "image_url",
                        "image_url": map[string]string{
                            "url": fmt.Sprintf("data:%s;base64,%s", 
                                file.MimeType, 
                                base64.StdEncoding.EncodeToString(file.Content)),
                        },
                    })
                } else {
                    // Inject text files directly
                    label := file.Label
                    if label == "" {
                        label = file.Path
                    }
                    content = append(content, map[string]string{
                        "type": "text",
                        "text": fmt.Sprintf("\nFile: %s\n```\n%s\n```", label, string(file.Content)),
                    })
                }
            }
            
            openAIMessages = append(openAIMessages, openai.Message{
                Role:    msg.Role,
                Content: content,
            })
        } else {
            openAIMessages = append(openAIMessages, openai.Message{
                Role:    msg.Role,
                Content: msg.Content,
            })
        }
    }
    
    // Send to OpenAI API
    return p.client.Send(ctx, openAIMessages)
}

func (p *OpenAIProvider) SupportsFileType(mimeType string) bool {
    return strings.HasPrefix(mimeType, "text/") || 
           strings.HasPrefix(mimeType, "image/")
}

// Anthropic Provider Implementation
type AnthropicProvider struct {
    client      *anthropic.Client
    fileContext []ResolvedFile
}

func (p *AnthropicProvider) AddFileContext(ctx context.Context, files []ResolvedFile) error {
    p.fileContext = files
    return nil
}

func (p *AnthropicProvider) SendMessage(ctx context.Context, messages []Message, config Config) (Response, error) {
    // Build Anthropic-specific message format internally
    anthropicMessages := []anthropic.Message{}
    
    for _, msg := range messages {
        if msg.Role == "user" && len(p.fileContext) > 0 {
            // Build content blocks for Anthropic
            content := []interface{}{
                map[string]string{"type": "text", "text": msg.Content},
            }
            
            for _, file := range p.fileContext {
                if strings.HasPrefix(file.MimeType, "image/") {
                    content = append(content, map[string]interface{}{
                        "type": "image",
                        "source": map[string]interface{}{
                            "type": "base64",
                            "media_type": file.MimeType,
                            "data": base64.StdEncoding.EncodeToString(file.Content),
                        },
                    })
                } else if file.MimeType == "application/pdf" {
                    // Claude native PDF support
                    content = append(content, map[string]interface{}{
                        "type": "document",
                        "source": map[string]interface{}{
                            "type": "base64",
                            "media_type": "application/pdf",
                            "data": base64.StdEncoding.EncodeToString(file.Content),
                        },
                    })
                } else {
                    // Text injection
                    label := file.Label
                    if label == "" {
                        label = file.Path
                    }
                    content = append(content, map[string]string{
                        "type": "text",
                        "text": fmt.Sprintf("\nFile: %s\n```\n%s\n```", label, string(file.Content)),
                    })
                }
            }
            
            anthropicMessages = append(anthropicMessages, anthropic.Message{
                Role:    msg.Role,
                Content: content,
            })
        } else {
            anthropicMessages = append(anthropicMessages, anthropic.Message{
                Role:    msg.Role,
                Content: msg.Content,
            })
        }
    }
    
    // Send to Anthropic API
    return p.client.Send(ctx, anthropicMessages)
}

func (p *AnthropicProvider) SupportsFileType(mimeType string) bool {
    return strings.HasPrefix(mimeType, "text/") || 
           strings.HasPrefix(mimeType, "image/") ||
           mimeType == "application/pdf"
}

// Google Gemini Provider Implementation
type GeminiProvider struct {
    client      *gemini.Client
    fileContext []ResolvedFile
}

func (p *GeminiProvider) AddFileContext(ctx context.Context, files []ResolvedFile) error {
    // Upload files to Google's Files API
    for i, file := range files {
        uploadedURI, err := p.uploadFile(ctx, file)
        if err != nil {
            log.Printf("Gemini: failed to upload file %s: %v", file.Path, err)
            continue
        }
        // Store URI for later use
        files[i].Metadata = map[string]interface{}{
            "uploaded_uri": uploadedURI,
        }
    }
    p.fileContext = files
    return nil
}

func (p *GeminiProvider) SendMessage(ctx context.Context, messages []Message, config Config) (Response, error) {
    // Build Gemini-specific message format
    parts := []interface{}{}
    
    for _, msg := range messages {
        if msg.Role == "user" {
            parts = append(parts, map[string]string{"text": msg.Content})
            
            // Add file parts if context exists
            for _, file := range p.fileContext {
                if uri, ok := file.Metadata["uploaded_uri"].(string); ok {
                    parts = append(parts, map[string]interface{}{
                        "fileData": map[string]string{
                            "mimeType": file.MimeType,
                            "fileUri": uri,
                        },
                    })
                } else {
                    // Fallback to text injection if upload failed
                    label := file.Label
                    if label == "" {
                        label = file.Path
                    }
                    parts = append(parts, map[string]string{
                        "text": fmt.Sprintf("\nFile: %s\n```\n%s\n```", label, string(file.Content)),
                    })
                }
            }
        }
    }
    
    // Send to Gemini API
    return p.client.Send(ctx, parts)
}

func (p *GeminiProvider) SupportsFileType(mimeType string) bool {
    // Gemini supports many file types
    return true
}

// Factory function to get appropriate provider
func getProvider(model string) LLMProvider {
    switch {
    case strings.HasPrefix(model, "gpt"):
        return &OpenAIProvider{client: openai.NewClient()}
    case strings.HasPrefix(model, "claude"):
        return &AnthropicProvider{client: anthropic.NewClient()}
    case strings.HasPrefix(model, "gemini"):
        return &GeminiProvider{client: gemini.NewClient()}
    default:
        return &TextFallbackProvider{}
    }
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