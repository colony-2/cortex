# Cortex

## Overview
Cortex is a CLI-based recipe management and visualization tool that provides validation, schema generation, and execution capabilities for YAML-based recipes. It combines a web server for visualization with command-line tools for CI/CD integration and standalone recipe operations.

## Architecture

### Core Components
- **Main CLI (`cmd/cortex/main.go`)** - Cobra-based CLI with server, schema, validate, and execute subcommands
- **Config (`internal/config/config.go`)** - Application configuration management with path validation
- **Setup (`internal/setup/setup.go`)** - Dependency injection and server initialization
- **Static (`internal/static/`)** - Embedded React frontend assets with production/development build modes

### Shared Services
- **RegistryManager (`internal/shared/registry.go`)** - Activity registry management and executor initialization
- **SchemaManager (`internal/shared/schema.go`)** - JSON schema generation for recipes and activities
- **RecipeValidator (`internal/shared/validator.go`)** - Recipe structure and input validation
- **TemplateResolver (`internal/shared/template.go`)** - Template resolution with Go templates, CEL expressions, and simple references

### Dependencies
- **Storage** - BoltDB persistence layer (`.vibethis` directory)
- **Graph** - Project dependency graph builder
- **Files** - File system browser
- **Git** - Git repository operations
- **Container** - Development container management
- **API** - Web server and HTTP routing

## Key Interfaces

### CLI Commands
```go
// Server command - starts web visualization
cortex server [path] --port 8080 --new

// Schema generation - generates JSON schemas
cortex schema --format json --activity <type> --output schema.json

// Validation - validates recipe files
cortex validate <recipe-files...> --format text --strict

// Execution - executes recipes directly
cortex execute <recipe-file> --input '{"key":"value"}' --output json
```

### RegistryManager
```go
type RegistryManager struct {
    registry *worker.ActivityRegistry
    executor *executor.StandaloneExecutor
    logger   *zap.Logger
}

func NewRegistryManager(logger *zap.Logger) (*RegistryManager, error)
func (rm *RegistryManager) GetRegistry() *worker.ActivityRegistry
func (rm *RegistryManager) GetExecutor() *executor.StandaloneExecutor
```

### RecipeValidator
```go
type RecipeValidator struct {
    registryManager *RegistryManager
}

func NewRecipeValidator(rm *RegistryManager) *RecipeValidator
func (rv *RecipeValidator) ValidateRecipeStructure(recipe *yamlpkg.RecipeDefinition) error
func (rv *RecipeValidator) ValidateInputs(recipe *yamlpkg.RecipeDefinition, inputs map[string]interface{}) error
```

### TemplateResolver
```go
type TemplateResolver struct {
    funcMap template.FuncMap
    celEval *CELEvaluator
}

func NewTemplateResolver() (*TemplateResolver, error)
func (tr *TemplateResolver) ResolveOutputTemplate(outputTemplate map[string]interface{}, nodeOutputs map[string]map[string]interface{}, inputs map[string]interface{}) (map[string]interface{}, error)
func (tr *TemplateResolver) ResolveInputTemplate(inputTemplate map[string]interface{}, context map[string]interface{}) (map[string]interface{}, error)
```

## Usage Examples

### Starting Web Server
```bash
# Default server on port 8080
cortex server /path/to/project

# Custom port with new database
cortex server /path/to/project --port 3000 --new
```

### Schema Generation
```bash
# Generate complete recipe schema
cortex schema --format json --output recipe-schema.json

# Generate schema for specific activity
cortex schema --activity git-clone --format yaml

# Generate OpenAPI schema
cortex schema --format openapi --version
```

### Recipe Validation
```bash
# Validate single recipe
cortex validate recipe.yaml

# Validate multiple recipes with strict mode
cortex validate recipes/*.yaml --strict --format markdown

# Validate with custom schema
cortex validate recipe.yaml --schema custom-schema.json
```

### Recipe Execution
```bash
# Execute with JSON inputs
cortex execute recipe.yaml --input '{"source":"repo.git","branch":"main"}'

# Execute with input file
cortex execute recipe.yaml --input-file inputs.json --output yaml

# Dry run validation
cortex execute recipe.yaml --dry-run --log-level debug

# Execute with timeout and cleanup
cortex execute recipe.yaml --timeout 10m --cleanup --parallel-limit 5
```

### Template Resolution Examples
```go
// Initialize resolver
resolver, err := shared.NewTemplateResolver()

// Simple reference
template := "{{ .nodes.clone.outputs.path }}"

// CEL expression
template := "{{ cel: nodes.clone.outputs.path + '/' + inputs.subdir }}"

// Go template with functions
template := "{{ .inputs.name | upper | trim }}"
```

## Configuration

### Server Configuration
```go
type Config struct {
    Port      int    // Server port (default: 8080)
    RootPath  string // Project root path (default: current directory)
    CreateNew bool   // Create new database if missing
}
```

### Build Configuration
- **Production**: `go build -tags prod` - Embeds React assets
- **Development**: `go build` - Serves assets from filesystem
- **Moon Tasks**: `build`, `build-frontend-copy`, `serve`, `install`

### Database Location
- Database directory: `{RootPath}/.vibethis`
- Automatically created when `--new` flag is used
- BoltDB storage with configurable read-only mode