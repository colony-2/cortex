# VibeThis OpenAPI Code Generator

## Overview
This module generates strongly-typed Go code from the VibeThis OpenAPI specification using oapi-codegen. It produces data models, HTTP clients, and embedded OpenAPI specifications for type-safe API interactions across the VibeThis ecosystem.

## Architecture

### Code Generation Pipeline
- **Source**: `api/openapi/vibethis-api.yaml` - OpenAPI 3.0.3 specification
- **Generator**: `oapi-codegen v2.1.0` - Go code generation tool
- **Configuration**: `codegen.yml` - Generation settings (models, client, embedded spec)
- **Output**: `pkg/openapi/generated.go` - Generated Go types and client

### Key Components
- **Data Models**: Go structs for Cell, Edge, Graph, GitStatus, ContainerStatus, FileInfo
- **HTTP Client**: Type-safe client interfaces (`Client`, `ClientWithResponses`)
- **Request/Response Types**: Strongly-typed API parameters and responses
- **Embedded Specification**: Runtime OpenAPI spec access via `GetSwagger()`

### Dependencies
- `github.com/getkin/kin-openapi` - OpenAPI spec parsing and validation
- `github.com/oapi-codegen/runtime` - Runtime support for generated code

## Key Interfaces

### Client Creation
```go
func NewClient(server string, opts ...ClientOption) (*Client, error)
func NewClientWithResponses(server string, opts ...ClientOption) (*ClientWithResponses, error)
```

### Core Client Interface
```go
type ClientInterface interface {
    // Graph operations
    GetGraph(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error)
    
    // Container operations
    CreateContainer(ctx context.Context, cellId string, reqEditors ...RequestEditorFn) (*http.Response, error)
    UpdateDevcontainerWithBody(ctx context.Context, cellId string, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error)
    
    // File operations
    GetFiles(ctx context.Context, cellId string, params *GetFilesParams, reqEditors ...RequestEditorFn) (*http.Response, error)
    GetFileContent(ctx context.Context, cellId string, params *GetFileContentParams, reqEditors ...RequestEditorFn) (*http.Response, error)
    
    // Git operations
    GetGitStatus(ctx context.Context, cellId string, reqEditors ...RequestEditorFn) (*http.Response, error)
    GetGitCommits(ctx context.Context, cellId string, params *GetGitCommitsParams, reqEditors ...RequestEditorFn) (*http.Response, error)
}
```

### Response-Wrapped Client
```go
type ClientWithResponsesInterface interface {
    GetGraphWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*GetGraphResponse, error)
    CreateContainerWithResponse(ctx context.Context, cellId string, reqEditors ...RequestEditorFn) (*CreateContainerResponse, error)
    GetFilesWithResponse(ctx context.Context, cellId string, params *GetFilesParams, reqEditors ...RequestEditorFn) (*GetFilesResponse, error)
    // ... other operations with typed responses
}
```

### Core Data Models
```go
type Graph struct {
    Cells []Cell `json:"cells"`
    Edges []Edge `json:"edges"`
}

type Cell struct {
    Id           string   `json:"id"`
    Name         string   `json:"name"`
    Path         string   `json:"path"`
    Type         string   `json:"type"`
    Dependencies []string `json:"dependencies"`
}

type Edge struct {
    Id     string `json:"id"`
    Source string `json:"source"`
    Target string `json:"target"`
}
```

## Usage Examples

### Basic Client Setup
```go
import "github.com/divisive-ai/vibethis/server/openapi/pkg/openapi"

// Create basic HTTP client
client, err := openapi.NewClient("http://localhost:8080")
if err != nil {
    return err
}

// Create response-wrapped client for easier handling
clientWithResponses, err := openapi.NewClientWithResponses("http://localhost:8080")
if err != nil {
    return err
}
```

### Fetch Graph Data
```go
// Using response-wrapped client
response, err := clientWithResponses.GetGraphWithResponse(ctx)
if err != nil {
    return err
}

if response.StatusCode() == 200 {
    graph := response.JSON200
    for _, cell := range graph.Cells {
        fmt.Printf("Cell: %s (%s)\n", cell.Name, cell.Type)
    }
}
```

### Container Management
```go
// Create container for a cell
response, err := clientWithResponses.CreateContainerWithResponse(ctx, "cell-id")
if err != nil {
    return err
}

if response.StatusCode() == 200 {
    status := response.JSON200
    fmt.Printf("Container status: %s\n", status.Status)
}
```

### File Operations
```go
// List files in a cell
params := &openapi.GetFilesParams{
    Path: openapi.StringPtr("src/"),
}

response, err := clientWithResponses.GetFilesWithResponse(ctx, "cell-id", params)
if err != nil {
    return err
}

if response.StatusCode() == 200 {
    files := response.JSON200
    for _, file := range *files {
        fmt.Printf("File: %s (size: %d)\n", file.Name, file.Size)
    }
}
```

### Git Status Check
```go
response, err := clientWithResponses.GetGitStatusWithResponse(ctx, "cell-id")
if err != nil {
    return err
}

if response.StatusCode() == 200 {
    status := response.JSON200
    fmt.Printf("Branch: %s, Clean: %v\n", status.Branch, status.Clean)
    for _, file := range status.Files {
        fmt.Printf("  %s: %s\n", file.Status, file.Path)
    }
}
```

## Configuration

### Code Generation Settings (`codegen.yml`)
```yaml
package: openapi
generate:
  models: true        # Generate Go structs for API types
  embedded-spec: true # Embed OpenAPI spec for runtime access
  client: true        # Generate HTTP client interfaces
output-options:
  skip-prune: true    # Retain all types from specification
```

### Build Configuration (`moon.yml`)
```yaml
tasks:
  build:
    script: |
      rm -f pkg/openapi/generated.go
      go run github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@v2.1.0 -config codegen.yml -o pkg/openapi/generated.go ../../api/openapi/vibethis-api.yaml
    deps:
      - 'api-openapi:build'  # Ensure spec is built first
```

### Client Configuration Options
```go
// Custom HTTP client
httpClient := &http.Client{Timeout: 30 * time.Second}
client, err := openapi.NewClient("http://localhost:8080", 
    openapi.WithHTTPClient(httpClient))

// Request editor for authentication
authEditor := func(ctx context.Context, req *http.Request) error {
    req.Header.Set("Authorization", "Bearer "+token)
    return nil
}

response, err := client.GetGraph(ctx, authEditor)
```