package codex

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/colony-2/shai/pkg/shai"
)

// Execute runs Codex in non-interactive mode and returns the normalized result.
// The caller is responsible for managing the returned stdoutPath, stderrPath, and tempDir.
// If creating artifacts, use the cleanup callback to remove tempDir.
// If not creating artifacts, caller must clean up tempDir immediately.
func Execute(ctx context.Context, opts Options) (Result, string, string, string, error) {
	if err := opts.validate(); err != nil {
		return Result{}, "", "", "", err
	}

	tempDir, err := opts.tempDir()
	if err != nil {
		return Result{}, "", "", "", err
	}

	schemaPath := opts.schemaPath(tempDir)
	if err := os.WriteFile(schemaPath, opts.StructuredSchema, 0o644); err != nil {
		return Result{}, "", "", "", fmt.Errorf("write schema: %w", err)
	}

	stdoutPath := opts.stdoutPath(tempDir)
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return Result{}, "", "", "", fmt.Errorf("create stdout capture: %w", err)
	}

	stderrPath := opts.stderrPath(tempDir)
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		return Result{}, "", "", "", fmt.Errorf("create stderr capture: %w", err)
	}

	collector := newOutputCollector(stdoutFile, stderrFile)

	containerSchemaPath, err := opts.containerPath(schemaPath)
	if err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return Result{}, "", "", "", err
	}

	command := buildCommand(opts, containerSchemaPath)
	env := buildEnv(opts)

	cfg := &shai.SandboxConfig{
		WorkingDir:     opts.WorktreeRoot,
		ReadWritePaths: []string{opts.CellRelativePath},
		PostSetupExec: &shai.SandboxExec{
			Command: command,
			Env:     env,
			UseTTY:  false,
		},
		Stdout: collector.stdoutWriter(),
		Stderr: collector.stderrWriter(),
	}

	runner, err := opts.RunnerFactory(cfg)
	if err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return Result{}, "", "", "", fmt.Errorf("create runner: %w", err)
	}
	defer runner.Close()

	runErr := runner.Run(ctx)
	if closeErr := stdoutFile.Close(); closeErr != nil {
		if runErr == nil {
			runErr = fmt.Errorf("close stdout capture: %w", closeErr)
		}
	}
	if closeErr := stderrFile.Close(); closeErr != nil {
		if runErr == nil {
			runErr = fmt.Errorf("close stderr capture: %w", closeErr)
		}
	}

	parseRes, parseErr := parseJSONL(stdoutPath)
	if parseErr != nil {
		return Result{}, "", "", "", parseErr
	}

	result := buildResultFromParse(opts, parseRes)

	if runErr != nil {
		result.Status = StatusError
		if result.ErrorMessage == "" {
			result.ErrorMessage = runErr.Error()
		} else {
			result.ErrorMessage = fmt.Sprintf("%s; %v", result.ErrorMessage, runErr)
		}
	}

	if result.SessionID == "" {
		if parseRes.sessionID != "" {
			result.SessionID = parseRes.sessionID
		} else if opts.SessionID != "" {
			result.SessionID = opts.SessionID
		}
	}

	return result, stdoutPath, stderrPath, tempDir, nil
}

func buildResultFromParse(opts Options, outcome parseOutcome) Result {
	res := Result{}

	if outcome.payload == nil {
		res.Status = StatusError
		res.ErrorMessage = fallbackErrorMessage(outcome)
		return res
	}

	switch strings.ToLower(outcome.payload.Status) {
	case string(StatusCompleted):
		res.Status = StatusCompleted
	case string(StatusIncomplete):
		res.Status = StatusIncomplete
	case string(StatusError):
		res.Status = StatusError
	default:
		res.Status = StatusError
		res.ErrorMessage = fmt.Sprintf("unknown status %q", outcome.payload.Status)
	}

	res.SessionID = outcome.sessionID
	res.AssistantSummary = outcome.payload.AssistantSummary
	res.IncompleteReason = outcome.payload.IncompleteReason
	res.IncompleteCategory = outcome.payload.IncompleteCategory
	res.PendingDependencies = outcome.payload.PendingDeps
	if len(res.PendingDependencies) == 0 && res.Status == StatusIncomplete && res.IncompleteCategory == "" {
		res.IncompleteCategory = "agent_abandoned"
	}
	if outcome.payload.ErrorMessage != "" {
		res.ErrorMessage = outcome.payload.ErrorMessage
	} else if outcome.failureMessage != "" && res.Status == StatusError {
		res.ErrorMessage = outcome.failureMessage
	}
	return res
}

func fallbackErrorMessage(outcome parseOutcome) string {
	if outcome.failureMessage != "" {
		return outcome.failureMessage
	}
	return "codex did not produce structured output"
}
