# Bash Operations and Tools Specification

## Overview
This specification defines the bash command execution system and related shell operations that recipes can use to interact with the development environment, run builds, execute tests, and manage the workspace.

## Architecture

### 1. Bash Execution Service

#### Core Components
```
Bash Service
├── Command Executor
│   ├── Shell Manager
│   ├── Process Controller
│   ├── Output Streaming
│   └── Signal Handling
├── Security Layer
│   ├── Command Validator
│   ├── Path Sandboxing
│   ├── Resource Limiter
│   └── Allowlist/Denylist
├── Session Manager
│   ├── Persistent Sessions
│   ├── Environment Variables
│   ├── Working Directory
│   └── History Tracking
└── Integration Layer
    ├── LLM Tool Adapter
    ├── Recipe Activity
    ├── Temporal Activity
    └── Result Formatter
```

### 2. Command Execution Models

#### Single Command Execution
```go
type BashExecutor struct {
    workDir     string
    env         map[string]string
    timeout     time.Duration
    validator   *CommandValidator
    limiter     *ResourceLimiter
}

func (e *BashExecutor) Execute(command string, opts ...ExecOption) (*CommandResult, error) {
    // Apply options
    config := e.defaultConfig()
    for _, opt := range opts {
        opt(&config)
    }
    
    // Validate command
    if err := e.validator.Validate(command, config); err != nil {
        return nil, fmt.Errorf("command validation failed: %w", err)
    }
    
    // Create command with context
    ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "bash", "-c", command)
    cmd.Dir = config.WorkDir
    cmd.Env = e.buildEnv(config.Env)
    
    // Setup resource limits
    if err := e.limiter.Apply(cmd); err != nil {
        return nil, err
    }
    
    // Capture output
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    // Execute
    start := time.Now()
    err := cmd.Run()
    duration := time.Since(start)
    
    return &CommandResult{
        Command:  command,
        Stdout:   stdout.String(),
        Stderr:   stderr.String(),
        ExitCode: cmd.ProcessState.ExitCode(),
        Duration: duration,
        Error:    err,
    }, nil
}
```

#### Persistent Shell Session
```go
type ShellSession struct {
    id          string
    shell       *exec.Cmd
    stdin       io.WriteCloser
    stdout      *bufio.Reader
    stderr      *bufio.Reader
    workDir     string
    env         map[string]string
    history     []string
    mu          sync.Mutex
}

func NewShellSession(workDir string, env map[string]string) (*ShellSession, error) {
    cmd := exec.Command("bash", "-i")
    cmd.Dir = workDir
    cmd.Env = append(os.Environ(), formatEnv(env)...)
    
    stdin, _ := cmd.StdinPipe()
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()
    
    if err := cmd.Start(); err != nil {
        return nil, err
    }
    
    return &ShellSession{
        id:      generateID(),
        shell:   cmd,
        stdin:   stdin,
        stdout:  bufio.NewReader(stdout),
        stderr:  bufio.NewReader(stderr),
        workDir: workDir,
        env:     env,
        history: []string{},
    }, nil
}

func (s *ShellSession) Execute(command string) (*CommandResult, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Add to history
    s.history = append(s.history, command)
    
    // Send command
    marker := fmt.Sprintf("__MARKER_%d__", time.Now().UnixNano())
    fullCommand := fmt.Sprintf("%s; echo '%s' $?", command, marker)
    
    if _, err := s.stdin.Write([]byte(fullCommand + "\n")); err != nil {
        return nil, err
    }
    
    // Read output until marker
    var output strings.Builder
    for {
        line, err := s.stdout.ReadString('\n')
        if err != nil {
            return nil, err
        }
        
        if strings.Contains(line, marker) {
            // Extract exit code
            parts := strings.Split(line, marker)
            if len(parts) > 1 {
                exitCode, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
                return &CommandResult{
                    Command:  command,
                    Stdout:   output.String(),
                    ExitCode: exitCode,
                }, nil
            }
            break
        }
        
        output.WriteString(line)
    }
    
    return &CommandResult{
        Command: command,
        Stdout:  output.String(),
    }, nil
}
```

### 3. Command Validation and Security

#### Command Validator
```go
type CommandValidator struct {
    allowedCommands  []string
    deniedCommands   []string
    allowedPaths     []string
    deniedPaths      []string
    dangerousPatterns []string
}

func (v *CommandValidator) Validate(command string, config ExecConfig) error {
    // Check against denied commands
    for _, denied := range v.deniedCommands {
        if strings.Contains(command, denied) {
            return fmt.Errorf("command contains denied pattern: %s", denied)
        }
    }
    
    // Check for dangerous patterns
    for _, pattern := range v.dangerousPatterns {
        if matched, _ := regexp.MatchString(pattern, command); matched {
            return fmt.Errorf("command matches dangerous pattern: %s", pattern)
        }
    }
    
    // Parse command for path validation
    paths := v.extractPaths(command)
    for _, path := range paths {
        if !v.isPathAllowed(path, config.WorkDir) {
            return fmt.Errorf("path not allowed: %s", path)
        }
    }
    
    // Check command allowlist if configured
    if len(v.allowedCommands) > 0 {
        baseCmd := v.extractBaseCommand(command)
        if !v.isCommandAllowed(baseCmd) {
            return fmt.Errorf("command not in allowlist: %s", baseCmd)
        }
    }
    
    return nil
}

func DefaultValidator() *CommandValidator {
    return &CommandValidator{
        deniedCommands: []string{
            "rm -rf /",
            "sudo",
            "su",
            "shutdown",
            "reboot",
            "mkfs",
            "dd if=/dev/zero",
        },
        dangerousPatterns: []string{
            `>\s*/dev/s`,           // Writing to device files
            `chmod\s+777`,          // Overly permissive permissions
            `curl.*\|\s*bash`,      // Piping curl to bash
            `wget.*\|\s*sh`,        // Piping wget to shell
        },
    }
}
```

#### Resource Limiter
```go
type ResourceLimiter struct {
    maxCPU      float64  // CPU cores
    maxMemory   int64    // Bytes
    maxDisk     int64    // Bytes
    maxProcs    int      // Process count
    maxFileSize int64    // Bytes
}

func (l *ResourceLimiter) Apply(cmd *exec.Cmd) error {
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true,
    }
    
    // Set resource limits using ulimit wrapper
    wrapper := fmt.Sprintf(
        "ulimit -v %d -f %d -u %d; %s",
        l.maxMemory/1024,  // Memory in KB
        l.maxFileSize/512, // File size in 512-byte blocks
        l.maxProcs,
        cmd.Args[len(cmd.Args)-1],
    )
    
    cmd.Args[len(cmd.Args)-1] = wrapper
    
    return nil
}
```

### 4. Specialized Bash Operations

#### Build Operations
```go
type BuildOperations struct {
    executor *BashExecutor
}

func (b *BuildOperations) DetectBuildSystem(workDir string) (string, error) {
    // Check for common build files
    checks := map[string]string{
        "package.json":  "npm",
        "go.mod":        "go",
        "Cargo.toml":    "cargo",
        "pom.xml":       "maven",
        "build.gradle":  "gradle",
        "Makefile":      "make",
        "CMakeLists.txt": "cmake",
    }
    
    for file, system := range checks {
        if exists(filepath.Join(workDir, file)) {
            return system, nil
        }
    }
    
    return "", fmt.Errorf("no build system detected")
}

func (b *BuildOperations) Build(workDir string) (*CommandResult, error) {
    system, err := b.DetectBuildSystem(workDir)
    if err != nil {
        return nil, err
    }
    
    commands := map[string]string{
        "npm":    "npm run build",
        "go":     "go build ./...",
        "cargo":  "cargo build",
        "maven":  "mvn compile",
        "gradle": "gradle build",
        "make":   "make",
        "cmake":  "cmake --build .",
    }
    
    cmd, ok := commands[system]
    if !ok {
        return nil, fmt.Errorf("unknown build system: %s", system)
    }
    
    return b.executor.Execute(cmd, WithWorkDir(workDir))
}
```

#### Test Operations
```go
type TestOperations struct {
    executor *BashExecutor
}

func (t *TestOperations) RunTests(workDir string, pattern string) (*TestResult, error) {
    system, _ := DetectTestFramework(workDir)
    
    var cmd string
    switch system {
    case "jest":
        cmd = fmt.Sprintf("npm test -- %s", pattern)
    case "pytest":
        cmd = fmt.Sprintf("pytest %s", pattern)
    case "go":
        cmd = fmt.Sprintf("go test %s ./...", pattern)
    case "cargo":
        cmd = fmt.Sprintf("cargo test %s", pattern)
    default:
        cmd = "npm test"
    }
    
    result, err := t.executor.Execute(cmd, WithWorkDir(workDir))
    if err != nil {
        return nil, err
    }
    
    return t.parseTestOutput(result, system), nil
}

func (t *TestOperations) parseTestOutput(result *CommandResult, framework string) *TestResult {
    // Parse test output based on framework
    parser := GetTestParser(framework)
    return parser.Parse(result.Stdout)
}
```

#### Dependency Management
```go
type DependencyOperations struct {
    executor *BashExecutor
}

func (d *DependencyOperations) InstallDependencies(workDir string) error {
    system, err := d.DetectPackageManager(workDir)
    if err != nil {
        return err
    }
    
    commands := map[string]string{
        "npm":     "npm install",
        "yarn":    "yarn install",
        "pnpm":    "pnpm install",
        "pip":     "pip install -r requirements.txt",
        "poetry":  "poetry install",
        "go":      "go mod download",
        "cargo":   "cargo fetch",
        "composer": "composer install",
    }
    
    cmd, ok := commands[system]
    if !ok {
        return fmt.Errorf("unknown package manager: %s", system)
    }
    
    _, err = d.executor.Execute(cmd, 
        WithWorkDir(workDir),
        WithTimeout(5*time.Minute),
    )
    return err
}

func (d *DependencyOperations) AddDependency(workDir, pkg string) error {
    system, _ := d.DetectPackageManager(workDir)
    
    commands := map[string]string{
        "npm":    fmt.Sprintf("npm install %s", pkg),
        "yarn":   fmt.Sprintf("yarn add %s", pkg),
        "go":     fmt.Sprintf("go get %s", pkg),
        "pip":    fmt.Sprintf("pip install %s", pkg),
        "cargo":  fmt.Sprintf("cargo add %s", pkg),
    }
    
    cmd, ok := commands[system]
    if !ok {
        return fmt.Errorf("unknown package manager: %s", system)
    }
    
    _, err := d.executor.Execute(cmd, WithWorkDir(workDir))
    return err
}
```

### 5. Temporal Activity Integration

#### Bash Activity for Recipes
```go
type BashActivity struct {
    executor  *BashExecutor
    sessions  map[string]*ShellSession
    mu        sync.RWMutex
}

func (a *BashActivity) ExecuteCommand(ctx context.Context, req BashRequest) (*BashResponse, error) {
    activity.GetLogger(ctx).Info("Executing bash command", 
        "command", req.Command,
        "workDir", req.WorkDir,
    )
    
    // Set up execution options
    opts := []ExecOption{
        WithWorkDir(req.WorkDir),
        WithTimeout(time.Duration(req.TimeoutSeconds) * time.Second),
        WithEnv(req.Environment),
    }
    
    // Execute command
    result, err := a.executor.Execute(req.Command, opts...)
    if err != nil && !req.AllowFailure {
        return nil, err
    }
    
    return &BashResponse{
        Stdout:   result.Stdout,
        Stderr:   result.Stderr,
        ExitCode: result.ExitCode,
        Duration: result.Duration,
    }, nil
}

func (a *BashActivity) CreateSession(ctx context.Context, req SessionRequest) (*SessionResponse, error) {
    session, err := NewShellSession(req.WorkDir, req.Environment)
    if err != nil {
        return nil, err
    }
    
    a.mu.Lock()
    a.sessions[session.id] = session
    a.mu.Unlock()
    
    return &SessionResponse{
        SessionID: session.id,
    }, nil
}

func (a *BashActivity) ExecuteInSession(ctx context.Context, req SessionExecRequest) (*BashResponse, error) {
    a.mu.RLock()
    session, ok := a.sessions[req.SessionID]
    a.mu.RUnlock()
    
    if !ok {
        return nil, fmt.Errorf("session not found: %s", req.SessionID)
    }
    
    result, err := session.Execute(req.Command)
    if err != nil {
        return nil, err
    }
    
    return &BashResponse{
        Stdout:   result.Stdout,
        ExitCode: result.ExitCode,
    }, nil
}
```

### 6. LLM Tool Integration

#### Bash Tool for LLM
```go
type BashLLMTool struct {
    executor    *BashExecutor
    operations  *BashOperations
}

func (t *BashLLMTool) GetSchema() map[string]interface{} {
    return map[string]interface{}{
        "name": "bash",
        "description": "Execute bash commands and scripts",
        "parameters": map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "command": map[string]interface{}{
                    "type":        "string",
                    "description": "The bash command to execute",
                },
                "operation": map[string]interface{}{
                    "type":        "string",
                    "enum":        []string{"execute", "build", "test", "install"},
                    "description": "Type of operation",
                },
                "workDir": map[string]interface{}{
                    "type":        "string",
                    "description": "Working directory",
                },
                "timeout": map[string]interface{}{
                    "type":        "integer",
                    "description": "Timeout in seconds",
                    "default":     30,
                },
            },
            "required": []string{"command"},
        },
    }
}

func (t *BashLLMTool) Execute(params map[string]interface{}) (interface{}, error) {
    operation := params["operation"].(string)
    
    switch operation {
    case "build":
        return t.operations.Build.Build(params["workDir"].(string))
        
    case "test":
        pattern := ""
        if p, ok := params["pattern"].(string); ok {
            pattern = p
        }
        return t.operations.Test.RunTests(params["workDir"].(string), pattern)
        
    case "install":
        return nil, t.operations.Deps.InstallDependencies(params["workDir"].(string))
        
    default:
        command := params["command"].(string)
        workDir := params["workDir"].(string)
        timeout := 30
        if t, ok := params["timeout"].(int); ok {
            timeout = t
        }
        
        return t.executor.Execute(command,
            WithWorkDir(workDir),
            WithTimeout(time.Duration(timeout)*time.Second),
        )
    }
}
```

### 7. Configuration

#### Global Bash Configuration
```yaml
# /recipes/global/bash.yaml
bash:
  defaults:
    timeout: 30s
    workDir: "{{.WORKSPACE_PATH}}"
    shell: "/bin/bash"
    
  security:
    enableValidation: true
    allowedCommands:
      - ls
      - cat
      - grep
      - find
      - npm
      - go
      - python
      - make
      - docker
      - git
      
    deniedCommands:
      - sudo
      - su
      - rm -rf /
      - shutdown
      - reboot
      
    allowedPaths:
      - "{{.WORKSPACE_PATH}}"
      - "{{.SPEC_PATH}}"
      - "/tmp"
      
  resources:
    maxCPU: 2.0
    maxMemory: 2GB
    maxDisk: 10GB
    maxProcesses: 100
    maxFileSize: 100MB
    
  operations:
    build:
      timeout: 5m
      retries: 2
      
    test:
      timeout: 10m
      parallel: true
      
    install:
      timeout: 5m
      cache: true
```

### 8. Output Handling

#### Stream Processing
```go
type OutputStreamer struct {
    maxBufferSize int
    filters       []OutputFilter
}

func (s *OutputStreamer) Stream(reader io.Reader, callback func(line string)) error {
    scanner := bufio.NewScanner(reader)
    scanner.Buffer(make([]byte, s.maxBufferSize), s.maxBufferSize)
    
    for scanner.Scan() {
        line := scanner.Text()
        
        // Apply filters
        for _, filter := range s.filters {
            line = filter.Apply(line)
        }
        
        callback(line)
    }
    
    return scanner.Err()
}

type OutputFilter interface {
    Apply(line string) string
}

type AnsiStripFilter struct{}

func (f *AnsiStripFilter) Apply(line string) string {
    // Strip ANSI color codes
    re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
    return re.ReplaceAllString(line, "")
}

type SecretMaskFilter struct {
    secrets []string
}

func (f *SecretMaskFilter) Apply(line string) string {
    result := line
    for _, secret := range f.secrets {
        result = strings.ReplaceAll(result, secret, "***")
    }
    return result
}
```

### 9. Error Handling

#### Error Types and Recovery
```go
type BashError struct {
    Command  string
    ExitCode int
    Stderr   string
    Timeout  bool
}

func (e *BashError) Error() string {
    if e.Timeout {
        return fmt.Sprintf("command timeout: %s", e.Command)
    }
    return fmt.Sprintf("command failed (exit %d): %s\nstderr: %s", 
        e.ExitCode, e.Command, e.Stderr)
}

type ErrorHandler struct {
    retryable map[int]bool
}

func (h *ErrorHandler) ShouldRetry(err error) bool {
    if bashErr, ok := err.(*BashError); ok {
        // Timeout errors are retryable
        if bashErr.Timeout {
            return true
        }
        
        // Check exit code
        return h.retryable[bashErr.ExitCode]
    }
    
    return false
}

func DefaultErrorHandler() *ErrorHandler {
    return &ErrorHandler{
        retryable: map[int]bool{
            124: true,  // Timeout
            137: true,  // SIGKILL (OOM)
            139: true,  // Segfault
        },
    }
}
```

### 10. Monitoring and Metrics

#### Execution Metrics
```go
type BashMetrics struct {
    CommandCount      int64
    SuccessCount      int64
    FailureCount      int64
    TimeoutCount      int64
    TotalDuration     time.Duration
    CommandDurations  map[string][]time.Duration
}

func (m *BashMetrics) Record(result *CommandResult) {
    atomic.AddInt64(&m.CommandCount, 1)
    
    if result.ExitCode == 0 {
        atomic.AddInt64(&m.SuccessCount, 1)
    } else {
        atomic.AddInt64(&m.FailureCount, 1)
    }
    
    // Record duration
    baseCmd := extractBaseCommand(result.Command)
    m.CommandDurations[baseCmd] = append(
        m.CommandDurations[baseCmd], 
        result.Duration,
    )
}
```