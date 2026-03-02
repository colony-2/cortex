package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeCodexHomeOutputRemovesSensitiveAndUnchanged(t *testing.T) {
	workdir := t.TempDir()
	inbox := filepath.Join(workdir, "inbox")
	outbox := filepath.Join(workdir, "outbox")
	inboxCodexHome := filepath.Join(inbox, codexHomeArtifactDirName)
	outboxCodexHome := filepath.Join(outbox, codexHomeArtifactDirName)
	require.NoError(t, os.MkdirAll(inboxCodexHome, 0o755))
	require.NoError(t, os.MkdirAll(outboxCodexHome, 0o755))

	write := func(path string, value string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(value), 0o644))
	}

	write(filepath.Join(inboxCodexHome, "auth.json"), "inbox-auth")
	write(filepath.Join(inboxCodexHome, "config.toml"), "inbox-config")
	write(filepath.Join(inboxCodexHome, "unchanged.txt"), "same")
	write(filepath.Join(inboxCodexHome, "nested", "same.txt"), "same-nested")
	write(filepath.Join(inboxCodexHome, "changed.txt"), "before")

	write(filepath.Join(outboxCodexHome, "auth.json"), "host-auth")
	write(filepath.Join(outboxCodexHome, "config.toml"), "host-config")
	write(filepath.Join(outboxCodexHome, "unchanged.txt"), "same")
	write(filepath.Join(outboxCodexHome, "nested", "same.txt"), "same-nested")
	write(filepath.Join(outboxCodexHome, "changed.txt"), "after")
	write(filepath.Join(outboxCodexHome, "new.txt"), "new")

	opts := Options{
		Prompt:        "test",
		WorkDirRoot:   workdir,
		WorktreeRoot:  filepath.Join(workdir, "worktree"),
		ArtifactInbox: inbox,
		CodexHome:     outboxCodexHome,
	}

	require.NoError(t, sanitizeCodexHomeOutput(opts))

	require.NoFileExists(t, filepath.Join(outboxCodexHome, "auth.json"))
	require.NoFileExists(t, filepath.Join(outboxCodexHome, "config.toml"))
	require.NoFileExists(t, filepath.Join(outboxCodexHome, "unchanged.txt"))
	require.NoFileExists(t, filepath.Join(outboxCodexHome, "nested", "same.txt"))
	require.NoDirExists(t, filepath.Join(outboxCodexHome, "nested"))

	require.FileExists(t, filepath.Join(outboxCodexHome, "changed.txt"))
	require.FileExists(t, filepath.Join(outboxCodexHome, "new.txt"))
}

func TestSanitizeCodexHomeOutputWithoutInboxKeepsNonSensitiveFiles(t *testing.T) {
	workdir := t.TempDir()
	outbox := filepath.Join(workdir, "outbox")
	codexHome := filepath.Join(outbox, codexHomeArtifactDirName)
	require.NoError(t, os.MkdirAll(codexHome, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte("auth"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("cfg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "session.json"), []byte("session"), 0o644))

	opts := Options{
		Prompt:        "test",
		WorkDirRoot:   workdir,
		WorktreeRoot:  filepath.Join(workdir, "worktree"),
		ArtifactInbox: filepath.Join(workdir, "inbox"),
		CodexHome:     codexHome,
	}

	require.NoError(t, sanitizeCodexHomeOutput(opts))

	require.NoFileExists(t, filepath.Join(codexHome, "auth.json"))
	require.NoFileExists(t, filepath.Join(codexHome, "config.toml"))
	require.FileExists(t, filepath.Join(codexHome, "session.json"))
}
