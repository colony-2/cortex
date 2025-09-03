# API Generator Framework Specification

## Overview
A modular framework where each language has its own API extraction script that follows a standard interface. Scripts can be written in their native language or shell, whichever makes most sense.

## Directory Structure

```
server/ops/pkg/apigen/
├── activity.go           # Main RegisterableActivity
├── tokei.go             # Language detection via tokei
├── framework.go         # Script execution framework
├── types.go             # Shared types
├── scripts/             # Language-specific scripts
│   ├── go.go            # Go extractor (written in Go)
│   ├── python.py        # Python extractor (written in Python)
│   ├── node.js          # JavaScript/TypeScript extractor
│   ├── java.sh          # Java extractor (shell script)
│   ├── rust.sh          # Rust extractor (shell script)
│   ├── csharp.sh        # C# extractor (shell script)
│   ├── ruby.rb          # Ruby extractor (written in Ruby)
│   ├── php.php          # PHP extractor (written in PHP)
│   └── fallback.sh      # Universal fallback
└── scripts_test/        # Test data for each language
```

## Script Interface Contract

Every script MUST:
1. Accept command-line arguments
2. Output JSON to stdout
3. Output errors/warnings to stderr
4. Return exit code 0 on success, non-zero on failure

### Input Arguments

```bash
./script.ext <source_dir> <output_dir> [options]

# Required arguments:
# $1: source_dir - Directory to analyze
# $2: output_dir - Directory for output files

# Optional flags:
# --include-private    Include private/internal APIs
# --detail-level=<level>  minimal|standard|detailed
# --format=<format>    json|text|html|markdown
```

### Output Format (stdout)

All scripts must output this JSON structure to stdout:

```json
{
  "packages": [
    {
      "name": "string",
      "path": "string",
      "description": "string"
    }
  ],
  "exports": [
    {
      "name": "string",
      "kind": "function|type|const|var|class|interface",
      "package": "string",
      "file": "string",
      "line": 0,
      "signature": "string",
      "description": "string"
    }
  ],
  "functions": [...],
  "types": [...],
  "constants": [...],
  "tools_used": ["tool1", "tool2"],
  "files_generated": {
    "json": "path/to/api.json",
    "text": "path/to/api.txt",
    "html": "path/to/index.html"
  }
}
```

## Language-Specific Scripts

### Go Script (go.sh)

```bash
#!/bin/bash

# API extractor for Go projects
# Usage: ./go.sh <source_dir> <output_dir> [flags]

set -e

SOURCE_DIR="$1"
OUTPUT_DIR="$2"
INCLUDE_PRIVATE=false

# Parse flags
for arg in "${@:3}"; do
    case $arg in
        --include-private)
            INCLUDE_PRIVATE=true
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Initialize output JSON
OUTPUT_JSON='{
  "packages": [],
  "exports": [],
  "functions": [],
  "types": [],
  "constants": [],
  "tools_used": [],
  "files_generated": {}
}'

cd "$SOURCE_DIR"

# Use go list to get package information
if command -v go &> /dev/null; then
    # Get package list with JSON output
    go list -json ./... > "$OUTPUT_DIR/packages.json" 2>/dev/null || true
    if [ -f "$OUTPUT_DIR/packages.json" ]; then
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["go list"]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.packages = "'$OUTPUT_DIR'/packages.json"')
        
        # Parse packages into our format
        PACKAGES=$(cat "$OUTPUT_DIR/packages.json" | jq -s '[.[] | {name: .Name, path: .ImportPath, description: .Doc}]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --argjson packages "$PACKAGES" '.packages = $packages')
    fi
    
    # Use go doc for documentation
    if [ "$INCLUDE_PRIVATE" = true ]; then
        go doc -all -u ./... > "$OUTPUT_DIR/api.txt" 2>/dev/null || true
    else
        go doc -all ./... > "$OUTPUT_DIR/api.txt" 2>/dev/null || true
    fi
    
    if [ -f "$OUTPUT_DIR/api.txt" ]; then
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["go doc"]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.text = "'$OUTPUT_DIR'/api.txt"')
        
        # Parse go doc output to extract exports
        # Extract function signatures
        grep "^func " "$OUTPUT_DIR/api.txt" | while read -r line; do
            func_name=$(echo "$line" | sed -E 's/^func (\(.*\) )?([A-Za-z0-9_]+).*/\2/')
            if [ -n "$func_name" ]; then
                OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --arg name "$func_name" \
                    '.exports += [{name: $name, kind: "function"}]')
            fi
        done
        
        # Extract type definitions
        grep "^type " "$OUTPUT_DIR/api.txt" | while read -r line; do
            type_name=$(echo "$line" | sed -E 's/^type ([A-Za-z0-9_]+).*/\1/')
            if [ -n "$type_name" ]; then
                OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --arg name "$type_name" \
                    '.exports += [{name: $name, kind: "type"}]')
            fi
        done
    fi
    
    # Use godoc if available for HTML
    if command -v godoc &> /dev/null; then
        # Start godoc server briefly to generate HTML
        godoc -http=:6060 &
        GODOC_PID=$!
        sleep 2
        
        # Try to fetch the documentation
        curl -s "http://localhost:6060/pkg/" > "$OUTPUT_DIR/godoc.html" 2>/dev/null || true
        
        kill $GODOC_PID 2>/dev/null || true
        
        if [ -f "$OUTPUT_DIR/godoc.html" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["godoc"]')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.html = "'$OUTPUT_DIR'/godoc.html"')
        fi
    fi
fi

# Write final JSON
echo "$OUTPUT_JSON" | jq '.' > "$OUTPUT_DIR/api.json"
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.json = "'$OUTPUT_DIR'/api.json"')

# Output to stdout
echo "$OUTPUT_JSON" | jq '.'
```

### Python Script (python.sh)

```bash
#!/bin/bash

# API extractor for Python projects
# Usage: ./python.sh <source_dir> <output_dir> [flags]

set -e

SOURCE_DIR="$1"
OUTPUT_DIR="$2"
INCLUDE_PRIVATE=false

# Parse flags
for arg in "${@:3}"; do
    case $arg in
        --include-private)
            INCLUDE_PRIVATE=true
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Initialize output JSON
OUTPUT_JSON='{
  "packages": [],
  "exports": [],
  "functions": [],
  "types": [],
  "constants": [],
  "tools_used": [],
  "files_generated": {}
}'

cd "$SOURCE_DIR"

# Use pydoc for documentation
if command -v python3 &> /dev/null; then
    # Generate text documentation with pydoc
    python3 -m pydoc -w . > /dev/null 2>&1 || true
    
    # Move generated HTML files to output directory
    if ls *.html 1> /dev/null 2>&1; then
        mv *.html "$OUTPUT_DIR/" 2>/dev/null || true
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["pydoc"]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.html = "'$OUTPUT_DIR'"')
    fi
    
    # Use pydoc to get module information
    for py_file in $(find . -name "*.py" -not -path "./__pycache__/*" -not -path "./venv/*" -not -path "./.venv/*"); do
        module_name=$(echo "$py_file" | sed 's|^\./||' | sed 's|\.py$||' | sed 's|/|.|g')
        
        # Get module documentation
        python3 -m pydoc "$module_name" > "$OUTPUT_DIR/${module_name}.txt" 2>/dev/null || true
        
        if [ -f "$OUTPUT_DIR/${module_name}.txt" ]; then
            # Extract functions
            grep "^    [a-zA-Z_]" "$OUTPUT_DIR/${module_name}.txt" | while read -r line; do
                func_name=$(echo "$line" | sed -E 's/^    ([a-zA-Z_][a-zA-Z0-9_]*).*/\1/')
                if [ -n "$func_name" ]; then
                    if [ "$INCLUDE_PRIVATE" = true ] || [[ ! "$func_name" =~ ^_ ]]; then
                        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --arg name "$func_name" --arg module "$module_name" \
                            '.exports += [{name: $name, kind: "function", package: $module}]')
                    fi
                fi
            done
            
            # Extract classes
            grep "^class " "$OUTPUT_DIR/${module_name}.txt" | while read -r line; do
                class_name=$(echo "$line" | sed -E 's/^class ([A-Za-z_][A-Za-z0-9_]*).*/\1/')
                if [ -n "$class_name" ]; then
                    if [ "$INCLUDE_PRIVATE" = true ] || [[ ! "$class_name" =~ ^_ ]]; then
                        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --arg name "$class_name" --arg module "$module_name" \
                            '.exports += [{name: $name, kind: "class", package: $module}]')
                        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --arg name "$class_name" --arg module "$module_name" \
                            '.types += [{name: $name, kind: "class", package: $module}]')
                    fi
                fi
            done
        fi
    done
    
    # Try pdoc3 if available for better JSON output
    if command -v pdoc3 &> /dev/null; then
        pdoc3 --json . > "$OUTPUT_DIR/pdoc.json" 2>/dev/null || true
        if [ -f "$OUTPUT_DIR/pdoc.json" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["pdoc3"]')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.pdoc = "'$OUTPUT_DIR'/pdoc.json"')
        fi
    fi
    
    # Try sphinx-apidoc if available
    if command -v sphinx-apidoc &> /dev/null; then
        sphinx-apidoc -o "$OUTPUT_DIR/sphinx" . 2>/dev/null || true
        if [ -d "$OUTPUT_DIR/sphinx" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["sphinx-apidoc"]')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.sphinx = "'$OUTPUT_DIR'/sphinx"')
        fi
    fi
fi

# Write final JSON
echo "$OUTPUT_JSON" | jq '.' > "$OUTPUT_DIR/api.json"
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.json = "'$OUTPUT_DIR'/api.json"')

# Output to stdout
echo "$OUTPUT_JSON" | jq '.'
```

### Node.js Script (node.sh)

```bash
#!/bin/bash

# API extractor for JavaScript/TypeScript projects
# Usage: ./node.sh <source_dir> <output_dir> [flags]

set -e

SOURCE_DIR="$1"
OUTPUT_DIR="$2"
INCLUDE_PRIVATE=false

# Parse flags
for arg in "${@:3}"; do
    case $arg in
        --include-private)
            INCLUDE_PRIVATE=true
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Initialize output JSON
OUTPUT_JSON='{
  "packages": [],
  "exports": [],
  "functions": [],
  "types": [],
  "constants": [],
  "tools_used": [],
  "files_generated": {}
}'

cd "$SOURCE_DIR"

# Check if TypeScript project
if [ -f "tsconfig.json" ]; then
    # Use TypeDoc for TypeScript
    if command -v npx &> /dev/null; then
        npx typedoc --json "$OUTPUT_DIR/typedoc.json" --emit none . 2>/dev/null || true
        if [ -f "$OUTPUT_DIR/typedoc.json" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["typedoc"]')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.typedoc = "'$OUTPUT_DIR'/typedoc.json"')
            
            # Parse TypeDoc output to extract exports
            EXPORTS=$(cat "$OUTPUT_DIR/typedoc.json" | jq -r '
                [.children[]?.children[]? | 
                select(.name != null) |
                {name: .name, kind: .kindString, file: .sources[0].fileName}]
            ')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --argjson exports "$EXPORTS" '.exports = $exports')
        fi
        
        # Also generate HTML docs
        npx typedoc --out "$OUTPUT_DIR/html" . 2>/dev/null || true
        if [ -d "$OUTPUT_DIR/html" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.html = "'$OUTPUT_DIR'/html"')
        fi
    fi
else
    # Use documentation.js for JavaScript
    if command -v npx &> /dev/null; then
        # Generate JSON documentation
        npx documentation build . -f json > "$OUTPUT_DIR/docs.json" 2>/dev/null || true
        if [ -f "$OUTPUT_DIR/docs.json" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["documentation.js"]')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.docs = "'$OUTPUT_DIR'/docs.json"')
            
            # Parse documentation.js output
            EXPORTS=$(cat "$OUTPUT_DIR/docs.json" | jq -r '
                [.[] | 
                {name: .name, kind: .kind, description: .description.children[0].value}]
            ')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --argjson exports "$EXPORTS" '.exports = $exports')
        fi
        
        # Generate HTML documentation
        npx documentation build . -f html -o "$OUTPUT_DIR/html" 2>/dev/null || true
        if [ -d "$OUTPUT_DIR/html" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.html = "'$OUTPUT_DIR'/html"')
        fi
        
        # Generate Markdown documentation
        npx documentation build . -f md > "$OUTPUT_DIR/api.md" 2>/dev/null || true
        if [ -f "$OUTPUT_DIR/api.md" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.markdown = "'$OUTPUT_DIR'/api.md"')
        fi
    fi
    
    # Try JSDoc as fallback
    if [ ! -f "$OUTPUT_DIR/docs.json" ] && command -v npx &> /dev/null; then
        npx jsdoc -X . > "$OUTPUT_DIR/jsdoc.json" 2>/dev/null || true
        if [ -f "$OUTPUT_DIR/jsdoc.json" ]; then
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["jsdoc"]')
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.jsdoc = "'$OUTPUT_DIR'/jsdoc.json"')
            
            # Parse JSDoc output
            EXPORTS=$(cat "$OUTPUT_DIR/jsdoc.json" | jq -r '
                [.[] | 
                select(.undocumented != true) |
                select(.access == "public" or .access == null) |
                {name: .name, kind: .kind, description: .description}]
            ')
            
            if [ "$INCLUDE_PRIVATE" = true ]; then
                EXPORTS=$(cat "$OUTPUT_DIR/jsdoc.json" | jq -r '
                    [.[] | 
                    select(.undocumented != true) |
                    {name: .name, kind: .kind, description: .description}]
                ')
            fi
            
            OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --argjson exports "$EXPORTS" '.exports = $exports')
        fi
    fi
fi

# Write final JSON
echo "$OUTPUT_JSON" | jq '.' > "$OUTPUT_DIR/api.json"
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.json = "'$OUTPUT_DIR'/api.json"')

# Output to stdout
echo "$OUTPUT_JSON" | jq '.'
```

### Java Script (java.sh)

```bash
#!/bin/bash

# API extractor for Java projects
# Usage: ./java.sh <source_dir> <output_dir> [flags]

set -e

SOURCE_DIR="$1"
OUTPUT_DIR="$2"
INCLUDE_PRIVATE=false

# Parse flags
for arg in "${@:3}"; do
    case $arg in
        --include-private)
            INCLUDE_PRIVATE=true
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Initialize output JSON
OUTPUT_JSON='{
  "packages": [],
  "exports": [],
  "functions": [],
  "types": [],
  "constants": [],
  "tools_used": [],
  "files_generated": {}
}'

# Try using javadoc
if command -v javadoc &> /dev/null; then
    javadoc -d "$OUTPUT_DIR/javadoc" \
            -sourcepath "$SOURCE_DIR" \
            -subpackages . \
            -quiet \
            2>/dev/null || true
    
    if [ -d "$OUTPUT_DIR/javadoc" ]; then
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["javadoc"]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.html = "'$OUTPUT_DIR'/javadoc"')
    fi
fi

# Extract public APIs using grep (fallback)
EXPORTS_FILE="$OUTPUT_DIR/exports.json"
echo '[]' > "$EXPORTS_FILE"

find "$SOURCE_DIR" -name "*.java" -type f | while read -r file; do
    # Extract public classes
    grep -n "public class\|public interface\|public enum" "$file" 2>/dev/null | while IFS=: read -r line_num line_content; do
        class_name=$(echo "$line_content" | sed -E 's/.*public (class|interface|enum) ([A-Za-z0-9_]+).*/\2/')
        if [ -n "$class_name" ]; then
            jq --arg name "$class_name" \
               --arg kind "class" \
               --arg file "$file" \
               --arg line "$line_num" \
               '. += [{name: $name, kind: $kind, file: $file, line: ($line | tonumber)}]' \
               "$EXPORTS_FILE" > "$EXPORTS_FILE.tmp" && mv "$EXPORTS_FILE.tmp" "$EXPORTS_FILE"
        fi
    done
    
    # Extract public methods
    if [ "$INCLUDE_PRIVATE" = true ]; then
        pattern="(public|protected|private).*\s+\w+\s*\("
    else
        pattern="public.*\s+\w+\s*\("
    fi
    
    grep -n "$pattern" "$file" 2>/dev/null | while IFS=: read -r line_num line_content; do
        # Simple method name extraction
        method_name=$(echo "$line_content" | sed -E 's/.*\s+([A-Za-z0-9_]+)\s*\(.*/\1/')
        if [ -n "$method_name" ] && [ "$method_name" != "class" ] && [ "$method_name" != "interface" ]; then
            jq --arg name "$method_name" \
               --arg kind "function" \
               --arg file "$file" \
               --arg line "$line_num" \
               '. += [{name: $name, kind: $kind, file: $file, line: ($line | tonumber)}]' \
               "$EXPORTS_FILE" > "$EXPORTS_FILE.tmp" && mv "$EXPORTS_FILE.tmp" "$EXPORTS_FILE"
        fi
    done
done

# Add exports to output
EXPORTS=$(cat "$EXPORTS_FILE")
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq --argjson exports "$EXPORTS" '.exports = $exports')

# Write final JSON
echo "$OUTPUT_JSON" | jq '.' > "$OUTPUT_DIR/api.json"
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.json = "'$OUTPUT_DIR'/api.json"')

# Output to stdout
echo "$OUTPUT_JSON" | jq '.'
```

### Fallback Script (fallback.sh)

```bash
#!/bin/bash

# Universal fallback API extractor
# Usage: ./fallback.sh <source_dir> <output_dir> [flags]

set -e

SOURCE_DIR="$1"
OUTPUT_DIR="$2"

mkdir -p "$OUTPUT_DIR"

OUTPUT_JSON='{
  "packages": [],
  "exports": [],
  "functions": [],
  "types": [],
  "constants": [],
  "tools_used": [],
  "files_generated": {}
}'

# Try ctags
if command -v ctags &> /dev/null; then
    ctags -R --output-format=json -o "$OUTPUT_DIR/tags.json" "$SOURCE_DIR" 2>/dev/null || true
    if [ -f "$OUTPUT_DIR/tags.json" ]; then
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["ctags"]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.ctags = "'$OUTPUT_DIR'/tags.json"')
    fi
fi

# Try doxygen
if command -v doxygen &> /dev/null; then
    cat > "$OUTPUT_DIR/Doxyfile" << EOF
PROJECT_NAME = "API Documentation"
OUTPUT_DIRECTORY = $OUTPUT_DIR/doxygen
INPUT = $SOURCE_DIR
GENERATE_XML = YES
GENERATE_HTML = YES
RECURSIVE = YES
EXTRACT_ALL = YES
QUIET = YES
WARNINGS = NO
EOF
    
    doxygen "$OUTPUT_DIR/Doxyfile" 2>/dev/null || true
    if [ -d "$OUTPUT_DIR/doxygen" ]; then
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.tools_used += ["doxygen"]')
        OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.doxygen = "'$OUTPUT_DIR'/doxygen"')
    fi
fi

# Basic file listing as last resort
find "$SOURCE_DIR" -type f | head -100 > "$OUTPUT_DIR/files.txt"
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.files = "'$OUTPUT_DIR'/files.txt"')

# Write and output JSON
echo "$OUTPUT_JSON" | jq '.' > "$OUTPUT_DIR/api.json"
OUTPUT_JSON=$(echo "$OUTPUT_JSON" | jq '.files_generated.json = "'$OUTPUT_DIR'/api.json"')

echo "$OUTPUT_JSON" | jq '.'
```

## Go Activity Integration

### framework.go

```go
package apigen

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "time"
)

// ScriptRunner executes language-specific API extraction scripts
type ScriptRunner struct {
    scriptsDir string
    timeout    time.Duration
}

// NewScriptRunner creates a new script runner
func NewScriptRunner(scriptsDir string, timeout time.Duration) *ScriptRunner {
    return &ScriptRunner{
        scriptsDir: scriptsDir,
        timeout:    timeout,
    }
}

// Run executes the appropriate script for the given language
func (r *ScriptRunner) Run(ctx context.Context, language, sourceDir, outputDir string, options map[string]interface{}) (APISummary, map[string]string, []string, error) {
    // Map language to script
    scriptName := r.getScriptName(language)
    scriptPath := filepath.Join(r.scriptsDir, scriptName)
    
    // Check if script exists
    if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
        // Use fallback
        scriptPath = filepath.Join(r.scriptsDir, "fallback.sh")
    }
    
    // Build command arguments
    args := []string{sourceDir, outputDir}
    
    // Add options as flags
    if includePrivate, ok := options["include_private"].(bool); ok && includePrivate {
        args = append(args, "--include-private")
    }
    if detailLevel, ok := options["detail_level"].(string); ok {
        args = append(args, fmt.Sprintf("--detail-level=%s", detailLevel))
    }
    if format, ok := options["format"].(string); ok {
        args = append(args, fmt.Sprintf("--format=%s", format))
    }
    
    // Determine how to run the script
    var cmd *exec.Cmd
    ext := filepath.Ext(scriptPath)
    
    switch ext {
    case ".go":
        cmd = exec.CommandContext(ctx, "go", append([]string{"run", scriptPath}, args...)...)
    case ".py":
        cmd = exec.CommandContext(ctx, "python3", append([]string{scriptPath}, args...)...)
    case ".js":
        cmd = exec.CommandContext(ctx, "node", append([]string{scriptPath}, args...)...)
    case ".rb":
        cmd = exec.CommandContext(ctx, "ruby", append([]string{scriptPath}, args...)...)
    case ".php":
        cmd = exec.CommandContext(ctx, "php", append([]string{scriptPath}, args...)...)
    case ".sh":
        cmd = exec.CommandContext(ctx, "bash", append([]string{scriptPath}, args...)...)
    default:
        // Try to execute directly
        cmd = exec.CommandContext(ctx, scriptPath, args...)
    }
    
    // Set timeout
    ctx, cancel := context.WithTimeout(ctx, r.timeout)
    defer cancel()
    
    // Execute script
    output, err := cmd.Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            return APISummary{}, nil, nil, fmt.Errorf("script failed: %s", exitErr.Stderr)
        }
        return APISummary{}, nil, nil, fmt.Errorf("script execution failed: %w", err)
    }
    
    // Parse JSON output
    var scriptOutput struct {
        Packages       []PackageInfo        `json:"packages"`
        Exports        []ExportInfo         `json:"exports"`
        Functions      []FunctionInfo       `json:"functions"`
        Types          []TypeInfo           `json:"types"`
        Constants      []ConstantInfo       `json:"constants"`
        ToolsUsed      []string             `json:"tools_used"`
        FilesGenerated map[string]string    `json:"files_generated"`
    }
    
    if err := json.Unmarshal(output, &scriptOutput); err != nil {
        return APISummary{}, nil, nil, fmt.Errorf("failed to parse script output: %w", err)
    }
    
    // Convert to APISummary
    summary := APISummary{
        Packages:  scriptOutput.Packages,
        Exports:   scriptOutput.Exports,
        Functions: scriptOutput.Functions,
        Types:     scriptOutput.Types,
        Constants: scriptOutput.Constants,
    }
    
    return summary, scriptOutput.FilesGenerated, scriptOutput.ToolsUsed, nil
}

// getScriptName maps language names to script files
func (r *ScriptRunner) getScriptName(language string) string {
    language = strings.ToLower(language)
    
    scriptMap := map[string]string{
        "go":         "go.go",
        "golang":     "go.go",
        "python":     "python.py",
        "javascript": "node.js",
        "jsx":        "node.js",
        "typescript": "node.js",
        "tsx":        "node.js",
        "java":       "java.sh",
        "rust":       "rust.sh",
        "c#":         "csharp.sh",
        "csharp":     "csharp.sh",
        "ruby":       "ruby.rb",
        "php":        "php.php",
        "c":          "c.sh",
        "c++":        "cpp.sh",
        "cpp":        "cpp.sh",
    }
    
    if script, ok := scriptMap[language]; ok {
        return script
    }
    
    return "fallback.sh"
}
```

### Updated Activity Execute Method

```go
// In activity.go, update the Execute method to use ScriptRunner

func (a *APIGeneratorActivity) Execute(
    ctx context.Context,
    config APIGeneratorConfig,
    input APIGeneratorInput,
) (APIGeneratorOutput, error) {
    // ... initialization code ...
    
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
    
    // Step 2: Use ScriptRunner for API extraction
    scriptsDir := filepath.Join(runtime.GOROOT(), "src", "github.com/divisive-ai/vibethis/server/ops/pkg/apigen/scripts")
    if envDir := os.Getenv("APIGEN_SCRIPTS_DIR"); envDir != "" {
        scriptsDir = envDir
    }
    
    runner := NewScriptRunner(scriptsDir, config.ToolTimeout)
    
    options := map[string]interface{}{
        "include_private": input.IncludePrivate,
        "detail_level":    input.DetailLevel,
        "format":          strings.Join(input.OutputFormats, ","),
    }
    
    summary, paths, tools, err := runner.Run(
        ctx,
        primaryLang,
        input.SourceDir,
        input.OutputDir,
        options,
    )
    
    if err != nil {
        output.Errors = append(output.Errors, fmt.Sprintf("Script execution failed: %v", err))
        // Try fallback
        summary, paths, tools, _ = runner.Run(ctx, "unknown", input.SourceDir, input.OutputDir, options)
    }
    
    output.APISummary = summary
    output.DocumentationPaths = paths
    output.ToolsUsed = append(output.ToolsUsed, tools...)
    
    return output, nil
}
```

## Benefits

1. **Language-Native Scripts**: Each language uses its own tools and libraries
2. **Standard Interface**: All scripts follow the same input/output contract
3. **Easy to Extend**: Just add a new script for a new language
4. **Flexible Implementation**: Scripts can be shell, native language, or Go
5. **Graceful Fallback**: Universal fallback for unsupported languages
6. **Tool Agnostic**: Each script can use whatever tools are available
7. **Testable**: Each script can be tested independently
8. **Maintainable**: Language experts can maintain their own scripts