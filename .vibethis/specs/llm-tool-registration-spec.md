# LLM Tool Registration System Specification

## Overview
This specification defines a system for registering and exposing tools to LLM agents within recipe workflows, enabling them to interact with the codebase, execute commands, and perform development tasks.

## Architecture

### 1. Tool Registry System

#### Core Components
```
Tool Registry
├── Tool Definitions
│   ├── Bash Tools
│   ├── Git Operations
│   ├── File Operations
│   ├── Code Analysis
│   └── Custom Tools
├── Tool Adapter
│   ├── OpenAI Function Calling
│   ├── Anthropic Tool Use
│   └── Generic JSON Schema
├── Permission Manager
│   ├── Tool Permissions
│   ├── Path Restrictions
│   └── Command Allowlists
└── Execution Engine
    ├── Tool Invocation
    ├── Result Processing
    └── Error Handling
```

### 2. Tool Definition Format

#### Standard Tool Schema
```yaml
apiVersion: v1
kind: Tool
metadata:
  name: bash_execute
  version: 1.0.0
  category: system
  
spec:
  description: Execute bash commands in the workspace
  
  parameters:
    type: object
    properties:
      command:
        type: string
        description: The bash command to execute
      workingDirectory:
        type: string
        description: Working directory for command execution
        default: "{{.WORKSPACE_PATH}}"
      timeout:
        type: integer
        description: Command timeout in seconds
        default: 30
    required: ["command"]
  
  permissions:
    requireApproval: false
    allowedPaths:
      - "{{.WORKSPACE_PATH}}"
      - "{{.SPEC_PATH}}"
    deniedCommands:
      - "rm -rf /"
      - "sudo"
      - "chmod 777"
    maxExecutionTime: 60
  
  implementation:
    type: native
    handler: BashExecuteHandler
```

### 3. Built-in Tool Definitions

#### Bash Execution Tool
```go
type BashTool struct {
    BaseDir         string
    AllowedCommands []string
    DeniedCommands  []string
    Timeout         time.Duration
}

func (t *BashTool) Schema() ToolSchema {
    return ToolSchema{
        Name:        "bash",
        Description: "Execute bash commands in the cell workspace",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "command": {
                    "type": "string",
                    "description": "Bash command to execute"
                },
                "cwd": {
                    "type": "string", 
                    "description": "Working directory"
                }
            },
            "required": ["command"]
        }`),
    }
}

func (t *BashTool) Execute(params map[string]interface{}) (interface{}, error) {
    command := params["command"].(string)
    
    // Security validation
    if err := t.validateCommand(command); err != nil {
        return nil, err
    }
    
    // Execute with timeout
    ctx, cancel := context.WithTimeout(context.Background(), t.Timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "bash", "-c", command)
    cmd.Dir = t.BaseDir
    
    output, err := cmd.CombinedOutput()
    return map[string]interface{}{
        "output": string(output),
        "exitCode": cmd.ProcessState.ExitCode(),
    }, err
}
```

#### File Operations Tool
```go
type FileTool struct {
    WorkspacePath string
    ReadOnly      bool
}

func (t *FileTool) Schema() ToolSchema {
    return ToolSchema{
        Name:        "file",
        Description: "Read, write, and manipulate files",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "operation": {
                    "type": "string",
                    "enum": ["read", "write", "append", "delete", "exists", "list"],
                    "description": "File operation to perform"
                },
                "path": {
                    "type": "string",
                    "description": "File path relative to workspace"
                },
                "content": {
                    "type": "string",
                    "description": "Content for write/append operations"
                }
            },
            "required": ["operation", "path"]
        }`),
    }
}

func (t *FileTool) Execute(params map[string]interface{}) (interface{}, error) {
    operation := params["operation"].(string)
    path := filepath.Join(t.WorkspacePath, params["path"].(string))
    
    // Validate path is within workspace
    if !t.isPathAllowed(path) {
        return nil, fmt.Errorf("path outside workspace: %s", path)
    }
    
    switch operation {
    case "read":
        content, err := os.ReadFile(path)
        return map[string]interface{}{"content": string(content)}, err
        
    case "write":
        if t.ReadOnly {
            return nil, fmt.Errorf("write operation not allowed in read-only mode")
        }
        content := params["content"].(string)
        return nil, os.WriteFile(path, []byte(content), 0644)
        
    case "list":
        entries, err := os.ReadDir(filepath.Dir(path))
        if err != nil {
            return nil, err
        }
        files := []string{}
        for _, e := range entries {
            files = append(files, e.Name())
        }
        return map[string]interface{}{"files": files}, nil
        
    default:
        return nil, fmt.Errorf("unknown operation: %s", operation)
    }
}
```

#### Git Operations Tool
```go
type GitTool struct {
    RepoPath    string
    GitPackPath string
}

func (t *GitTool) Schema() ToolSchema {
    return ToolSchema{
        Name:        "git",
        Description: "Perform git operations",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "operation": {
                    "type": "string",
                    "enum": ["status", "diff", "add", "commit", "branch", "checkout", "log"],
                    "description": "Git operation"
                },
                "args": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Additional arguments"
                }
            },
            "required": ["operation"]
        }`),
    }
}

func (t *GitTool) Execute(params map[string]interface{}) (interface{}, error) {
    operation := params["operation"].(string)
    args := []string{operation}
    
    if argsParam, ok := params["args"].([]interface{}); ok {
        for _, arg := range argsParam {
            args = append(args, arg.(string))
        }
    }
    
    cmd := exec.Command("git", args...)
    cmd.Dir = t.RepoPath
    
    output, err := cmd.CombinedOutput()
    return map[string]interface{}{
        "output": string(output),
        "success": err == nil,
    }, nil
}
```

### 4. Tool Registration and Discovery

#### Dynamic Tool Registration
```go
type ToolRegistry struct {
    tools       map[string]Tool
    permissions PermissionManager
    mu          sync.RWMutex
}

func (r *ToolRegistry) Register(tool Tool) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    // Validate tool schema
    if err := r.validateSchema(tool.Schema()); err != nil {
        return err
    }
    
    r.tools[tool.Schema().Name] = tool
    return nil
}

func (r *ToolRegistry) GetToolsForLLM(format string) interface{} {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    switch format {
    case "openai":
        return r.formatOpenAITools()
    case "anthropic":
        return r.formatAnthropicTools()
    default:
        return r.formatGenericTools()
    }
}

func (r *ToolRegistry) formatOpenAITools() []map[string]interface{} {
    tools := []map[string]interface{}{}
    
    for _, tool := range r.tools {
        schema := tool.Schema()
        tools = append(tools, map[string]interface{}{
            "type": "function",
            "function": map[string]interface{}{
                "name":        schema.Name,
                "description": schema.Description,
                "parameters":  schema.Parameters,
            },
        })
    }
    
    return tools
}
```

### 5. LLM Integration

#### Tool Executor for LLM Calls
```go
type LLMToolExecutor struct {
    registry    *ToolRegistry
    permissions *PermissionManager
    logger      *Logger
}

func (e *LLMToolExecutor) ExecuteToolCall(call ToolCall) (interface{}, error) {
    // Get tool from registry
    tool, exists := e.registry.GetTool(call.Name)
    if !exists {
        return nil, fmt.Errorf("tool not found: %s", call.Name)
    }
    
    // Check permissions
    if err := e.permissions.CheckPermission(call); err != nil {
        return nil, fmt.Errorf("permission denied: %v", err)
    }
    
    // Log the tool call
    e.logger.LogToolCall(call)
    
    // Execute with timeout and resource limits
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    resultChan := make(chan interface{})
    errChan := make(chan error)
    
    go func() {
        result, err := tool.Execute(call.Parameters)
        if err != nil {
            errChan <- err
        } else {
            resultChan <- result
        }
    }()
    
    select {
    case result := <-resultChan:
        e.logger.LogToolResult(call, result)
        return result, nil
    case err := <-errChan:
        e.logger.LogToolError(call, err)
        return nil, err
    case <-ctx.Done():
        return nil, fmt.Errorf("tool execution timeout")
    }
}
```

#### Recipe Activity with Tool Support
```go
type LLMActivity struct {
    llmClient     LLMClient
    toolExecutor  *LLMToolExecutor
    toolRegistry  *ToolRegistry
}

func (a *LLMActivity) ProcessWithTools(ctx context.Context, prompt string) (string, error) {
    // Get available tools
    tools := a.toolRegistry.GetToolsForLLM("openai")
    
    // Create LLM request with tools
    request := LLMRequest{
        Model:    "gpt-4",
        Messages: []Message{{Role: "user", Content: prompt}},
        Tools:    tools,
    }
    
    // Process LLM response with tool calls
    for {
        response, err := a.llmClient.Complete(request)
        if err != nil {
            return "", err
        }
        
        // Check if LLM wants to use tools
        if len(response.ToolCalls) == 0 {
            return response.Content, nil
        }
        
        // Execute tool calls
        toolResults := []ToolResult{}
        for _, call := range response.ToolCalls {
            result, err := a.toolExecutor.ExecuteToolCall(call)
            toolResults = append(toolResults, ToolResult{
                CallID: call.ID,
                Result: result,
                Error:  err,
            })
        }
        
        // Add tool results to conversation
        request.Messages = append(request.Messages, Message{
            Role:        "assistant",
            Content:     response.Content,
            ToolCalls:   response.ToolCalls,
        })
        request.Messages = append(request.Messages, Message{
            Role:        "tool",
            ToolResults: toolResults,
        })
    }
}
```

### 6. Permission Management

#### Permission System
```go
type PermissionManager struct {
    rules       []PermissionRule
    auditLog    *AuditLogger
}

type PermissionRule struct {
    ToolPattern    string
    AllowedPaths   []string
    DeniedPaths    []string
    AllowedCommands []string
    DeniedCommands  []string
    RequireApproval bool
    MaxExecutions   int
    TimeWindow      time.Duration
}

func (p *PermissionManager) CheckPermission(call ToolCall) error {
    for _, rule := range p.rules {
        if matched, _ := filepath.Match(rule.ToolPattern, call.Name); matched {
            // Check path restrictions
            if path, ok := call.Parameters["path"].(string); ok {
                if !p.isPathAllowed(path, rule) {
                    return fmt.Errorf("path not allowed: %s", path)
                }
            }
            
            // Check command restrictions
            if cmd, ok := call.Parameters["command"].(string); ok {
                if !p.isCommandAllowed(cmd, rule) {
                    return fmt.Errorf("command not allowed: %s", cmd)
                }
            }
            
            // Check rate limits
            if !p.checkRateLimit(call, rule) {
                return fmt.Errorf("rate limit exceeded")
            }
            
            // Check if approval needed
            if rule.RequireApproval {
                if !p.hasApproval(call) {
                    return fmt.Errorf("approval required")
                }
            }
        }
    }
    
    return nil
}
```

### 7. Tool Configuration

#### Global Tool Configuration
```yaml
# /recipes/global/tools.yaml
tools:
  bash:
    enabled: true
    timeout: 30
    allowedCommands:
      - "ls"
      - "cat"
      - "grep"
      - "find"
      - "npm"
      - "go"
      - "python"
    deniedCommands:
      - "sudo"
      - "rm -rf"
      - "shutdown"
    workingDirectory: "{{.WORKSPACE_PATH}}"
    
  file:
    enabled: true
    allowedPaths:
      - "{{.WORKSPACE_PATH}}"
      - "{{.SPEC_PATH}}"
    maxFileSize: 10MB
    
  git:
    enabled: true
    allowedOperations:
      - status
      - diff
      - add
      - commit
      - branch
      - log
    deniedOperations:
      - push
      - force
      
  codeAnalysis:
    enabled: true
    languages:
      - go
      - python
      - javascript
      - typescript
```

#### Cell-Specific Tool Override
```yaml
# /cells/${CELL_ID}/.colony2/tools.yaml
tools:
  bash:
    additionalCommands:
      - "docker"
      - "kubectl"
      
  custom:
    - name: deploymentTool
      enabled: true
      script: /cells/${CELL_ID}/scripts/deploy.sh
```

### 8. Tool Usage Examples

#### In Recipe Workflow
```python
class CodeWorkflow:
    def __init__(self, tool_executor):
        self.tools = tool_executor
        
    async def implement_feature(self, spec):
        # Use LLM with tools to implement
        prompt = f"""
        Implement this feature based on the spec:
        {spec}
        
        You have access to:
        - bash: execute commands
        - file: read/write files
        - git: version control
        - codeAnalysis: analyze code structure
        
        Please implement the feature step by step.
        """
        
        result = await self.llm_with_tools(prompt)
        
        # The LLM will use tools like:
        # 1. codeAnalysis to understand structure
        # 2. file.read to examine existing code
        # 3. file.write to create/modify files
        # 4. bash to run tests
        # 5. git to commit changes
        
        return result
```

### 9. Security Considerations

#### Sandboxing
- Tools execute within container boundaries
- Network access controlled per tool
- File system access restricted to workspace
- Resource limits enforced (CPU, memory, disk)

#### Audit Logging
```go
type ToolAuditLog struct {
    Timestamp   time.Time
    CellID      string
    RecipeType  string
    ToolName    string
    Parameters  map[string]interface{}
    Result      interface{}
    Error       error
    ExecutionMs int64
}
```

### 10. Monitoring and Metrics

#### Tool Usage Metrics
- Tool invocation count per recipe
- Tool execution time distribution
- Tool error rates
- Most frequently used tools
- Resource consumption per tool

#### Health Checks
```go
func (r *ToolRegistry) HealthCheck() map[string]bool {
    health := make(map[string]bool)
    
    for name, tool := range r.tools {
        // Test each tool with a simple operation
        testParams := tool.GetTestParameters()
        _, err := tool.Execute(testParams)
        health[name] = err == nil
    }
    
    return health
}
```