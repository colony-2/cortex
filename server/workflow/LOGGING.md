# Workflow Service Logging

## Overview

This document describes the structured logging implementation in the workflow service using Go's standard `log/slog` package. The logging provides comprehensive visibility into all strata client operations, SWF engine operations, and workflow data flows.

## Why Structured Logging?

We use `log/slog` for several key benefits:

1. **Structured data**: Key-value pairs make logs machine-readable and easily searchable
2. **Standard library**: No external dependencies, future-proof
3. **Performance**: Efficient with minimal overhead
4. **Flexibility**: Easy to configure different output formats (JSON, text) and log levels
5. **Context preservation**: Includes relevant IDs and metadata for tracing operations

## Changes Made

### Service Configuration

The `Service` struct now includes a logger instance:

```go
type Config struct {
    Engine   swf.SWFEngine
    Strata   *client.Client
    Tickets  ticket.Service
    Cells    cell.Service
    Projects project.Service
    Logger   *slog.Logger  // New field
}

type Service struct {
    engine   swf.SWFEngine
    strata   *client.Client
    tickets  ticket.Service
    cells    cell.Service
    projects project.Service
    logger   *slog.Logger  // New field
}
```

The `New()` constructor accepts an optional logger and defaults to `slog.Default()` if none is provided:

```go
func New(cfg Config) (*Service, error) {
    logger := cfg.Logger
    if logger == nil {
        logger = slog.Default()
    }
    return &Service{
        // ... other fields ...
        logger: logger,
    }, nil
}
```

## Logging Patterns

### Log Levels

We use three primary log levels:

- **Debug**: Detailed operational information for troubleshooting
- **Warn**: Warning conditions that don't prevent operation
- **Error**: Error conditions that caused operation failure

### Key Naming Conventions

Consistent key names across the service:

- `project_id`: Project identifier
- `workflow_id`: Workflow/job identifier
- `tenant_id`: Tenant ID from SWF engine
- `job_id`: Job ID from SWF engine
- `chapter_number`: Chapter ordinal in a workflow
- `artifact_name`: Name of an artifact
- `artifact_id`: Unique artifact identifier
- `error`: Error details

### SWF Engine Operations

All `s.engine.ListJobs()` calls are logged:

```go
s.logger.Debug("GetWorkflow: querying engine for job",
    "project_id", req.ProjectID,
    "workflow_id", req.WorkflowID)

resp, err := s.engine.ListJobs(ctx, swf.ListJobsRequest{...})

if err != nil {
    s.logger.Error("GetWorkflow: engine query failed",
        "project_id", req.ProjectID,
        "workflow_id", req.WorkflowID,
        "error", err)
    return nil, err
}

s.logger.Debug("GetWorkflow: found workflow in engine",
    "project_id", req.ProjectID,
    "workflow_id", req.WorkflowID,
    "status", resp.Jobs[0].Status)
```

**Pattern**: Log before the operation (Debug), log errors (Error), log successful results (Debug).

### Strata Client Operations

All strata operations include comprehensive logging:

#### Loading Chapters

```go
s.logger.Debug("loadStartJob: loading chapter 0",
    "tenant_id", jobKey.TenantId,
    "job_id", jobKey.JobId)

chap, err := s.strata.Chapter(ctx, jobKey.ToStoryKey(), 0)

if err != nil {
    s.logger.Error("loadStartJob: failed to load chapter from strata",
        "tenant_id", jobKey.TenantId,
        "job_id", jobKey.JobId,
        "error", err)
    return nil, err
}
```

#### Loading Stories

```go
s.logger.Debug("loadChapters: loading story from strata",
    "tenant_id", jobKey.TenantId,
    "job_id", jobKey.JobId)

storyHandle, err := s.strata.Story(ctx, jobKey.ToStoryKey())

if err != nil {
    s.logger.Error("loadChapters: failed to load story from strata",
        "tenant_id", jobKey.TenantId,
        "job_id", jobKey.JobId,
        "error", err)
    return nil, err
}
```

#### Iterating Chapters

```go
s.logger.Debug("loadChapters: creating chapters iterator",
    "tenant_id", jobKey.TenantId,
    "job_id", jobKey.JobId,
    "page_size", 100)

iter, err := storyHandle.Chapters(ctx, story.ChaptersOptions{...})

if err != nil {
    s.logger.Error("loadChapters: failed to create chapters iterator",
        "tenant_id", jobKey.TenantId,
        "job_id", jobKey.JobId,
        "error", err)
    return nil, err
}

// After successful iteration
s.logger.Debug("loadChapters: successfully loaded all chapters",
    "tenant_id", jobKey.TenantId,
    "job_id", jobKey.JobId,
    "chapter_count", len(chapters))
```

#### Loading Artifacts

```go
s.logger.Debug("GetWorkflowArtifact: loading artifact bytes from strata",
    "tenant_id", jobKey.TenantId,
    "job_id", jobKey.JobId,
    "chapter_number", req.ChapterNumber,
    "artifact_name", req.ArtifactName,
    "artifact_id", art.ID())

content, err := art.Bytes(ctx)

if err != nil {
    s.logger.Error("GetWorkflowArtifact: failed to read artifact content from strata",
        "tenant_id", jobKey.TenantId,
        "job_id", jobKey.JobId,
        "chapter_number", req.ChapterNumber,
        "artifact_name", req.ArtifactName,
        "artifact_id", art.ID(),
        "error", err)
    return nil, fmt.Errorf("failed to read artifact content: %w", err)
}

s.logger.Debug("GetWorkflowArtifact: successfully loaded artifact",
    "tenant_id", jobKey.TenantId,
    "job_id", jobKey.JobId,
    "chapter_number", req.ChapterNumber,
    "artifact_name", req.ArtifactName,
    "size_bytes", len(content))
```

**Pattern**: Log intent → Perform operation → Log error or success with results.

## Usage

### Providing a Custom Logger

When initializing the workflow service, you can provide a custom logger:

```go
// Create a JSON logger for production
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))

service, err := workflow.New(workflow.Config{
    Engine: engine,
    Strata: strataClient,
    Logger: logger,
})
```

### Using the Default Logger

If no logger is provided, the service uses `slog.Default()`:

```go
service, err := workflow.New(workflow.Config{
    Engine: engine,
    Strata: strataClient,
    // Logger will default to slog.Default()
})
```

### Configuring Log Levels

Set the minimum log level based on environment:

```go
// Development: See all debug logs
handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})

// Production: Info and above only
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})

logger := slog.New(handler)
```

## Debugging with Logs

### Finding a Specific Workflow

```bash
# Search logs for a specific workflow ID
grep "workflow_id=wf_123" service.log

# JSON logs (use jq)
grep "wf_123" service.log | jq 'select(.workflow_id == "wf_123")'
```

### Tracing a Request Flow

All operations include `tenant_id` and `job_id` for tracing:

```bash
# Trace all operations for a specific job
grep "job_id=job_abc123" service.log

# See just strata operations
grep "strata" service.log | grep "job_id=job_abc123"
```

### Identifying Errors

```bash
# Find all errors
grep "level=ERROR" service.log

# Find strata-related errors
grep "level=ERROR" service.log | grep "strata"

# Count error types
grep "level=ERROR" service.log | grep -o "failed to [^\"]*" | sort | uniq -c
```

## Best Practices

1. **Always include context**: Include relevant IDs (workflow_id, job_id, etc.) in every log entry
2. **Use consistent key names**: Follow the naming conventions documented above
3. **Log at appropriate levels**:
   - Debug: Detailed flow information
   - Info: Significant events (not currently used, but available)
   - Warn: Unusual but handled conditions
   - Error: Operation failures
4. **Include error details**: Always include the error in the attributes when logging errors
5. **Log before and after**: Log intent before operations, results after
6. **Use structured data**: Always use key-value pairs, never string interpolation
7. **Avoid PII**: Don't log sensitive user data (passwords, tokens, etc.)

## Migration Notes

This implementation replaced all `fmt.Printf` debug statements with structured slog calls. The old debug prints were:

```go
// OLD
fmt.Printf("DEBUG ListWorkflows: PGWF returned %d jobs for project %s (statuses=%v)\n",
    len(resp.Jobs), req.ProjectID, jobStatuses)

// NEW
s.logger.Debug("ListWorkflows: engine returned jobs",
    "job_count", len(resp.Jobs),
    "project_id", req.ProjectID,
    "statuses", jobStatuses)
```

Benefits of the new approach:
- Machine-readable output
- Consistent formatting
- Configurable output format (text vs JSON)
- Configurable log levels
- Better performance in production

## Future Enhancements

Potential improvements to consider:

1. **Add trace IDs**: Include request trace IDs for distributed tracing
2. **Metrics integration**: Add duration logging for performance monitoring
3. **Contextual loggers**: Create child loggers with pre-populated context
4. **Log sampling**: Sample high-volume debug logs in production
5. **Integration with observability platforms**: Send logs to centralized logging systems
