# Command Execution Activity Specification

## Overview
A registerable activity that executes arbitrary shell commands with GitHub Actions-style configuration patterns.

## Activity Type
`command_execution`

## Configuration (CommandExecutionConfig)
```go
type CommandExecutionConfig struct {
    WorkingDir string            `json:"working_dir"` // Optional: working directory
    Shell      string            `json:"shell"`       // Optional: shell to use (defaults to system default)
    Env        map[string]string `json:"env"`         // Optional: environment variables
}
```

## Input (CommandExecutionInput)
Following GitHub Actions patterns:
```go
type CommandExecutionInput struct {
    Run              string            `json:"run"`               // Required: command to execute
    WorkingDirectory string            `json:"working_directory"` // Optional: override working directory
    Shell            string            `json:"shell"`             // Optional: override shell
    Env              map[string]string `json:"env"`               // Optional: additional env vars
    ContinueOnError  bool              `json:"continue_on_error"` // Optional: don't fail on non-zero exit
    Timeout          string            `json:"timeout"`           // Optional: timeout duration (e.g., "30s")
}
```

## Output (CommandExecutionOutput)
```go
type CommandExecutionOutput struct {
    Stdout       string `json:"stdout"`        // Standard output
    Stderr       string `json:"stderr"`        // Standard error
    ExitCode     int    `json:"exit_code"`     // Process exit code
    Success      bool   `json:"success"`       // Whether command succeeded
    TimedOut     bool   `json:"timed_out"`     // Whether command timed out
    ErrorMessage string `json:"error_message"` // Error message if failed
}
```

## Features
1. **Shell Selection**: Support for bash, sh, powershell, cmd
2. **Environment Variables**: Merge config env with input env
3. **Working Directory**: Support both config and input-level working directories
4. **Timeout Support**: Parse duration strings like "30s", "5m"
5. **Error Handling**: Capture exit codes, stderr, and system errors
6. **Continue on Error**: Option to not fail the activity on non-zero exit codes

## Security Considerations
- No shell injection protection (by design - this executes arbitrary commands)
- Should only be used in trusted environments
- Consider adding allowlist/denylist for commands in production

## Example Usage
```yaml
- type: command_execution
  config:
    working_dir: /tmp
    shell: bash
  inputs:
    run: |
      echo "Hello from command execution"
      ls -la
    env:
      MY_VAR: "value"
    timeout: "30s"
```