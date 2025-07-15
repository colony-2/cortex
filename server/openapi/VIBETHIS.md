# server/openapi

## Purpose

This directory contains the Go code generation infrastructure for the VibeThis OpenAPI specification. It uses `oapi-codegen` to generate strongly-typed Go models and HTTP client code from the OpenAPI specification located at `api/openapi/vibethis-api.yaml`.

## Key Components

### Code Generation
- **Tool**: Uses `oapi-codegen` v2.1.0 to generate Go code
- **Config**: `codegen.yml` configures generation options:
  - Generates models, embedded spec, and client code
  - Skips pruning to retain all types
- **Output**: Generated code is placed in `pkg/openapi/generated.go`

### Generated Artifacts
The generated code includes:
- **Data Models**: Go structs for all API types (Node, Edge, Graph, GitStatus, ContainerStatus, etc.)
- **HTTP Client**: Type-safe client (`Client` and `ClientWithResponses`) for making API calls
- **Request/Response Types**: Strongly-typed request parameters and response structures
- **OpenAPI Spec**: Embedded OpenAPI specification for runtime validation

### Build Process
- Integrated with Moon build system via `moon.yml`
- Build task removes old generated code and regenerates from the latest API spec
- Depends on the `api-openapi:build` task to ensure the spec is up-to-date

## Usage

To regenerate the OpenAPI code:
```bash
moon run server-openapi:build
```

The generated client can be used by other Go services to interact with the VibeThis API:
```go
client, err := openapi.NewClient("http://localhost:8080")
// Use client to make type-safe API calls
```

## Important Notes
- Generated code should **never** be manually edited
- All API contract changes must be made in the source OpenAPI spec
- The generated package provides both low-level HTTP client and high-level response-wrapped client interfaces
- Supports custom request editors for authentication, headers, etc.