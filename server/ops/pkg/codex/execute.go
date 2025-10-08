package codex

import (
	"context"
	"fmt"
	"os"
	"strings"

	shai "github.com/divisive-ai/vibethis/server/container/pkg/shai"
	"github.com/google/uuid"
)

// Execute runs Codex in non-interactive mode and returns the normalized result.
func Execute(ctx context.Context, opts Options) (Result, error) {
	if err := opts.validate(); err != nil {
		return Result{}, err
	}

	tempDir, err := opts.tempDir()
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tempDir)

	schemaPath := opts.schemaPath(tempDir)
	if err := os.WriteFile(schemaPath, opts.StructuredSchema, 0o644); err != nil {
		return Result{}, fmt.Errorf("write schema: %w", err)
	}

	stdoutPath := opts.stdoutPath(tempDir)
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return Result{}, fmt.Errorf("create stdout capture: %w", err)
	}

	collector := newOutputCollector(stdoutFile)

	containerSchemaPath, err := opts.containerPath(schemaPath)
	if err != nil {
		stdoutFile.Close()
		return Result{}, err
	}

	command := buildCommand(opts, containerSchemaPath)
	env := buildEnv(opts)

	cfg := &shai.EphemeralConfig{
		WorkingDir:          opts.WorktreeRoot,
		ReadWritePaths:      []string{opts.CellRelativePath},
		HideProgressMarkers: true,
		PostSetupExec: &shai.ExecSpec{
			Command: command,
			Env:     env,
			UseTTY:  false,
		},
		Output: collector,
	}

	runner, err := opts.RunnerFactory(cfg)
	if err != nil {
		stdoutFile.Close()
		return Result{}, fmt.Errorf("create runner: %w", err)
	}
	defer runner.Close()

	runErr := runner.Run(ctx)
	if closeErr := stdoutFile.Close(); closeErr != nil {
		if runErr == nil {
			runErr = fmt.Errorf("close stdout capture: %w", closeErr)
		}
	}

	parseRes, parseErr := parseJSONL(stdoutPath)
	if parseErr != nil {
		return Result{}, parseErr
	}

	result := buildResultFromParse(opts, parseRes, collector.stderrString())

	if runErr != nil {
		result.Status = StatusError
		if result.ErrorMessage == "" {
			result.ErrorMessage = runErr.Error()
		} else {
			result.ErrorMessage = fmt.Sprintf("%s; %v", result.ErrorMessage, runErr)
		}
	}

	stdoutID := uuid.NewString()
	timestamp := opts.Clock.Now().Format("20060102T150405Z")
	relPath := opts.stdoutRelativePath(timestamp, stdoutID)
	if _, err := opts.BlobStore.Put(ctx, opts.BlobstoreURI, relPath, stdoutPath); err != nil {
		return Result{}, fmt.Errorf("store stdout: %w", err)
	}
	result.StdoutBlobURI = joinBlobURI(opts.BlobstoreURI, relPath)

	if result.SessionID == "" {
		if parseRes.sessionID != "" {
			result.SessionID = parseRes.sessionID
		} else if opts.SessionID != "" {
			result.SessionID = opts.SessionID
		}
	}

	return result, nil
}

func buildResultFromParse(opts Options, outcome parseOutcome, stderr string) Result {
	res := Result{
		Stderr: stderr,
	}

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
