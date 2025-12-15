# API Generator Op Specification

## Overview
A Go RegisterableActivity that generates language-specific API documentation from source code directories using tokei for language detection and native documentation tools.

## Package Structure

```
server/ops/pkg/apigen/
├── activity.go         # Main activity implementation
├── activity_test.go    # Unit tests
├── tokei.go           # Tokei integration
├── generators.go      # Language-specific generators
└── types.go          # Type definitions
```

## Type Definitions

```go
// Package apigen provides API documentation generation
package apigen

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/colony-2/colony2/server/ops/pkg/types"
)

// APIGeneratorConfig provides configuration for the API generator activity
type APIGeneratorConfig struct {
    // Path to tokei binary (defaults to "tokei" in PATH)
    TokeiPath string `json:"tokei_path,omitempty" yaml:"tokei_path,omitempty"`
    
    // Default output directory name
    DefaultOutputDir string `json:"default_output_dir,omitempty" yaml:"default_output_dir,omitempty"`
    
    // Tool timeouts
    ToolTimeout time.Duration `json:"tool_timeout,omitempty" yaml:"tool_timeout,omitempty"`
}

// APIGeneratorInput defines the input parameters
type APIGeneratorInput struct {
    // Required: Source directory to analyze
    SourceDir string `json:"source_dir" validate:"required,dir"`
    
    // Optional: Output directory (defaults to source_dir/colony2.api)
    OutputDir string `json:"output_dir,omitempty"`
    
    // Optional: Documentation detail level
    DetailLevel string `json:"detail_level,omitempty"` // minimal, standard, detailed
    
    // Optional: Include private/internal APIs
    IncludePrivate bool `json:"include_private,omitempty"`
    
    // Optional: Output formats to generate
    OutputFormats []string `json:"output_formats,omitempty"` // json, text, html
    
    // Optional: Language override (if auto-detection should be skipped)
    Language string `json:"language,omitempty"`
}

// APIGeneratorOutput defines the output structure
type APIGeneratorOutput struct {
    // Primary language detected
    PrimaryLanguage string `json:"primary_language"`
    
    // Language statistics from tokei
    LanguageStats map[string]LanguageStats `json:"language_stats"`
    
    // Generated documentation file paths
    DocumentationPaths map[string]string `json:"documentation_paths"`
    
    // Extracted API summary
    APISummary APISummary `json:"api_summary"`
    
    // Tools that were used
    ToolsUsed []string `json:"tools_used"`
    
    // Any errors encountered (non-fatal)
    Errors []string `json:"errors,omitempty"`
    
    // Any warnings
    Warnings []string `json:"warnings,omitempty"`
}

// LanguageStats contains statistics for a programming language
type LanguageStats struct {
    Files       int     `json:"files"`
    Lines       int     `json:"lines"`
    Code        int     `json:"code"`
    Comments    int     `json:"comments"`
    Blanks      int     `json:"blanks"`
    Percentage  float64 `json:"percentage"`
}

// APISummary contains the extracted API information
type APISummary struct {
    Packages    []PackageInfo    `json:"packages,omitempty"`
    Exports     []ExportInfo     `json:"exports,omitempty"`
    Functions   []FunctionInfo   `json:"functions,omitempty"`
    Types       []TypeInfo       `json:"types,omitempty"`
    Constants   []ConstantInfo   `json:"constants,omitempty"`
}

// PackageInfo describes a package/module
type PackageInfo struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    Description string `json:"description,omitempty"`
}

// ExportInfo describes an exported symbol
type ExportInfo struct {
    Name        string `json:"name"`
    Kind        string `json:"kind"` // function, type, const, var
    Package     string `json:"package,omitempty"`
    File        string `json:"file"`
    Line        int    `json:"line,omitempty"`
    Signature   string `json:"signature,omitempty"`
    Description string `json:"description,omitempty"`
}

// FunctionInfo describes a function/method
type FunctionInfo struct {
    Name        string   `json:"name"`
    Package     string   `json:"package,omitempty"`
    Signature   string   `json:"signature"`
    Parameters  []string `json:"parameters,omitempty"`
    Returns     []string `json:"returns,omitempty"`
    Description string   `json:"description,omitempty"`
}

// TypeInfo describes a type definition
type TypeInfo struct {
    Name        string   `json:"name"`
    Kind        string   `json:"kind"` // struct, interface, enum, class
    Package     string   `json:"package,omitempty"`
    Fields      []string `json:"fields,omitempty"`
    Methods     []string `json:"methods,omitempty"`
    Description string   `json:"description,omitempty"`
}

// ConstantInfo describes a constant
type ConstantInfo struct {
    Name        string `json:"name"`
    Type        string `json:"type,omitempty"`
    Value       string `json:"value,omitempty"`
    Package     string `json:"package,omitempty"`
    Description string `json:"description,omitempty"`
}
```

## Activity Implementation

### Main Activity (activity.go)

```go
package apigen

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "time"
    
    "github.com/colony-2/colony2/server/ops/pkg/types"
)

// APIGeneratorActivity implements the API documentation generator
type APIGeneratorActivity struct {
    config *APIGeneratorConfig
}

// Ensure we implement the interface
var _ types.RegisterableActivity[APIGeneratorConfig, APIGeneratorInput, APIGeneratorOutput] = (*APIGeneratorActivity)(nil)

// NewAPIGeneratorActivity creates a new API generator activity
func NewAPIGeneratorActivity() types.RegisterableActivity[APIGeneratorConfig, APIGeneratorInput, APIGeneratorOutput] {
    return &APIGeneratorActivity{}
}

// GetMetadata returns activity metadata
func (a *APIGeneratorActivity) GetMetadata() types.ActivityMetadata {
    return types.ActivityMetadata{
        Type:           "api_generator",
        Name:           "API Documentation Generator",
        Description:    "Generates API documentation using language-specific tools",
        Version:        "1.0.0",
        DefaultTimeout: 5 * time.Minute,
        RetryPolicy: &types.RetryPolicy{
            MaximumAttempts:    2,
            InitialInterval:    5 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    30 * time.Second,
        },
    }
}

// Execute runs the API generator activity
func (a *APIGeneratorActivity) Execute(
    ctx context.Context,
    config APIGeneratorConfig,
    input APIGeneratorInput,
) (APIGeneratorOutput, error) {
    a.config = &config
    
    // Set defaults
    if config.TokeiPath == "" {
        config.TokeiPath = "tokei"
    }
    if config.DefaultOutputDir == "" {
        config.DefaultOutputDir = "colony2.api"
    }
    if config.ToolTimeout == 0 {
        config.ToolTimeout = 60 * time.Second
    }
    
    // Validate input
    if _, err := os.Stat(input.SourceDir); err != nil {
        return APIGeneratorOutput{}, fmt.Errorf("source directory not found: %w", err)
    }
    
    // Set output directory
    if input.OutputDir == "" {
        input.OutputDir = filepath.Join(input.SourceDir, config.DefaultOutputDir)
    }
    
    // Create output directory
    if err := os.MkdirAll(input.OutputDir, 0755); err != nil {
        return APIGeneratorOutput{}, fmt.Errorf("failed to create output directory: %w", err)
    }
    
    output := APIGeneratorOutput{
        DocumentationPaths: make(map[string]string),
        ToolsUsed:         []string{},
        Errors:            []string{},
        Warnings:          []string{},
    }
    
    // Step 1: Detect languages using tokei
    langStats, err := a.detectLanguages(ctx, input.SourceDir)
    if err != nil {
        return output, fmt.Errorf("language detection failed: %w", err)
    }
    output.LanguageStats = langStats
    output.ToolsUsed = append(output.ToolsUsed, "tokei")
    
    // Determine primary language
    primaryLang := a.determinePrimaryLanguage(langStats, input.Language)
    output.PrimaryLanguage = primaryLang
    
    // Step 2: Generate API documentation based on language
    apiSummary, docPaths, toolsUsed, errors := a.generateAPIDocs(
        ctx, 
        primaryLang, 
        input.SourceDir, 
        input.OutputDir,
        input.DetailLevel,
        input.IncludePrivate,
        input.OutputFormats,
    )
    
    output.APISummary = apiSummary
    output.DocumentationPaths = docPaths
    output.ToolsUsed = append(output.ToolsUsed, toolsUsed...)
    output.Errors = append(output.Errors, errors...)
    
    return output, nil
}

// determinePrimaryLanguage determines the primary language from stats
func (a *APIGeneratorActivity) determinePrimaryLanguage(stats map[string]LanguageStats, override string) string {
    if override != "" {
        return override
    }
    
    // Find language with most code lines
    var primaryLang string
    var maxCode int
    
    for lang, stat := range stats {
        if stat.Code > maxCode {
            maxCode = stat.Code
            primaryLang = lang
        }
    }
    
    return primaryLang
}
```

### Tokei Integration (tokei.go)

```go
package apigen

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "time"
)

// TokeiOutput represents the JSON output from tokei
type TokeiOutput map[string]TokeiLanguage

// TokeiLanguage represents language statistics from tokei
type TokeiLanguage struct {
    Blanks   int              `json:"blanks"`
    Code     int              `json:"code"`
    Comments int              `json:"comments"`
    Lines    int              `json:"lines"`
    Stats    []TokeiFileStat  `json:"stats"`
}

// TokeiFileStat represents per-file statistics
type TokeiFileStat struct {
    Blanks   int    `json:"blanks"`
    Code     int    `json:"code"`
    Comments int    `json:"comments"`
    Lines    int    `json:"lines"`
    Name     string `json:"name"`
}

// detectLanguages uses tokei to detect languages and gather statistics
func (a *APIGeneratorActivity) detectLanguages(ctx context.Context, sourceDir string) (map[string]LanguageStats, error) {
    ctx, cancel := context.WithTimeout(ctx, a.config.ToolTimeout)
    defer cancel()
    
    // Run tokei with JSON output
    cmd := exec.CommandContext(ctx, a.config.TokeiPath, sourceDir, "--output", "json")
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("tokei execution failed: %w", err)
    }
    
    // Parse tokei output
    var tokeiOutput TokeiOutput
    if err := json.Unmarshal(output, &tokeiOutput); err != nil {
        return nil, fmt.Errorf("failed to parse tokei output: %w", err)
    }
    
    // Convert to our format
    stats := make(map[string]LanguageStats)
    totalLines := 0
    
    // First pass: calculate totals
    for _, lang := range tokeiOutput {
        totalLines += lang.Lines
    }
    
    // Second pass: convert to our format
    for langName, lang := range tokeiOutput {
        percentage := 0.0
        if totalLines > 0 {
            percentage = (float64(lang.Lines) / float64(totalLines)) * 100
        }
        
        stats[langName] = LanguageStats{
            Files:      len(lang.Stats),
            Lines:      lang.Lines,
            Code:       lang.Code,
            Comments:   lang.Comments,
            Blanks:     lang.Blanks,
            Percentage: percentage,
        }
    }
    
    return stats, nil
}
```

### Language-Specific Generators (generators.go)

```go
package apigen

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "go/ast"
    "go/parser"
    "go/token"
    "io/ioutil"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// generateAPIDocs generates API documentation based on the detected language
func (a *APIGeneratorActivity) generateAPIDocs(
    ctx context.Context,
    language string,
    sourceDir string,
    outputDir string,
    detailLevel string,
    includePrivate bool,
    outputFormats []string,
) (APISummary, map[string]string, []string, []string) {
    
    summary := APISummary{}
    paths := make(map[string]string)
    tools := []string{}
    errors := []string{}
    
    // Normalize language name from tokei
    language = strings.ToLower(language)
    
    switch language {
    case "go":
        s, p, t, e := a.generateGoAPIDocs(ctx, sourceDir, outputDir, includePrivate)
        summary = s
        paths = p
        tools = t
        errors = e
        
    case "javascript", "jsx", "typescript", "tsx":
        s, p, t, e := a.generateJSAPIDocs(ctx, sourceDir, outputDir, includePrivate)
        summary = s
        paths = p
        tools = t
        errors = e
        
    case "python":
        s, p, t, e := a.generatePythonAPIDocs(ctx, sourceDir, outputDir, includePrivate)
        summary = s
        paths = p
        tools = t
        errors = e
        
    case "java":
        s, p, t, e := a.generateJavaAPIDocs(ctx, sourceDir, outputDir, includePrivate)
        summary = s
        paths = p
        tools = t
        errors = e
        
    case "rust":
        s, p, t, e := a.generateRustAPIDocs(ctx, sourceDir, outputDir, includePrivate)
        summary = s
        paths = p
        tools = t
        errors = e
        
    default:
        // Fallback to universal tools
        s, p, t, e := a.generateFallbackAPIDocs(ctx, sourceDir, outputDir)
        summary = s
        paths = p
        tools = t
        errors = e
    }
    
    // Generate requested output formats
    if len(outputFormats) == 0 {
        outputFormats = []string{"json"}
    }
    
    for _, format := range outputFormats {
        if format == "json" {
            jsonPath := filepath.Join(outputDir, "api.json")
            data, _ := json.MarshalIndent(summary, "", "  ")
            if err := ioutil.WriteFile(jsonPath, data, 0644); err == nil {
                paths["json"] = jsonPath
            }
        }
    }
    
    return summary, paths, tools, errors
}

// generateGoAPIDocs generates API documentation for Go projects
func (a *APIGeneratorActivity) generateGoAPIDocs(
    ctx context.Context,
    sourceDir string,
    outputDir string,
    includePrivate bool,
) (APISummary, map[string]string, []string, []string) {
    
    summary := APISummary{
        Packages:  []PackageInfo{},
        Exports:   []ExportInfo{},
        Functions: []FunctionInfo{},
        Types:     []TypeInfo{},
        Constants: []ConstantInfo{},
    }
    paths := make(map[string]string)
    tools := []string{}
    errors := []string{}
    
    // Use go list to get package information
    cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
    cmd.Dir = sourceDir
    output, err := cmd.Output()
    if err == nil {
        tools = append(tools, "go list")
        
        // Parse package information
        decoder := json.NewDecoder(bytes.NewReader(output))
        for decoder.More() {
            var pkg struct {
                Name       string   `json:"Name"`
                ImportPath string   `json:"ImportPath"`
                Dir        string   `json:"Dir"`
                GoFiles    []string `json:"GoFiles"`
                Doc        string   `json:"Doc"`
            }
            if err := decoder.Decode(&pkg); err == nil {
                summary.Packages = append(summary.Packages, PackageInfo{
                    Name:        pkg.Name,
                    Path:        pkg.ImportPath,
                    Description: pkg.Doc,
                })
            }
        }
    } else {
        errors = append(errors, fmt.Sprintf("go list failed: %v", err))
    }
    
    // Use go doc for text documentation
    cmd = exec.CommandContext(ctx, "go", "doc", "-all", ".")
    cmd.Dir = sourceDir
    if output, err := cmd.Output(); err == nil {
        tools = append(tools, "go doc")
        docPath := filepath.Join(outputDir, "api.txt")
        if err := ioutil.WriteFile(docPath, output, 0644); err == nil {
            paths["text"] = docPath
        }
    }
    
    // Parse Go files using go/ast for detailed API extraction
    err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || !strings.HasSuffix(path, ".go") {
            return nil
        }
        
        // Skip test files and vendor
        if strings.Contains(path, "vendor/") || strings.HasSuffix(path, "_test.go") {
            return nil
        }
        
        fset := token.NewFileSet()
        node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
        if err != nil {
            return nil
        }
        
        // Extract exported symbols
        for _, decl := range node.Decls {
            switch d := decl.(type) {
            case *ast.FuncDecl:
                if d.Name.IsExported() || includePrivate {
                    funcInfo := FunctionInfo{
                        Name:      d.Name.Name,
                        Package:   node.Name.Name,
                        Signature: extractFuncSignature(d),
                    }
                    summary.Functions = append(summary.Functions, funcInfo)
                    
                    summary.Exports = append(summary.Exports, ExportInfo{
                        Name:      d.Name.Name,
                        Kind:      "function",
                        Package:   node.Name.Name,
                        File:      path,
                        Signature: funcInfo.Signature,
                    })
                }
                
            case *ast.GenDecl:
                for _, spec := range d.Specs {
                    switch s := spec.(type) {
                    case *ast.TypeSpec:
                        if s.Name.IsExported() || includePrivate {
                            typeInfo := TypeInfo{
                                Name:    s.Name.Name,
                                Package: node.Name.Name,
                            }
                            
                            // Determine type kind
                            switch s.Type.(type) {
                            case *ast.StructType:
                                typeInfo.Kind = "struct"
                            case *ast.InterfaceType:
                                typeInfo.Kind = "interface"
                            default:
                                typeInfo.Kind = "type"
                            }
                            
                            summary.Types = append(summary.Types, typeInfo)
                            summary.Exports = append(summary.Exports, ExportInfo{
                                Name:    s.Name.Name,
                                Kind:    "type",
                                Package: node.Name.Name,
                                File:    path,
                            })
                        }
                        
                    case *ast.ValueSpec:
                        for _, name := range s.Names {
                            if name.IsExported() || includePrivate {
                                kind := "var"
                                if d.Tok == token.CONST {
                                    kind = "const"
                                    summary.Constants = append(summary.Constants, ConstantInfo{
                                        Name:    name.Name,
                                        Package: node.Name.Name,
                                    })
                                }
                                
                                summary.Exports = append(summary.Exports, ExportInfo{
                                    Name:    name.Name,
                                    Kind:    kind,
                                    Package: node.Name.Name,
                                    File:    path,
                                })
                            }
                        }
                    }
                }
            }
        }
        
        return nil
    })
    
    if err != nil {
        errors = append(errors, fmt.Sprintf("AST parsing error: %v", err))
    }
    
    return summary, paths, tools, errors
}

// extractFuncSignature extracts a function signature as a string
func extractFuncSignature(fn *ast.FuncDecl) string {
    var sig strings.Builder
    sig.WriteString("func ")
    
    if fn.Recv != nil && len(fn.Recv.List) > 0 {
        sig.WriteString("(")
        // Simplified receiver rendering
        sig.WriteString("...)")
        sig.WriteString(" ")
    }
    
    sig.WriteString(fn.Name.Name)
    sig.WriteString("(")
    
    // Simplified parameter rendering
    if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
        sig.WriteString("...")
    }
    
    sig.WriteString(")")
    
    // Simplified return type rendering
    if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
        sig.WriteString(" ...")
    }
    
    return sig.String()
}

// Additional language generators would follow similar patterns...
func (a *APIGeneratorActivity) generateJSAPIDocs(ctx context.Context, sourceDir, outputDir string, includePrivate bool) (APISummary, map[string]string, []string, []string) {
    // Implementation for JavaScript/TypeScript
    return APISummary{}, map[string]string{}, []string{}, []string{"JavaScript API generation not yet implemented"}
}

func (a *APIGeneratorActivity) generatePythonAPIDocs(ctx context.Context, sourceDir, outputDir string, includePrivate bool) (APISummary, map[string]string, []string, []string) {
    // Implementation for Python
    return APISummary{}, map[string]string{}, []string{}, []string{"Python API generation not yet implemented"}
}

func (a *APIGeneratorActivity) generateJavaAPIDocs(ctx context.Context, sourceDir, outputDir string, includePrivate bool) (APISummary, map[string]string, []string, []string) {
    // Implementation for Java
    return APISummary{}, map[string]string{}, []string{}, []string{"Java API generation not yet implemented"}
}

func (a *APIGeneratorActivity) generateRustAPIDocs(ctx context.Context, sourceDir, outputDir string, includePrivate bool) (APISummary, map[string]string, []string, []string) {
    // Implementation for Rust
    return APISummary{}, map[string]string{}, []string{}, []string{"Rust API generation not yet implemented"}
}

func (a *APIGeneratorActivity) generateFallbackAPIDocs(ctx context.Context, sourceDir, outputDir string) (APISummary, map[string]string, []string, []string) {
    // Fallback implementation
    return APISummary{}, map[string]string{}, []string{}, []string{"No language-specific generator available"}
}
```

## Configuration

```yaml
# server/ops/config/activities.yaml
api_generator:
  tokei_path: "tokei"  # Or full path if not in PATH
  default_output_dir: "colony2.api"
  tool_timeout: 60s
```

## Registration

```go
// In server/ops/pkg/activity/registry.go or similar
package activity

import (
    "github.com/colony-2/colony2/server/ops/pkg/apigen"
)

func RegisterActivities(registry *Registry) {
    // Register API generator activity
    registry.Register("api_generator", apigen.NewAPIGeneratorActivity())
}
```

## Usage in Recipes

```yaml
name: document_cell
version: "1.0"

sequence:
  - id: generate_api
    op: api_generator
    inputs:
      source_dir: "{{ .inputs.cell_directory }}"
      output_dir: "{{ .inputs.cell_directory }}/colony2.api"
      detail_level: "standard"
      include_private: false
      output_formats: ["json", "text"]
    outputs:
      primary_language: "{{ .outputs.primary_language }}"
      language_stats: "{{ .outputs.language_stats }}"
      api_summary: "{{ .outputs.api_summary }}"
      documentation_paths: "{{ .outputs.documentation_paths }}"
```

## Testing

```go
// activity_test.go
package apigen

import (
    "context"
    "testing"
    "io/ioutil"
    "os"
    "path/filepath"
)

func TestAPIGeneratorActivity_Execute(t *testing.T) {
    tests := []struct {
        name    string
        input   APIGeneratorInput
        wantErr bool
        checkOutput func(t *testing.T, output APIGeneratorOutput)
    }{
        {
            name: "go_project",
            input: APIGeneratorInput{
                SourceDir: "./testdata/go_sample",
            },
            wantErr: false,
            checkOutput: func(t *testing.T, output APIGeneratorOutput) {
                if output.PrimaryLanguage != "Go" {
                    t.Errorf("expected Go, got %s", output.PrimaryLanguage)
                }
                if len(output.APISummary.Functions) == 0 {
                    t.Error("expected functions to be extracted")
                }
                if _, ok := output.DocumentationPaths["json"]; !ok {
                    t.Error("expected JSON output")
                }
            },
        },
        {
            name: "invalid_directory",
            input: APIGeneratorInput{
                SourceDir: "/nonexistent",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create temp output dir
            tmpDir, err := ioutil.TempDir("", "api_gen_test")
            if err != nil {
                t.Fatal(err)
            }
            defer os.RemoveAll(tmpDir)
            
            if tt.input.OutputDir == "" {
                tt.input.OutputDir = tmpDir
            }
            
            activity := NewAPIGeneratorActivity()
            config := APIGeneratorConfig{
                TokeiPath: "tokei",
            }
            
            output, err := activity.Execute(context.Background(), config, tt.input)
            
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            
            if !tt.wantErr && tt.checkOutput != nil {
                tt.checkOutput(t, output)
            }
        })
    }
}
```

## Required Dependencies

```go
// go.mod additions
require (
    // Standard library only for core functionality
    // External tools called via exec.Command
)
```

## Installation Requirements

The API generator requires `tokei` to be installed:

```bash
# macOS
brew install tokei

# Linux/Windows
cargo install tokei

# Or download from releases
# https://github.com/XAMPPRocky/tokei/releases
```

## Benefits

1. **Go RegisterableActivity**: Fully integrated with the existing ops framework
2. **Tokei for Detection**: Fast, accurate language statistics without regex hacks
3. **Native Go AST**: Uses Go's built-in AST parser for Go projects
4. **Extensible**: Easy to add more language-specific generators
5. **Structured Output**: Consistent JSON API summary across all languages
6. **No External Go Dependencies**: Uses only standard library for core functionality
7. **Tool Flexibility**: Can work with whatever documentation tools are available