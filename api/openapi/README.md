# VibeThis API OpenAPI Specification

This directory contains the OpenAPI specification for the VibeThis API.

## Files

- `colony2-api.yaml` - The complete OpenAPI 3.0.3 specification for all API endpoints

## Overview

The VibeThis API provides the following functionality:

### Graph Operations
- Get the complete dependency graph of all nodes (boxes)

### Position Management
- Save and retrieve visual positions of nodes in the UI

### File Operations
- List files in node directories
- Read and write file contents

### Git Integration
- Get repository status
- View diffs and commit history
- Create commits with optional file staging

### Container Management
- Create, start, stop, and restart DevContainers
- Get container status
- Reset containers

## Using the Specification

You can use this OpenAPI specification with various tools:

1. **API Documentation**: Import into Swagger UI or ReDoc for interactive documentation
2. **Client Generation**: Generate API clients in various languages using OpenAPI Generator
3. **API Testing**: Use with Postman or Insomnia by importing the specification
4. **Mock Server**: Create a mock server using Prism or similar tools

## Validation

To validate the OpenAPI specification:

```bash
# Using openapi-generator-cli
openapi-generator-cli validate -i colony2-api.yaml

# Using swagger-cli
swagger-cli validate colony2-api.yaml
```