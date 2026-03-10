package codex

import (
	"bytes"
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
	worktreeTarget, err := opts.containerPath(opts.WorktreeRoot)
	if err != nil {
		return fmt.Errorf("resolve worktree container path: %w", err)
	}
	codexHomeInboxTarget, err := opts.containerPath(opts.codexHomeInboxPath())
	if err != nil {
		return fmt.Errorf("resolve codex home inbox path: %w", err)
	}
	skillDirs, err := discoverMergedSkillDirs(opts.WorktreeRoot, opts.ConfiguredSkillDirs)
	if err != nil {
		return err
	}
	skillDirTargets := make([]string, 0, len(skillDirs))
	for _, dir := range skillDirs {
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
		buildCodexHomeRootCommand(codexHomeTarget, codexHomeInboxTarget, hostCodexHomeMountTarget),
		buildWorktreeC2SkillsRootCommand(codexHomeTarget, worktreeTarget, skillDirTargets),
	}

	cfg := &shai.SandboxConfig{
		WorkingDir:     opts.WorkDirRoot,
		ReadWritePaths: []string{cellRelFromWorkdir},
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
	if err := removeSensitiveCodexFiles(opts.CodexHome); err != nil {
		return err
	}
	return pruneUnchangedCodexFiles(opts.codexHomeInboxPath(), opts.CodexHome)
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

func pruneUnchangedCodexFiles(sourceDir string, targetDir string) error {
	sourceDir = filepath.Clean(sourceDir)
	targetDir = filepath.Clean(targetDir)
	if sourceDir == targetDir {
		return nil
	}

	targetInfo, err := os.Stat(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat codex target dir %q: %w", targetDir, err)
	}
	if !targetInfo.IsDir() {
		return nil
	}

	if err := filepath.WalkDir(targetDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(targetDir, path)
		if err != nil {
			return err
		}
		sourcePath := filepath.Join(sourceDir, rel)
		equal, err := regularFilesEqual(sourcePath, path)
		if err != nil {
			return err
		}
		if !equal {
			return nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("prune unchanged codex files: %w", err)
	}

	if err := removeEmptyDirs(targetDir); err != nil {
		return fmt.Errorf("remove empty codex directories: %w", err)
	}
	return nil
}

func regularFilesEqual(a string, b string) (bool, error) {
	aInfo, err := os.Stat(a)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !aInfo.Mode().IsRegular() || !bInfo.Mode().IsRegular() {
		return false, nil
	}
	if aInfo.Size() != bInfo.Size() {
		return false, nil
	}
	aFile, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer aFile.Close()
	bFile, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer bFile.Close()

	const chunkSize = 32 * 1024
	aBuf := make([]byte, chunkSize)
	bBuf := make([]byte, chunkSize)

	for {
		aN, aErr := aFile.Read(aBuf)
		bN, bErr := bFile.Read(bBuf)
		if aN != bN {
			return false, nil
		}
		if aN > 0 && !bytes.Equal(aBuf[:aN], bBuf[:bN]) {
			return false, nil
		}
		if aErr == io.EOF && bErr == io.EOF {
			return true, nil
		}
		if aErr != nil && aErr != io.EOF {
			return false, aErr
		}
		if bErr != nil && bErr != io.EOF {
			return false, bErr
		}
	}
}

func removeEmptyDirs(root string) error {
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return err
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		if filepath.Clean(dir) == filepath.Clean(root) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func buildCodexHomeRootCommand(codexHomeTarget string, inboxCodexHomeTarget string, hostCodexHomeTarget string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "mkdir -p %s\n", shellQuote(codexHomeTarget))
	fmt.Fprintf(&builder, "if [ -d %s ]; then cp -a %s/. %s/; fi\n",
		shellQuote(inboxCodexHomeTarget),
		shellQuote(inboxCodexHomeTarget),
		shellQuote(codexHomeTarget),
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
	if err := copyDirContentsIfExists(opts.codexHomeInboxPath(), opts.CodexHome); err != nil {
		return err
	}
	if err := copyConfiguredAndWorktreeSkillsIfExists(opts); err != nil {
		return err
	}
	if err := seedCodexCredentialsIfNeeded(opts.HostCodexHome, opts.CodexHome); err != nil {
		return err
	}
	return nil
}

func copyConfiguredAndWorktreeSkillsIfExists(opts Options) error {
	skillDirs, err := discoverMergedSkillDirs(opts.WorktreeRoot, opts.ConfiguredSkillDirs)
	if err != nil {
		return err
	}
	targetDir := filepath.Join(opts.CodexHome, "skills")
	for _, sourceDir := range skillDirs {
		if err := copyDirContentsIfExists(sourceDir, targetDir); err != nil {
			return err
		}
	}
	return nil
}

func copyWorktreeC2SkillsIfExists(opts Options) error {
	skillDirs, err := discoverWorktreeC2SkillDirs(opts.WorktreeRoot)
	if err != nil {
		return fmt.Errorf("discover worktree c2 skill dirs: %w", err)
	}
	targetDir := filepath.Join(opts.CodexHome, "skills")
	for _, sourceDir := range skillDirs {
		if err := copyDirContentsIfExists(sourceDir, targetDir); err != nil {
			return err
		}
	}
	return nil
}

func discoverMergedSkillDirs(worktreeRoot string, configuredSkillDirs []string) ([]string, error) {
	dirs := make([]string, 0)
	seen := map[string]struct{}{}
	appendUnique := func(path string) {
		path = filepath.Clean(path)
		if strings.TrimSpace(path) == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		dirs = append(dirs, path)
	}

	for _, dir := range configuredSkillDirs {
		appendUnique(dir)
	}

	worktreeSkillDirs, err := discoverWorktreeC2SkillDirs(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("discover worktree c2 skill dirs: %w", err)
	}
	for _, dir := range worktreeSkillDirs {
		appendUnique(dir)
	}

	return dirs, nil
}

func buildWorktreeC2SkillsRootCommand(codexHomeTarget string, worktreeTarget string, skillDirTargets []string) string {
	var builder strings.Builder
	codexHomeSkillsTarget := filepath.ToSlash(filepath.Join(codexHomeTarget, "skills"))
	fmt.Fprintf(&builder, "if [ -d %s ]; then\n", shellQuote(worktreeTarget))
	fmt.Fprintf(&builder, "  mkdir -p %s\n", shellQuote(codexHomeSkillsTarget))
	for _, dirTarget := range skillDirTargets {
		fmt.Fprintf(&builder, "  if [ -d %s ]; then cp -a %s/. %s/; fi\n",
			shellQuote(dirTarget),
			shellQuote(dirTarget),
			shellQuote(codexHomeSkillsTarget),
		)
	}
	builder.WriteString("fi")
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
