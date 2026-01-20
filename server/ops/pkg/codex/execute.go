package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/colony-2/shai/pkg/shai"
)

// Execute runs Codex in non-interactive mode and returns the normalized result.
// The caller is responsible for managing the returned stdoutPath, stderrPath, and artifactDir.
func Execute(ctx context.Context, opts Options) (Result, string, string, string, error) {
	if err := opts.validate(); err != nil {
		return Result{}, "", "", "", err
	}
	if err := ensureCellPath(opts); err != nil {
		return Result{}, "", "", "", err
	}

	artifactDir, err := os.MkdirTemp("", "codex-artifacts-*")
	if err != nil {
		return Result{}, "", "", "", fmt.Errorf("create artifacts dir: %w", err)
	}
	var schemaPath string
	var schemaCleanup func()
	if useDirectCodex() {
		var schemaErr error
		schemaPath, schemaCleanup, schemaErr = writeSchemaTempFile(opts.StructuredSchema)
		if schemaErr != nil {
			_ = os.RemoveAll(artifactDir)
			return Result{}, "", "", "", schemaErr
		}
		defer schemaCleanup()
	} else {
		schemaPath = containerSchemaPath(opts)
	}

	stdoutPath := opts.stdoutPath(artifactDir)
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		_ = os.RemoveAll(artifactDir)
		return Result{}, "", "", "", fmt.Errorf("create stdout capture: %w", err)
	}
	debugArtifactStat("stdout-create", stdoutPath)

	stderrPath := opts.stderrPath(artifactDir)
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		_ = os.RemoveAll(artifactDir)
		return Result{}, "", "", "", fmt.Errorf("create stderr capture: %w", err)
	}
	debugArtifactStat("stderr-create", stderrPath)

	collector := newOutputCollector(stdoutFile, stderrFile)

	runErr := runCodexExec(ctx, opts, schemaPath, opts.StructuredSchema, collector)
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

	return result, stdoutPath, stderrPath, artifactDir, nil
}

func runCodexExec(ctx context.Context, opts Options, schemaPath string, schemaPayload []byte, collector *outputCollector) error {
	if useDirectCodex() {
		cmdArgs := buildCommand(opts, schemaPath)
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Env = os.Environ()
		for k, v := range buildEnv(opts) {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Dir = opts.WorktreeRoot
		cmd.Stdout = collector.stdoutWriter()
		cmd.Stderr = collector.stderrWriter()
		return cmd.Run()
	}

	command := buildCommand(opts, schemaPath)
	env := buildEnv(opts)

	cfg := &shai.SandboxConfig{
		WorkingDir:     opts.WorktreeRoot,
		ReadWritePaths: []string{opts.CellRelativePath},
		PrependResourceSet: &shai.ResourceSet{
			RootCommands: []string{buildSchemaRootCommand(schemaPath, schemaPayload)},
		},
		PostSetupExec: &shai.SandboxExec{
			Command: command,
			Env:     env,
			UseTTY:  false,
		},
		Stdout:           collector.stdoutWriter(),
		Stderr:           collector.stderrWriter(),
		ShowScriptOutput: false,
	}

	runner, err := opts.RunnerFactory(cfg)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	defer runner.Close()

	return runner.Run(ctx)
}

func writeSchemaTempFile(schema []byte) (string, func(), error) {
	file, err := os.CreateTemp("", "codex-schema-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("create schema temp file: %w", err)
	}
	if _, err := file.Write(schema); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("write schema: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("close schema file: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	return file.Name(), cleanup, nil
}

func containerSchemaPath(opts Options) string {
	ts := opts.Clock.Now().UTC().UnixNano()
	name := fmt.Sprintf("codex-schema-%d.json", ts)
	return filepath.ToSlash(filepath.Join("/tmp", name))
}

func ensureCellPath(opts Options) error {
	cellPath := filepath.Join(opts.WorktreeRoot, opts.CellRelativePath)
	if err := os.MkdirAll(cellPath, 0o755); err != nil {
		return fmt.Errorf("create cell path: %w", err)
	}
	return nil
}

func buildSchemaRootCommand(schemaPath string, schemaPayload []byte) string {
	delimiter := chooseHeredocDelimiter(schemaPayload)
	var builder strings.Builder
	fmt.Fprintf(&builder, "cat <<'%s' > %s\n", delimiter, shellQuote(schemaPath))
	builder.Write(schemaPayload)
	if len(schemaPayload) == 0 || schemaPayload[len(schemaPayload)-1] != '\n' {
		builder.WriteByte('\n')
	}
	fmt.Fprintf(&builder, "%s", delimiter)
	return builder.String()
}

func chooseHeredocDelimiter(schemaPayload []byte) string {
	base := "CODEX_SCHEMA_EOF"
	delimiter := base
	for i := 0; strings.Contains(string(schemaPayload), delimiter); i++ {
		delimiter = fmt.Sprintf("%s_%d", base, i+1)
	}
	return delimiter
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func useDirectCodex() bool {
	val := strings.TrimSpace(os.Getenv("VIBETHIS_CODEX_USE_DIRECT"))
	return val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "yes")
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
