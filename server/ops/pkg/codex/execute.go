package codex

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/colony-2/shai/pkg/shai"
)

const hostCodexHomeMountTarget = "/run/codex-host-home"

var codexCredentialFileNames = []string{
	"auth.json",
	"config.toml",
}

// Execute runs Codex in non-interactive mode and returns the normalized result.
// The caller is responsible for managing the returned stdoutPath, stderrPath, and artifactDir.
func Execute(ctx context.Context, opts Options) (Result, string, string, string, error) {
	if err := opts.validate(); err != nil {
		return Result{}, "", "", "", err
	}
	if err := ensureExecutionPaths(opts); err != nil {
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
	if sanitizeErr := sanitizeCodexHomeOutput(opts); sanitizeErr != nil {
		if runErr == nil {
			runErr = fmt.Errorf("sanitize codex home output: %w", sanitizeErr)
		} else {
			runErr = fmt.Errorf("%v; sanitize codex home output: %w", runErr, sanitizeErr)
		}
	}
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
		if err := prepareDirectCodexHome(opts); err != nil {
			return fmt.Errorf("prepare codex home: %w", err)
		}
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

	cellRelFromWorkdir, err := opts.relativeToWorkdir(filepath.Join(opts.WorktreeRoot, opts.CellRelativePath))
	if err != nil {
		return fmt.Errorf("resolve cell mount path: %w", err)
	}
	codexHomeRelFromWorkdir, err := opts.relativeToWorkdir(opts.CodexHome)
	if err != nil {
		return fmt.Errorf("resolve codex home workdir path: %w", err)
	}
	inboxRelFromWorkdir, err := opts.relativeToWorkdir(opts.ArtifactInbox)
	if err != nil {
		return fmt.Errorf("resolve inbox mount path: %w", err)
	}
	outboxRelFromWorkdir, err := opts.relativeToWorkdir(opts.ArtifactOutbox)
	if err != nil {
		return fmt.Errorf("resolve outbox mount path: %w", err)
	}
	inboxTarget, err := opts.containerPath(opts.ArtifactInbox)
	if err != nil {
		return fmt.Errorf("resolve inbox container path: %w", err)
	}
	outboxTarget, err := opts.containerPath(opts.ArtifactOutbox)
	if err != nil {
		return fmt.Errorf("resolve outbox container path: %w", err)
	}
	codexHomeTarget, err := opts.containerPath(opts.CodexHome)
	if err != nil {
		return fmt.Errorf("resolve codex home container path: %w", err)
	}
	codexSessionsInboxTarget, err := opts.containerPath(opts.codexSessionsInboxPath())
	if err != nil {
		return fmt.Errorf("resolve codex sessions inbox path: %w", err)
	}
	codexSessionsOutboxTarget, err := opts.containerPath(opts.codexSessionsOutboxPath())
	if err != nil {
		return fmt.Errorf("resolve codex sessions outbox path: %w", err)
	}
	skillDirTargets := make([]string, 0, len(opts.ConfiguredSkillDirs))
	for _, dir := range opts.ConfiguredSkillDirs {
		target, targetErr := opts.containerPath(dir)
		if targetErr != nil {
			return fmt.Errorf("resolve skill dir container path: %w", targetErr)
		}
		skillDirTargets = append(skillDirTargets, target)
	}

	command := buildCommand(opts, schemaPath)
	env := buildEnv(opts)
	env["CODEX_HOME"] = codexHomeTarget

	mounts := []shai.Mount{
		{
			Source: inboxRelFromWorkdir,
			Target: inboxTarget,
			Mode:   "ro",
		},
		{
			Source: outboxRelFromWorkdir,
			Target: outboxTarget,
			Mode:   "rw",
		},
	}
	if hostCodexHomeMountable(opts.HostCodexHome) {
		mounts = append(mounts, shai.Mount{
			Source: opts.HostCodexHome,
			Target: hostCodexHomeMountTarget,
			Mode:   "ro",
		})
	}
	rootCommands := []string{
		buildSchemaRootCommand(schemaPath, schemaPayload),
		buildCodexHomeRootCommand(codexHomeTarget, codexSessionsInboxTarget, codexSessionsOutboxTarget, hostCodexHomeMountTarget),
		buildConfiguredSkillsRootCommand(codexHomeTarget, skillDirTargets),
	}

	cfg := &shai.SandboxConfig{
		WorkingDir:     opts.WorkDirRoot,
		ReadWritePaths: []string{cellRelFromWorkdir, codexHomeRelFromWorkdir},
		PrependResourceSet: &shai.ResourceSet{
			Mounts:       mounts,
			RootCommands: rootCommands,
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

func ensureExecutionPaths(opts Options) error {
	cellPath := filepath.Join(opts.WorktreeRoot, opts.CellRelativePath)
	if err := os.MkdirAll(cellPath, 0o755); err != nil {
		return fmt.Errorf("create cell path: %w", err)
	}
	if err := os.MkdirAll(opts.ArtifactInbox, 0o755); err != nil {
		return fmt.Errorf("create inbox path: %w", err)
	}
	if err := os.MkdirAll(opts.ArtifactOutbox, 0o755); err != nil {
		return fmt.Errorf("create outbox path: %w", err)
	}
	if err := os.MkdirAll(opts.CodexHome, 0o755); err != nil {
		return fmt.Errorf("create codex home path: %w", err)
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

func sanitizeCodexHomeOutput(opts Options) error {
	return removeSensitiveCodexFiles(opts.CodexHome)
}

func removeSensitiveCodexFiles(codexHomeDir string) error {
	for _, name := range codexCredentialFileNames {
		targetPath := filepath.Join(codexHomeDir, name)
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove sensitive codex file %q: %w", targetPath, err)
		}
	}
	return nil
}

func buildCodexHomeRootCommand(codexHomeTarget string, inboxSessionsTarget string, outboxSessionsTarget string, hostCodexHomeTarget string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "mkdir -p %s\n", shellQuote(codexHomeTarget))
	fmt.Fprintf(&builder, "find %s -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null || true\n", shellQuote(codexHomeTarget))
	fmt.Fprintf(&builder, "mkdir -p %s\n", shellQuote(outboxSessionsTarget))
	fmt.Fprintf(&builder, "find %s -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null || true\n", shellQuote(outboxSessionsTarget))
	fmt.Fprintf(&builder, "if [ -d %s ]; then cp -a %s/. %s/; fi\n",
		shellQuote(inboxSessionsTarget),
		shellQuote(inboxSessionsTarget),
		shellQuote(outboxSessionsTarget),
	)
	fmt.Fprintf(&builder, "mkdir -p %s\n", shellQuote(filepath.ToSlash(filepath.Join(codexHomeTarget, ".agents"))))
	fmt.Fprintf(&builder, "ln -s %s %s\n",
		shellQuote(outboxSessionsTarget),
		shellQuote(filepath.ToSlash(filepath.Join(codexHomeTarget, "sessions"))),
	)
	fmt.Fprintf(&builder, "if [ -d %s ]; then\n", shellQuote(hostCodexHomeTarget))
	for _, name := range codexCredentialFileNames {
		sourcePath := filepath.ToSlash(filepath.Join(hostCodexHomeTarget, name))
		targetPath := filepath.ToSlash(filepath.Join(codexHomeTarget, name))
		fmt.Fprintf(&builder, "  if [ -f %s ] && [ ! -f %s ]; then cp %s %s; fi\n",
			shellQuote(sourcePath),
			shellQuote(targetPath),
			shellQuote(sourcePath),
			shellQuote(targetPath),
		)
	}
	builder.WriteString("fi")
	return builder.String()
}

func hostCodexHomeMountable(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func prepareDirectCodexHome(opts Options) error {
	if err := resetCodexHomeContents(opts.CodexHome); err != nil {
		return err
	}
	if err := bootstrapCodexSessions(opts.codexSessionsInboxPath(), opts.codexSessionsOutboxPath()); err != nil {
		return err
	}
	if err := linkCodexHomeSessions(opts.CodexHome, opts.codexSessionsOutboxPath()); err != nil {
		return err
	}
	if err := installConfiguredSkillsIfExists(opts); err != nil {
		return err
	}
	if err := seedCodexCredentialsIfNeeded(opts.HostCodexHome, opts.CodexHome); err != nil {
		return err
	}
	return nil
}

func installConfiguredSkillsIfExists(opts Options) error {
	targetDir := opts.codexAgentsSkillsPath()
	for _, sourceDir := range opts.ConfiguredSkillDirs {
		if err := copyDirContentsIfExists(sourceDir, targetDir); err != nil {
			return err
		}
	}
	return nil
}

func resetCodexHomeContents(codexHomeDir string) error {
	if err := os.MkdirAll(codexHomeDir, 0o755); err != nil {
		return fmt.Errorf("create codex home %q: %w", codexHomeDir, err)
	}
	entries, err := os.ReadDir(codexHomeDir)
	if err != nil {
		return fmt.Errorf("read codex home %q: %w", codexHomeDir, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(codexHomeDir, entry.Name())); err != nil {
			return fmt.Errorf("reset codex home entry %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func bootstrapCodexSessions(inboxSessionsDir string, outboxSessionsDir string) error {
	if err := os.MkdirAll(outboxSessionsDir, 0o755); err != nil {
		return fmt.Errorf("create codex sessions outbox %q: %w", outboxSessionsDir, err)
	}
	entries, err := os.ReadDir(outboxSessionsDir)
	if err != nil {
		return fmt.Errorf("read codex sessions outbox %q: %w", outboxSessionsDir, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(outboxSessionsDir, entry.Name())); err != nil {
			return fmt.Errorf("reset codex sessions entry %q: %w", entry.Name(), err)
		}
	}
	if err := copyDirContentsIfExists(inboxSessionsDir, outboxSessionsDir); err != nil {
		return fmt.Errorf("seed codex sessions from inbox: %w", err)
	}
	return nil
}

func linkCodexHomeSessions(codexHomeDir string, outboxSessionsDir string) error {
	agentsDir := filepath.Join(codexHomeDir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("create codex agents dir %q: %w", agentsDir, err)
	}
	sessionsLink := filepath.Join(codexHomeDir, "sessions")
	if err := os.RemoveAll(sessionsLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove codex sessions link %q: %w", sessionsLink, err)
	}
	if err := os.Symlink(outboxSessionsDir, sessionsLink); err != nil {
		return fmt.Errorf("link codex sessions %q -> %q: %w", sessionsLink, outboxSessionsDir, err)
	}
	return nil
}

func buildConfiguredSkillsRootCommand(codexHomeTarget string, skillDirTargets []string) string {
	var builder strings.Builder
	codexHomeSkillsTarget := filepath.ToSlash(filepath.Join(codexHomeTarget, ".agents", "skills"))
	fmt.Fprintf(&builder, "mkdir -p %s\n", shellQuote(codexHomeSkillsTarget))
	for _, dirTarget := range skillDirTargets {
		fmt.Fprintf(&builder, "if [ -d %s ]; then cp -a %s/. %s/; fi\n",
			shellQuote(dirTarget),
			shellQuote(dirTarget),
			shellQuote(codexHomeSkillsTarget),
		)
	}
	return builder.String()
}

func copyDirContentsIfExists(sourceDir string, targetDir string) error {
	if filepath.Clean(sourceDir) == filepath.Clean(targetDir) {
		return nil
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat inbox codex home %q: %w", sourceDir, err)
	}
	if !info.IsDir() {
		return nil
	}
	return copyDirContents(sourceDir, targetDir)
}

func copyDirContents(sourceDir string, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create target directory %q: %w", targetDir, err)
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", sourceDir, err)
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat source path %q: %w", sourcePath, err)
		}

		if info.IsDir() {
			if err := copyDirContents(sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}

		if !info.Mode().IsRegular() {
			continue
		}
		if err := copyRegularFile(sourcePath, targetPath, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func seedCodexCredentialsIfNeeded(sourceHomeDir string, targetHomeDir string) error {
	sourceHomeDir = strings.TrimSpace(sourceHomeDir)
	if sourceHomeDir == "" {
		return nil
	}
	info, err := os.Stat(sourceHomeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat host codex home %q: %w", sourceHomeDir, err)
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(targetHomeDir, 0o755); err != nil {
		return fmt.Errorf("create codex home %q: %w", targetHomeDir, err)
	}

	for _, name := range codexCredentialFileNames {
		sourcePath := filepath.Join(sourceHomeDir, name)
		targetPath := filepath.Join(targetHomeDir, name)
		if _, err := os.Stat(targetPath); err == nil {
			continue
		}

		sourceInfo, err := os.Stat(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat credential source %q: %w", sourcePath, err)
		}
		if !sourceInfo.Mode().IsRegular() {
			continue
		}
		if err := copyRegularFile(sourcePath, targetPath, sourceInfo.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyRegularFile(sourcePath string, targetPath string, mode os.FileMode) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", sourcePath, err)
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create target parent %q: %w", filepath.Dir(targetPath), err)
	}
	if mode == 0 {
		mode = 0o644
	}
	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("open target file %q: %w", targetPath, err)
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return fmt.Errorf("copy %q to %q: %w", sourcePath, targetPath, err)
	}
	return nil
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
